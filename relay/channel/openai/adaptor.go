package openai

import (
	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

// Gateway attempts keep protocol state here; provider operations are independent wire functions.
type Adaptor struct {
	ChannelType    int
	ResponseFormat string
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType, a.ResponseFormat = 0, ""
	if info != nil && info.HasChannelMeta() {
		a.ChannelType = info.ChannelType
	}
	InitializeProtocol(info)
}
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return RequestEndpoint(info)
}
func (a *Adaptor) SetupRequestHeader(c *gin.Context, headers *http.Header, info *relaycommon.RelayInfo) error {
	return SetupProviderHeaders(c, headers, info)
}
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return NativeChat(c, info, request)
}
func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return ChatFrom(c, info, request)
}
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return ChatFrom(c, info, request)
}
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return NativeResponses(c, info, request)
}
func (a *Adaptor) ConvertRerankRequest(_ *gin.Context, _ int, request dto.RerankRequest) (any, error) {
	return request, nil
}
func (a *Adaptor) ConvertEmbeddingRequest(_ *gin.Context, _ *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	body, err := AudioPayload(c, info, request)
	if err == nil {
		a.ResponseFormat = request.ResponseFormat
	}
	return body, err
}
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return ImagePayload(c, info, request)
}
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (any, error) {
	if err := requestConfiguration(info); err != nil {
		return nil, err
	}
	switch info.RelayMode {
	case relayconstant.RelayModeAudioTranscription, relayconstant.RelayModeAudioTranslation:
		return channel.DoFormRequest(a, c, info, body)
	case relayconstant.RelayModeImagesEdits:
		if !isJSONRequest(c) {
			return channel.DoFormRequest(a, c, info, body)
		}
	case relayconstant.RelayModeRealtime:
		return channel.DoWssRequest(a, c, info, body)
	}
	return channel.DoApiRequest(a, c, info, body)
}
func (a *Adaptor) DoResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (any, *types.GatewayError) {
	return ProtocolResponse(c, response, info, a.ResponseFormat)
}
func (a *Adaptor) GetModelList() []string {
	return append([]string(nil), ProviderFor(a.ChannelType).Models...)
}
func (a *Adaptor) GetChannelName() string { return ProviderFor(a.ChannelType).Name }
