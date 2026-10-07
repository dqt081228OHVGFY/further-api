package tencent

import (
	"errors"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"strings"
)

const tokenHubBaseURL = "https://tokenhub.tencentmaas.com"

// Credential shape chooses the wire contract once for the active attempt.
type DispatchAdaptor struct{ channel.Adaptor }

func (a *DispatchAdaptor) Init(info *relaycommon.RelayInfo) {
	if info != nil && info.HasChannelMeta() && strings.Contains(info.ApiKey, "|") {
		a.Adaptor = &Adaptor{}
	} else {
		a.Adaptor = &TokenHubProvider{}
		if info != nil && info.HasChannelMeta() && (info.ChannelBaseUrl == "" || info.ChannelBaseUrl == constant.ChannelBaseURLs[constant.ChannelTypeTencent]) {
			info.ChannelBaseUrl = tokenHubBaseURL
		}
	}
	a.Adaptor.Init(info)
}
func (a *DispatchAdaptor) GetModelList() []string { return append([]string(nil), ModelList...) }
func (a *DispatchAdaptor) GetChannelName() string { return ChannelName }

type TokenHubProvider struct{ audioFormat string }

func (a *TokenHubProvider) Init(info *relaycommon.RelayInfo) {
	a.audioFormat = ""
	openai.InitializeProtocol(info)
}
func (a *TokenHubProvider) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return openai.RequestEndpoint(info)
}
func (a *TokenHubProvider) SetupRequestHeader(c *gin.Context, headers *http.Header, info *relaycommon.RelayInfo) error {
	return openai.SetupProviderHeaders(c, headers, info)
}
func (a *TokenHubProvider) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return openai.NativeChat(c, info, request)
}
func (a *TokenHubProvider) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return openai.ChatFrom(c, info, request)
}
func (a *TokenHubProvider) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return openai.ChatFrom(c, info, request)
}
func (a *TokenHubProvider) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return openai.NativeResponses(c, info, request)
}
func (a *TokenHubProvider) ConvertEmbeddingRequest(_ *gin.Context, _ *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}
func (a *TokenHubProvider) ConvertRerankRequest(_ *gin.Context, _ int, request dto.RerankRequest) (any, error) {
	return request, nil
}
func (a *TokenHubProvider) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return openai.ImagePayload(c, info, request)
}
func (a *TokenHubProvider) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	body, err := openai.AudioPayload(c, info, request)
	if err == nil {
		a.audioFormat = request.ResponseFormat
	}
	return body, err
}
func (a *TokenHubProvider) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (any, error) {
	if info == nil {
		return nil, errors.New("TokenHub request context is missing")
	}
	switch info.RelayMode {
	case relayconstant.RelayModeAudioTranscription, relayconstant.RelayModeAudioTranslation:
		return channel.DoFormRequest(a, c, info, body)
	case relayconstant.RelayModeRealtime:
		return channel.DoWssRequest(a, c, info, body)
	default:
		return channel.DoApiRequest(a, c, info, body)
	}
}
func (a *TokenHubProvider) DoResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (any, *types.GatewayError) {
	return openai.ProtocolResponse(c, response, info, a.audioFormat)
}
func (a *TokenHubProvider) GetModelList() []string { return append([]string(nil), ModelList...) }
func (a *TokenHubProvider) GetChannelName() string { return ChannelName }
