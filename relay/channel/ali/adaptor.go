package ali

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/claude"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type Adaptor struct {
}

const aliAnthropicMessagesModelsEnv = "ALI_ANTHROPIC_MESSAGES_MODELS"
const defaultAliAnthropicMessagesModels = "qwen,deepseek-v4,kimi,glm,minimax-m"

func supportsAliAnthropicMessages(modelName string) bool {
	normalizedModelName := strings.ToLower(strings.TrimSpace(modelName))
	if normalizedModelName == "" {
		return false
	}

	return lo.SomeBy(aliAnthropicMessagesModelPatterns(), func(pattern string) bool {
		return strings.Contains(normalizedModelName, pattern)
	})
}

func aliAnthropicMessagesModelPatterns() []string {
	configuredModels := common.GetEnvOrDefaultString(aliAnthropicMessagesModelsEnv, defaultAliAnthropicMessagesModels)
	return lo.FilterMap(strings.Split(configuredModels, ","), func(item string, _ int) (string, bool) {
		pattern := strings.ToLower(strings.TrimSpace(item))
		return pattern, pattern != ""
	})
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	converted, err := openai.ChatFrom(c, info, request)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, converted)
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	if supportsAliAnthropicMessages(info.UpstreamModelName) {
		return req, nil
	}

	result, err := service.ConvertRequest(c, info, types.RelayFormatOpenAI, req)
	if err != nil {
		return nil, err
	}
	oaiReq, ok := result.Value.(*dto.GeneralOpenAIRequest)
	if !ok {
		return nil, fmt.Errorf("expected OpenAI chat completions request, got %T", result.Value)
	}
	if info.SupportStreamOptions && info.IsStream {
		oaiReq.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
	}
	return a.ConvertOpenAIRequest(c, info, oaiReq)
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	openai.InitializeProtocol(info)
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil || !info.HasChannelMeta() {
		return "", errors.New("Bailian provider configuration is missing")
	}
	route := "/compatible-mode/v1/chat/completions"
	if info.RelayFormat == types.RelayFormatClaude && supportsAliAnthropicMessages(info.UpstreamModelName) {
		route = "/apps/anthropic/v1/messages"
	} else {
		switch info.RelayMode {
		case constant.RelayModeEmbeddings:
			route = "/compatible-mode/v1/embeddings"
		case constant.RelayModeRerank:
			route = "/api/v1/services/rerank/text-rerank/text-rerank"
		case constant.RelayModeResponses:
			route = "/api/v2/apps/protocols/compatible-mode/v1/responses"
		case constant.RelayModeCompletions:
			route = "/compatible-mode/v1/completions"
		}
	}
	return openai.ProviderEndpoint(info.ChannelBaseUrl, route)
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	if info.IsStream {
		req.Set("X-DashScope-SSE", "enable")
	}
	if c.GetString("plugin") != "" {
		req.Set("X-DashScope-Plugin", c.GetString("plugin"))
	}
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	// docs: https://bailian.console.aliyun.com/?tab=api#/api/?type=model&url=2712216
	// fix: InternalError.Algo.InvalidParameter: The value of the enable_thinking parameter is restricted to True.
	//if strings.Contains(request.Model, "thinking") {
	//	request.EnableThinking = true
	//	request.Stream = true
	//	info.IsStream = true
	//}
	//// fix: ali parameter.enable_thinking must be set to false for non-streaming calls
	//if !info.IsStream {
	//	request.EnableThinking = false
	//}

	switch info.RelayMode {
	default:
		aliReq := requestOpenAI2Ali(*request, info.UpstreamModelName)
		return aliReq, nil
	}
}

// Bailian image generation and editing
// are served by the alibaba task plugin through the openai_image host
// protocol, which claims every declared image model on /v1/images/*. A model
// reaching this adaptor is not declared by the plugin (or is named
// differently from the Bailian model list), so the request cannot be served.
// The rejection is a 400 that skips channel retries: every channel of this
// type refuses the same name, and a retryable 500 would only hide the
// misconfiguration behind unrelated channels.
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, types.NewErrorWithStatusCode(
		fmt.Errorf("image model %q is not served by the alibaba task plugin; use a Bailian image model name declared by the plugin or map it with the channel model mapping", info.UpstreamModelName),
		types.ErrorCodeInvalidRequest,
		http.StatusBadRequest,
		types.ErrOptionWithSkipRetry(),
	)
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return ConvertRerankRequest(request), nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, openai.UnsupportedOperation("ali", "audio operations")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.GatewayError) {
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		if supportsAliAnthropicMessages(info.UpstreamModelName) {
			return claude.MessagesResponse(c, resp, info)
		}
		return openai.ProtocolResponse(c, resp, info, "")
	default:
		switch info.RelayMode {
		case constant.RelayModeRerank:
			err, usage = RerankHandler(c, resp, info)
		default:
			usage, err = openai.ProtocolResponse(c, resp, info, "")
		}
		return usage, err
	}
}

func (a *Adaptor) GetModelList() []string {
	return append([]string(nil), ModelList...)
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
