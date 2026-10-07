package ollama

import (
	"errors"

	"io"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/claude"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	converted, err := openai.ChatFrom(c, info, request)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, converted)
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return claude.NativeMessages(c, info, request)
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, openai.UnsupportedOperation("ollama", "audio operations")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, openai.UnsupportedOperation("ollama", "image generation")
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	// OpenAI-compatible chat responses are handled by the OpenAI adaptor, which
	// relies on the thinking-to-content state initialized here.
	openai.InitializeProtocol(info)
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil || !info.HasChannelMeta() {
		return "", errors.New("Ollama provider configuration is missing")
	}
	if info.RelayFormat == types.RelayFormatClaude {
		return claude.MessagesEndpoint(info)
	}
	route := "/api/chat"
	switch info.RelayMode {
	case relayconstant.RelayModeEmbeddings:
		route = "/api/embed"
	case relayconstant.RelayModeResponses:
		route = "/v1/responses"
	case relayconstant.RelayModeResponsesCompact:
		route = "/v1/responses/compact"
	case relayconstant.RelayModeCompletions:
		route = "/api/generate"
	default:
		if info.ChannelOtherSettings.OllamaOpenAIChat {
			route = "/v1/chat/completions"
		}
	}
	return openai.ProviderEndpoint(info.ChannelBaseUrl, route)
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		claude.CommonClaudeHeadersOperation(c, req, info)
		anthropicVersion := c.Request.Header.Get("anthropic-version")
		if anthropicVersion == "" {
			anthropicVersion = "2023-06-01"
		}
		req.Set("anthropic-version", anthropicVersion)
	}
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	switch info.RelayMode {
	case relayconstant.RelayModeCompletions:
		return openAIToGenerate(c, request)
	default:
		if info.ChannelOtherSettings.OllamaOpenAIChat {
			return openai.NativeChat(c, info, request)
		}
		return openAIChatToOllamaChat(c, request)
	}
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, openai.UnsupportedOperation("ollama", "reranking")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return requestOpenAI2Embeddings(request), nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return openai.NativeResponses(c, info, request)
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.GatewayError) {
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		return claude.MessagesResponse(c, resp, info)
	default:
		switch info.RelayMode {
		case relayconstant.RelayModeEmbeddings:
			return ollamaEmbeddingHandler(c, info, resp)
		case relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
			return openai.ProtocolResponse(c, resp, info, "")
		default:
			// /api/generate always returns native NDJSON; chat follows the channel setting.
			if info.RelayMode != relayconstant.RelayModeCompletions && info.ChannelOtherSettings.OllamaOpenAIChat {
				return openai.ProtocolResponse(c, resp, info, "")
			}
			if info.IsStream {
				return ollamaStreamHandler(c, info, resp)
			}
			return ollamaChatHandler(c, info, resp)
		}
	}
}

func (a *Adaptor) GetModelList() []string {
	return append([]string(nil), ModelList...)
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
