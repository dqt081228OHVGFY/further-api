package vertex

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/claude"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/gemini"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"

	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"

	"github.com/dqt081228OHVGFY/further-api/setting/model_setting"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

const (
	RequestModeClaude     = 1
	RequestModeGemini     = 2
	RequestModeOpenSource = 3
)

var claudeModelMap = map[string]string{
	"claude-3-sonnet-20240229":   "claude-3-sonnet@20240229",
	"claude-3-opus-20240229":     "claude-3-opus@20240229",
	"claude-3-haiku-20240307":    "claude-3-haiku@20240307",
	"claude-3-5-sonnet-20240620": "claude-3-5-sonnet@20240620",
	"claude-3-5-sonnet-20241022": "claude-3-5-sonnet-v2@20241022",
	"claude-3-7-sonnet-20250219": "claude-3-7-sonnet@20250219",
	"claude-sonnet-4-20250514":   "claude-sonnet-4@20250514",
	"claude-opus-4-20250514":     "claude-opus-4@20250514",
	"claude-opus-4-1-20250805":   "claude-opus-4-1@20250805",
	"claude-sonnet-4-5-20250929": "claude-sonnet-4-5@20250929",
	"claude-haiku-4-5-20251001":  "claude-haiku-4-5@20251001",
	"claude-opus-4-5-20251101":   "claude-opus-4-5@20251101",
	"claude-opus-4-6":            "claude-opus-4-6",
	"claude-opus-4-7":            "claude-opus-4-7",
	"claude-opus-4-8":            "claude-opus-4-8",
}

const anthropicVersion = "vertex-2023-10-16"

type Adaptor struct {
	RequestMode        int
	AccountCredentials Credentials
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	// Vertex AI's generateContent schema does not expose the Gemini API's
	// function-call identity fields. Strip both sides at this provider boundary.
	if model_setting.GetGeminiSettings().RemoveFunctionResponseIdEnabled {
		removeFunctionCallIDs(request)
	}
	return gemini.NativeContent(info, request)
}

func removeFunctionCallIDs(request *dto.GeminiChatRequest) {
	if request == nil {
		return
	}

	if len(request.Contents) > 0 {
		for i := range request.Contents {
			if len(request.Contents[i].Parts) == 0 {
				continue
			}
			for j := range request.Contents[i].Parts {
				part := &request.Contents[i].Parts[j]
				if part.FunctionCall != nil {
					part.FunctionCall.ID = ""
				}
				if part.FunctionResponse != nil && len(part.FunctionResponse.ID) > 0 {
					part.FunctionResponse.ID = nil
				}
			}
		}
	}

	if len(request.Requests) > 0 {
		for i := range request.Requests {
			removeFunctionCallIDs(&request.Requests[i])
		}
	}
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	if _, err := claude.NativeMessages(c, info, request); err != nil {
		return nil, err
	}
	if v, ok := claudeModelMap[info.UpstreamModelName]; ok {
		if c != nil {
			c.Set("request_model", v)
		}
	} else {
		if c != nil {
			c.Set("request_model", request.Model)
		}
	}
	vertexClaudeReq := copyRequest(request, anthropicVersion)
	return vertexClaudeReq, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, openai.UnsupportedOperation("Vertex", "this operation")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	image, err := gemini.ImagenRequest(info, request)
	if err != nil {
		return nil, err
	}
	return *image, nil
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	a.AccountCredentials = Credentials{}
	if info == nil || !info.HasChannelMeta() {
		a.RequestMode = 0
		return
	}
	if strings.HasPrefix(info.UpstreamModelName, "claude") {
		a.RequestMode = RequestModeClaude
	} else if strings.Contains(info.UpstreamModelName, "llama") ||
		// open source models
		strings.Contains(info.UpstreamModelName, "-maas") {
		a.RequestMode = RequestModeOpenSource
	} else {
		a.RequestMode = RequestModeGemini
	}
}

