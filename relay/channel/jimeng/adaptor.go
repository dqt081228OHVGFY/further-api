package jimeng

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, openai.UnsupportedOperation("jimeng", "Gemini content input")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, openai.UnsupportedOperation("jimeng", "Anthropic Messages input")
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil || !info.HasChannelMeta() {
		return "", errors.New("Jimeng provider configuration is missing")
	}
	return openai.ProviderEndpoint(info.ChannelBaseUrl, "/?Action=CVProcess&Version=2022-08-31")
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, headers *http.Header, info *relaycommon.RelayInfo) error {
	if c == nil || c.Request == nil || headers == nil || info == nil || !info.HasChannelMeta() {
		return errors.New("Jimeng request headers are missing")
	}
	if *headers == nil {
		*headers = make(http.Header)
	}
	channel.SetupApiRequestHeader(info, c, headers)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

type LogoInfo struct {
	AddLogo         bool    `json:"add_logo,omitempty"`
	Position        int     `json:"position,omitempty"`
	Language        int     `json:"language,omitempty"`
	Opacity         float64 `json:"opacity,omitempty"`
	LogoTextContent string  `json:"logo_text_content,omitempty"`
}

type imageRequestPayload struct {
	ReqKey     string   `json:"req_key"`                      // Service identifier, fixed value: jimeng_high_aes_general_v21_L
	Prompt     string   `json:"prompt"`                       // Prompt for image generation, supports both Chinese and English
	Seed       int64    `json:"seed,omitempty"`               // Random seed, default -1 (random)
	Width      int      `json:"width,omitempty"`              // Image width, default 512, range [256, 768]
	Height     int      `json:"height,omitempty"`             // Image height, default 512, range [256, 768]
	UsePreLLM  bool     `json:"use_pre_llm,omitempty"`        // Enable text expansion, default true
	UseSR      bool     `json:"use_sr,omitempty"`             // Enable super resolution, default true
	ReturnURL  bool     `json:"return_url,omitempty"`         // Whether to return image URL (valid for 24 hours)
	LogoInfo   LogoInfo `json:"logo_info,omitempty"`          // Watermark information
	ImageUrls  []string `json:"image_urls,omitempty"`         // Image URLs for input
	BinaryData []string `json:"binary_data_base64,omitempty"` // Base64 encoded binary data
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	payload := imageRequestPayload{
		ReqKey: request.Model,
		Prompt: request.Prompt,
	}
	if request.ResponseFormat == "" || request.ResponseFormat == "url" {
		payload.ReturnURL = true // Default to returning image URLs
	}

	if len(request.ExtraFields) > 0 {
		if err := json.Unmarshal(request.ExtraFields, &payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal extra fields: %w", err)
		}
	}

	return payload, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, openai.UnsupportedOperation("jimeng", "reranking")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, openai.UnsupportedOperation("jimeng", "embeddings")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, openai.UnsupportedOperation("jimeng", "audio operations")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, openai.UnsupportedOperation("jimeng", "Responses input")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (any, error) {
	if c == nil || c.Request == nil || info == nil || !info.HasChannelMeta() {
		return nil, errors.New("Jimeng request context is missing")
	}
	endpoint, err := a.GetRequestURL(info)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, endpoint, body)
	if err != nil {
		return nil, err
	}
	channel.ApplyUpstreamBodyMetadata(request, body)
	if err = a.SetupRequestHeader(c, &request.Header, info); err != nil {
		return nil, err
	}
	overrides, err := channel.ResolveHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	for name, value := range overrides {
		request.Header.Set(name, value)
	}
	if err = Sign(c, request, info.ApiKey); err != nil {
		return nil, err
	}
	return channel.DoRequest(c, request, info)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.GatewayError) {
	if info.RelayMode == relayconstant.RelayModeImagesGenerations {
		usage, err = jimengImageHandler(c, resp, info)
	} else if info.IsStream {
		usage, err = openai.OaiStreamHandler(c, info, resp)
	} else {
		usage, err = openai.OpenaiHandler(c, info, resp)
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	return append([]string(nil), ModelList...)
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
