package furtherapi

import (
	"errors"
	"io"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/claude"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/gemini"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// Adaptor connects a further-api gateway to the protocol-specific relay codecs.
// Its upstream routes are selected from the incoming wire protocol, independent
// of the model name or the channel's configured API type.
type Adaptor struct {
	responseFormat string
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	*a = Adaptor{}
	openai.InitializeProtocol(info)
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return upstreamURL(info)
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	if c == nil || c.Request == nil || header == nil || info == nil {
		return errors.New("request context, headers and relay info are required")
	}
	channel.SetupApiRequestHeader(info, c, header)
	return setProtocolHeaders(c, header, info)
}

func (a *Adaptor) ConvertOpenAIRequest(_ *gin.Context, _ *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(_ *gin.Context, _ *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(_ *gin.Context, _ *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return claude.NativeMessages(c, info, request)
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return gemini.NativeContent(info, request)
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return openai.ImagePayload(c, info, request)
}

func (a *Adaptor) ConvertRerankRequest(_ *gin.Context, _ int, request dto.RerankRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	body, err := openai.AudioPayload(c, info, request)
	if err == nil {
		a.responseFormat = request.ResponseFormat
	}
	return body, err
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	switch info.RelayMode {
	case relayconstant.RelayModeAudioTranscription, relayconstant.RelayModeAudioTranslation,
		relayconstant.RelayModeImagesEdits:
		return channel.DoFormRequest(a, c, info, requestBody)
	case relayconstant.RelayModeRealtime:
		return channel.DoWssRequest(a, c, info, requestBody)
	default:
		return channel.DoApiRequest(a, c, info, requestBody)
	}
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.GatewayError) {
	switch protocolFor(info) {
	case protocolClaude:
		return claude.MessagesResponse(c, resp, info)
	case protocolGemini:
		return gemini.ContentResponse(c, resp, info)
	default:
		return openai.ProtocolResponse(c, resp, info, a.responseFormat)
	}
}

func (a *Adaptor) GetModelList() []string { return nil }

func (a *Adaptor) GetChannelName() string { return "further-api" }