func (a *Adaptor) getRequestUrl(info *relaycommon.RelayInfo, modelName, action string) (string, error) {
	if info == nil || !info.HasChannelMeta() {
		return "", errors.New("Vertex provider configuration is missing")
	}
	region := GetModelRegion(info.ApiVersion, info.OriginModelName)
	project := ""
	apiKey := info.ChannelOtherSettings.VertexKeyType == dto.VertexKeyTypeAPIKey
	if !apiKey {
		var credentials Credentials
		if err := common.Unmarshal([]byte(info.ApiKey), &credentials); err != nil {
			return "", fmt.Errorf("decode Vertex service account: %w", err)
		}
		a.AccountCredentials = credentials
		project = credentials.ProjectID
	}
	var target string
	switch a.RequestMode {
	case RequestModeGemini:
		target = BuildGoogleModelURL(info.ChannelBaseUrl, DefaultAPIVersion, project, region, modelName, action)
	case RequestModeClaude:
		target = BuildAnthropicModelURL(info.ChannelBaseUrl, DefaultAPIVersion, project, region, modelName, action)
	case RequestModeOpenSource:
		if apiKey {
			return "", errors.New("Vertex open-source models require a service account")
		}
		target = BuildOpenSourceChatCompletionsURL(info.ChannelBaseUrl, project, region)
	default:
		return "", errors.New("Vertex model protocol is not supported")
	}
	endpoint, err := url.Parse(target)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return "", errors.New("Vertex provider endpoint is not valid")
	}
	if apiKey {
		query := endpoint.Query()
		query.Set("key", info.ApiKey)
		endpoint.RawQuery = query.Encode()
	}
	return endpoint.String(), nil
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil || !info.HasChannelMeta() {
		return "", errors.New("Vertex provider configuration is missing")
	}
	suffix := ""
	if a.RequestMode == RequestModeGemini {
		if info.IsStream {
			suffix = "streamGenerateContent?alt=sse"
		} else {
			suffix = "generateContent"
		}

		if strings.HasPrefix(info.UpstreamModelName, "imagen") {
			suffix = "predict"
		}
		return a.getRequestUrl(info, info.UpstreamModelName, suffix)
	} else if a.RequestMode == RequestModeClaude {
		if info.IsStream {
			suffix = "streamRawPredict?alt=sse"
		} else {
			suffix = "rawPredict"
		}
		model := info.UpstreamModelName
		if v, ok := claudeModelMap[info.UpstreamModelName]; ok {
			model = v
		}
		return a.getRequestUrl(info, model, suffix)
	} else if a.RequestMode == RequestModeOpenSource {
		return a.getRequestUrl(info, "", "")
	}
	return "", errors.New("unsupported request mode")
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	if c == nil || c.Request == nil || req == nil || info == nil || !info.HasChannelMeta() {
		return errors.New("Vertex request headers require a request and provider configuration")
	}
	channel.SetupApiRequestHeader(info, c, req)
	if info.ChannelOtherSettings.VertexKeyType != dto.VertexKeyTypeAPIKey {
		accessToken, err := getAccessTokenWithContext(c.Request.Context(), a, info)
		if err != nil {
			return err
		}
		req.Set("Authorization", "Bearer "+accessToken)
	}
	if a.AccountCredentials.ProjectID != "" {
		req.Set("x-goog-user-project", a.AccountCredentials.ProjectID)
	}
	if strings.Contains(info.UpstreamModelName, "claude") {
		claude.CommonClaudeHeadersOperation(c, req, info)
	}
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("Vertex chat request is missing")
	}
	if info == nil || !info.HasChannelMeta() {
		return nil, errors.New("Vertex provider configuration is missing")
	}
	switch a.RequestMode {
	case RequestModeClaude:
		converted, err := claude.MessagesFrom(c, info, request)
		if err != nil {
			return nil, err
		}
		return a.ConvertClaudeRequest(c, info, converted)
	case RequestModeGemini:
		if strings.HasPrefix(info.UpstreamModelName, "imagen") {
			return a.chatImage(c, info, request)
		}
		converted, err := gemini.ContentFrom(c, info, request)
		if err != nil {
			return nil, err
		}
		return a.ConvertGeminiRequest(c, info, converted)
	case RequestModeOpenSource:
		return openai.NativeChat(c, info, request)
	default:
		return nil, openai.UnsupportedOperation("Vertex", "this model protocol")
	}
}
func (a *Adaptor) chatImage(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	image := dto.ImageRequest{Model: request.Model, Size: request.Size, N: lo.ToPtr(uint(1))}
	if image.Size == "" {
		image.Size = "1024x1024"
	}
	if request.N != nil && *request.N > 0 {
		image.N = lo.ToPtr(uint(*request.N))
	}
	for _, message := range request.Messages {
		if message.Role == "user" && message.StringContent() != "" {
			image.Prompt = message.StringContent()
			break
		}
	}
	if image.Prompt == "" {
		image.Prompt, _ = request.Prompt.(string)
	}
	if image.Prompt == "" {
		return nil, errors.New("Imagen requires a user prompt")
	}
	if len(request.ExtraBody) > 0 {
		var extra struct {
			N          *uint  `json:"n"`
			Size       string `json:"size"`
			Aspect     string `json:"aspectRatio"`
			Parameters struct {
				Aspect string `json:"aspectRatio"`
			} `json:"parameters"`
		}
		if err := common.Unmarshal(request.ExtraBody, &extra); err != nil {
			return nil, fmt.Errorf("decode Imagen options: %w", err)
		}
		if extra.N != nil && *extra.N > 0 {
			image.N = extra.N
		}
		if extra.Size != "" {
			image.Size = extra.Size
		}
		if extra.Aspect != "" {
			image.Size = extra.Aspect
		}
		if extra.Parameters.Aspect != "" {
			image.Size = extra.Parameters.Aspect
		}
	}
	if c != nil {
		c.Set("request_model", request.Model)
	}
	return a.ConvertImageRequest(c, info, image)
}

func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, openai.UnsupportedOperation("Vertex", "reranking")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, openai.UnsupportedOperation("Vertex", "this operation")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	switch a.RequestMode {
	case RequestModeClaude:
		converted, err := claude.MessagesFrom(c, info, &request)
		if err != nil {
			return nil, err
		}
		return a.ConvertClaudeRequest(c, info, converted)
	case RequestModeGemini:
		converted, err := gemini.ContentFrom(c, info, &request)
		if err != nil {
			return nil, err
		}
		return a.ConvertGeminiRequest(c, info, converted)
	case RequestModeOpenSource:
		return openai.NativeResponses(c, info, request)
	default:
		return nil, openai.UnsupportedOperation("Vertex", "Responses input for this model")
	}
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (any, *types.GatewayError) {
	switch a.RequestMode {
	case RequestModeClaude:
		return claude.MessagesResponse(c, response, info)
	case RequestModeGemini:
		return gemini.ContentResponse(c, response, info)
	case RequestModeOpenSource:
		return openai.ProtocolResponse(c, response, info, "")
	default:
		return nil, types.NewError(errors.New("Vertex response protocol was not initialized"), types.ErrorCodeBadResponseBody)
	}
}

func (a *Adaptor) GetModelList() []string {
	result := make([]string, 0, len(ModelList)+len(claude.ModelList)+len(gemini.ModelList))
	result = append(result, ModelList...)
	result = append(result, claude.ModelList...)
	result = append(result, gemini.ModelList...)
	return lo.Uniq(result)
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
