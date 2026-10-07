package gemini

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting/model_setting"
	"github.com/gin-gonic/gin"
)

type modelProtocol uint8

const (
	protocolGenerate modelProtocol = iota
	protocolEmbed
	protocolImagen
)

func protocolForModel(model string) modelProtocol {
	if strings.HasPrefix(model, "imagen") {
		return protocolImagen
	}
	for _, prefix := range []string{"text-embedding", "embedding", "gemini-embedding"} {
		if strings.HasPrefix(model, prefix) {
			return protocolEmbed
		}
	}
	return protocolGenerate
}

func configured(info *relaycommon.RelayInfo) error {
	if info == nil || !info.HasChannelMeta() {
		return errors.New("Gemini provider configuration is missing")
	}
	return nil
}

func ContentEndpoint(info *relaycommon.RelayInfo) (string, error) {
	if err := configured(info); err != nil {
		return "", err
	}
	endpoint, err := url.Parse(info.ChannelBaseUrl)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return "", errors.New("Gemini requires an absolute HTTP(S) endpoint")
	}
	action := "generateContent"
	switch protocolForModel(info.UpstreamModelName) {
	case protocolImagen:
		action = "predict"
	case protocolEmbed:
		action = "embedContent"
		if info.IsGeminiBatchEmbedding {
			action = "batchEmbedContents"
		}
	default:
		if info.IsStream {
			action = "streamGenerateContent"
			query := endpoint.Query()
			query.Set("alt", "sse")
			endpoint.RawQuery = query.Encode()
			if info.RelayMode == constant.RelayModeGemini {
				info.DisablePing = true
			}
		}
	}
	prefix := strings.TrimRight(endpoint.Path, "/") + "/" + model_setting.GetGeminiVersionSetting(info.UpstreamModelName) + "/models/"
	escapedPrefix := strings.TrimRight(endpoint.EscapedPath(), "/") + "/" + model_setting.GetGeminiVersionSetting(info.UpstreamModelName) + "/models/"
	endpoint.Path = prefix + info.UpstreamModelName + ":" + action
	endpoint.RawPath = escapedPrefix + url.PathEscape(info.UpstreamModelName) + ":" + action
	endpoint.Fragment = ""
	return endpoint.String(), nil
}

func ContentHeaders(c *gin.Context, headers *http.Header, info *relaycommon.RelayInfo) error {
	if err := configured(info); err != nil {
		return err
	}
	if c == nil || c.Request == nil || headers == nil {
		return errors.New("Gemini authentication requires a request and headers")
	}
	if *headers == nil {
		*headers = make(http.Header)
	}
	channel.SetupApiRequestHeader(info, c, headers)
	headers.Set("X-Goog-Api-Key", info.ApiKey)
	return nil
}

func NativeContent(info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (*dto.GeminiChatRequest, error) {
	if request == nil {
		return nil, errors.New("Gemini content request is missing")
	}
	if err := configured(info); err != nil {
		return nil, err
	}
	if err := relayconvert.ApplyGeminiThinkingConfigChecked(request, info); err != nil {
		return nil, err
	}
	if len(request.Contents) > 0 && request.Contents[0].Role == "" {
		request.Contents[0].Role = "user"
	}
	for i := range request.Contents {
		for j := range request.Contents[i].Parts {
			file := request.Contents[i].Parts[j].FileData
			if file != nil && file.MimeType == "" && strings.Contains(file.FileUri, "www.youtube.com") {
				file.MimeType = "video/webm"
			}
		}
	}
	return request, nil
}

func ContentFrom(c *gin.Context, info *relaycommon.RelayInfo, source any) (*dto.GeminiChatRequest, error) {
	if err := configured(info); err != nil {
		return nil, err
	}
	result, err := service.ConvertRequest(c, info, types.RelayFormatGemini, source)
	if err != nil {
		return nil, err
	}
	request, ok := result.Value.(*dto.GeminiChatRequest)
	if !ok || request == nil {
		return nil, fmt.Errorf("Gemini conversion returned %T instead of generateContent", result.Value)
	}
	return request, nil
}

func BatchEmbeddings(info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	if err := configured(info); err != nil {
		return nil, err
	}
	inputs := request.ParseInput()
	if textInputs, ok := request.Input.([]string); ok {
		inputs = append([]string(nil), textInputs...)
	}
	if request.Input == nil || len(inputs) == 0 {
		return nil, errors.New("Gemini embeddings require a nonempty input")
	}
	items := make([]map[string]any, 0, len(inputs))
	for _, input := range inputs {
		item := map[string]any{"model": "models/" + info.UpstreamModelName, "content": dto.GeminiChatContent{Parts: []dto.GeminiPart{{Text: input}}}}
		switch info.UpstreamModelName {
		case "text-embedding-004", "gemini-embedding-exp-03-07", "gemini-embedding-001":
			if request.Dimensions != nil && *request.Dimensions > 0 {
				item["outputDimensionality"] = *request.Dimensions
			}
		}
		items = append(items, item)
	}
	info.IsGeminiBatchEmbedding = true
	return map[string]any{"requests": items}, nil
}

func ImagenRequest(info *relaycommon.RelayInfo, request dto.ImageRequest) (*dto.GeminiImageRequest, error) {
	if err := configured(info); err != nil {
		return nil, err
	}
	if protocolForModel(info.UpstreamModelName) != protocolImagen {
		return nil, errors.New("Gemini image generation requires an Imagen model")
	}
	aspect := "1:1"
	size := strings.TrimSpace(request.Size)
	if strings.Contains(size, ":") {
		aspect = size
	} else if mapped, ok := map[string]string{"256x256": "1:1", "512x512": "1:1", "1024x1024": "1:1", "1536x1024": "3:2", "1024x1536": "2:3", "1024x1792": "9:16", "1792x1024": "16:9"}[size]; ok {
		aspect = mapped
	}
	count := 1
	if request.N != nil {
		count = int(*request.N)
	}
	image := &dto.GeminiImageRequest{Instances: []dto.GeminiImageInstance{{Prompt: request.Prompt}}, Parameters: dto.GeminiImageParameters{SampleCount: count, AspectRatio: aspect, PersonGeneration: "allow_adult"}}
	if request.Quality != "" {
		image.Parameters.ImageSize = "1K"
		if request.Quality == "hd" || request.Quality == "high" || request.Quality == "2K" {
			image.Parameters.ImageSize = "2K"
		}
	}
	return image, nil
}

func ContentResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (any, *types.GatewayError) {
	if err := configured(info); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	if response == nil || response.Body == nil {
		return nil, types.NewOpenAIError(errors.New("Gemini returned an empty response"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	if info.RelayMode == constant.RelayModeResponses {
		if info.IsStream {
			return GeminiResponsesStreamHandler(c, info, response)
		}
		return GeminiResponsesHandler(c, info, response)
	}
	if info.RelayMode == constant.RelayModeGemini {
		if strings.Contains(info.RequestURLPath, ":embedContent") || strings.Contains(info.RequestURLPath, ":batchEmbedContents") {
			return NativeGeminiEmbeddingHandler(c, response, info)
		}
		if info.IsStream {
			return GeminiTextGenerationStreamHandler(c, info, response)
		}
		return GeminiTextGenerationHandler(c, info, response)
	}
	switch protocolForModel(info.UpstreamModelName) {
	case protocolImagen:
		return GeminiImageHandler(c, info, response)
	case protocolEmbed:
		return GeminiEmbeddingHandler(c, info, response)
	}
	if info.IsStream {
		return GeminiChatStreamHandler(c, info, response)
	}
	return GeminiChatHandler(c, info, response)
}
