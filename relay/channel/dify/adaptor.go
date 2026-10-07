package dify

import (
	"errors"

	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	"io"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

const (
	BotTypeChatFlow   = 1 // chatflow default
	BotTypeAgent      = 2
	BotTypeWorkFlow   = 3
	BotTypeCompletion = 4
)

type Adaptor struct {
	BotType int
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, openai.UnsupportedOperation("dify", "Gemini content input")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, openai.UnsupportedOperation("dify", "Anthropic Messages input")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, openai.UnsupportedOperation("dify", "audio operations")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, openai.UnsupportedOperation("dify", "image generation")
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	a.BotType = BotTypeChatFlow
	openai.InitializeProtocol(info)
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil || !info.HasChannelMeta() {
		return "", errors.New("Dify provider configuration is missing")
	}
	route := "/v1/chat-messages"
	switch a.BotType {
	case BotTypeWorkFlow:
		route = "/v1/workflows/run"
	case BotTypeCompletion:
		route = "/v1/completion-messages"
	}
	return openai.ProviderEndpoint(info.ChannelBaseUrl, route)
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, headers *http.Header, info *relaycommon.RelayInfo) error {
	return openai.SetupProviderHeaders(c, headers, info)
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return requestOpenAI2Dify(c, info, *request)
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, openai.UnsupportedOperation("dify", "reranking")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, openai.UnsupportedOperation("dify", "embeddings")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, openai.UnsupportedOperation("dify", "Responses input")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.GatewayError) {
	if info.IsStream {
		return difyStreamHandler(c, info, resp)
	} else {
		return difyHandler(c, info, resp)
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
