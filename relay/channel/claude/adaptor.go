package claude

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting/model_setting"
	"github.com/gin-gonic/gin"
)

const messagesVersion = "2023-06-01"

// Adaptor is an attempt-local Anthropic provider. Its protocol operations are
// independent functions so Bedrock, Vertex and compatible vendors share wire
// rules without constructing or dispatching through another channel adaptor.
type Adaptor struct{}

func messagesConfiguration(info *relaycommon.RelayInfo) error {
	if info == nil || !info.HasChannelMeta() {
		return errors.New("Anthropic provider configuration is missing")
	}
	return nil
}

func MessagesEndpoint(info *relaycommon.RelayInfo) (string, error) {
	if err := messagesConfiguration(info); err != nil {
		return "", err
	}
	endpoint, err := url.Parse(info.ChannelBaseUrl)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return "", errors.New("Anthropic requires an absolute HTTP(S) endpoint")
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/v1/messages"
	endpoint.RawPath = ""
	endpoint.Fragment = ""
	if shouldAppendClaudeBetaQuery(info) {
		query := endpoint.Query()
		query.Set("beta", "true")
		endpoint.RawQuery = query.Encode()
	}
	return endpoint.String(), nil
}

func shouldAppendClaudeBetaQuery(info *relaycommon.RelayInfo) bool {
	return info != nil && (info.IsClaudeBetaQuery || info.HasChannelMeta() && info.ChannelOtherSettings.ClaudeBetaQuery)
}

// NativeMessages applies provider constraints after model mapping. In
// particular a mapped, suffix-normalized name on RelayInfo remains authoritative.
func NativeMessages(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (*dto.ClaudeRequest, error) {
	if request == nil {
		return nil, errors.New("Anthropic messages request is missing")
	}
	if err := messagesConfiguration(info); err != nil {
		return nil, err
	}
	if request.MaxTokens != nil && *request.MaxTokens == 0 {
		request.MaxTokens = nil
	}
	if err := relayconvert.ApplyClaudeThinkingModel(request, info); err != nil {
		return nil, err
	}
	if request.MaxTokens == nil {
		tokens := uint(model_setting.GetClaudeSettings().GetDefaultMaxTokens(request.Model))
		request.MaxTokens = &tokens
	}
	if info.UpstreamModelName == "" {
		info.UpstreamModelName = request.Model
	}
	return request, nil
}

func MessagesFrom(c *gin.Context, info *relaycommon.RelayInfo, source any) (*dto.ClaudeRequest, error) {
	if err := messagesConfiguration(info); err != nil {
		return nil, err
	}
	result, err := service.ConvertRequest(c, info, types.RelayFormatClaude, source)
	if err != nil {
		return nil, err
	}
	request, ok := result.Value.(*dto.ClaudeRequest)
	if !ok || request == nil {
		return nil, fmt.Errorf("Anthropic conversion returned %T instead of Messages", result.Value)
	}
	return request, nil
}

func CommonClaudeHeadersOperation(c *gin.Context, headers *http.Header, info *relaycommon.RelayInfo) {
	if headers == nil || info == nil || !info.HasChannelMeta() {
		return
	}
	if *headers == nil {
		*headers = make(http.Header)
	}
	if c != nil && c.Request != nil {
		if beta := c.Request.Header.Get("Anthropic-Beta"); beta != "" {
			headers.Set("Anthropic-Beta", beta)
		}
	}
	model_setting.GetClaudeSettings().WriteHeaders(info.OriginModelName, headers)
}

func MessagesHeaders(c *gin.Context, headers *http.Header, info *relaycommon.RelayInfo) error {
	if err := messagesConfiguration(info); err != nil {
		return err
	}
	if c == nil || c.Request == nil || headers == nil {
		return errors.New("Anthropic authentication requires a request and headers")
	}
	if *headers == nil {
		*headers = make(http.Header)
	}
	channel.SetupApiRequestHeader(info, c, headers)
	version := c.Request.Header.Get("Anthropic-Version")
	if version == "" {
		version = messagesVersion
	}
	headers.Set("Anthropic-Version", version)
	headers.Set("X-Api-Key", info.ApiKey)
	CommonClaudeHeadersOperation(c, headers, info)
	return nil
}

func MessagesResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (any, *types.GatewayError) {
	if err := messagesConfiguration(info); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	if response == nil || response.Body == nil {
		return nil, types.NewOpenAIError(errors.New("Anthropic returned an empty response"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	info.FinalRequestRelayFormat = types.RelayFormatClaude
	if !info.IsStream {
		return ClaudeHandler(c, response, info)
	}
	if info.RelayFormat == types.RelayFormatOpenAIResponses {
		return ClaudeResponsesStreamHandler(c, response, info)
	}
	return ClaudeStreamHandler(c, response, info)
}

func unsupportedMessagesOperation(name string) error {
	return fmt.Errorf("Anthropic Messages does not support %s", name)
}
func (a *Adaptor) Init(*relaycommon.RelayInfo) {}
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return MessagesEndpoint(info)
}
func (a *Adaptor) SetupRequestHeader(c *gin.Context, headers *http.Header, info *relaycommon.RelayInfo) error {
	return MessagesHeaders(c, headers, info)
}
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return NativeMessages(c, info, request)
}
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("OpenAI request is missing")
	}
	return MessagesFrom(c, info, request)
}
func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	if request == nil {
		return nil, errors.New("Gemini request is missing")
	}
	return MessagesFrom(c, info, request)
}
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return MessagesFrom(c, info, &request)
}
func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, unsupportedMessagesOperation("reranking")
}
func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, unsupportedMessagesOperation("embeddings")
}
func (a *Adaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, unsupportedMessagesOperation("audio generation or transcription")
}
func (a *Adaptor) ConvertImageRequest(*gin.Context, *relaycommon.RelayInfo, dto.ImageRequest) (any, error) {
	return nil, unsupportedMessagesOperation("image generation")
}
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, body)
}
func (a *Adaptor) DoResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (any, *types.GatewayError) {
	return MessagesResponse(c, response, info)
}
func (a *Adaptor) GetModelList() []string { return append([]string(nil), ModelList...) }
func (a *Adaptor) GetChannelName() string { return ChannelName }
