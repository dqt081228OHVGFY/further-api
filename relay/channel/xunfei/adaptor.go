package xunfei

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type Adaptor struct {
	request *dto.GeneralOpenAIRequest
	version string
	socket  *channel.NativeSocketSession
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) { openai.InitializeProtocol(info) }
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil || !info.HasChannelMeta() {
		return "", errors.New("讯飞渠道配置不存在。")
	}
	_, secret, key, err := sparkCredentials(info.ApiKey)
	if err != nil {
		return "", err
	}
	version := a.version
	if version == "" {
		version = "v1.1"
	}
	endpoint, err := sparkEndpoint(info.ChannelBaseUrl, version)
	if err != nil {
		return "", err
	}
	return signSparkURLAt(endpoint, key, secret, time.Now())
}
func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, header)
	return nil
}
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("讯飞聊天请求为空。")
	}
	version := getAPIVersion(c, request.Model)
	if !sparkVersionPattern.MatchString(version) {
		return nil, errors.New("讯飞API版本格式无效。")
	}
	if _, err := prepareSparkRequest(request, "", apiVersion2domain(version)); err != nil {
		return nil, err
	}
	a.version = version
	a.request = request
	return request, nil
}
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	converted, err := openai.ChatFrom(c, info, request)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, converted)
}
func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	converted, err := openai.ChatFrom(c, info, request)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, converted)
}
func (a *Adaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, openai.UnsupportedOperation(ChannelName, "audio")
}
func (a *Adaptor) ConvertImageRequest(*gin.Context, *relaycommon.RelayInfo, dto.ImageRequest) (any, error) {
	return nil, openai.UnsupportedOperation(ChannelName, "images")
}
func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, openai.UnsupportedOperation(ChannelName, "reranking")
}
func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, openai.UnsupportedOperation(ChannelName, "embeddings")
}
func (a *Adaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, openai.UnsupportedOperation(ChannelName, "Responses")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (any, error) {
	if c == nil || c.Request == nil || info == nil || !info.HasChannelMeta() {
		return nil, errors.New("讯飞请求上下文无效。")
	}
	app, _, _, err := sparkCredentials(info.ApiKey)
	if err != nil {
		return nil, err
	}
	request := a.request
	if body != nil {
		data, err := io.ReadAll(io.LimitReader(body, 16<<20+1))
		if err != nil {
			return nil, err
		}
		if len(data) > 16<<20 {
			return nil, errors.New("讯飞请求超过容量限制。")
		}
		var decoded dto.GeneralOpenAIRequest
		if err := json.Unmarshal(data, &decoded); err != nil {
			return nil, err
		}
		request = &decoded
	}
	if request == nil {
		return nil, errors.New("讯飞请求尚未转换。")
	}
	a.version = getAPIVersion(c, request.Model)
	if !sparkVersionPattern.MatchString(a.version) {
		return nil, errors.New("讯飞API版本格式无效。")
	}
	payload, err := prepareSparkRequest(request, app, apiVersion2domain(a.version))
	if err != nil {
		return nil, err
	}
	target, err := a.GetRequestURL(info)
	if err != nil {
		return nil, err
	}
	headers := make(http.Header)
	if err := a.SetupRequestHeader(c, &headers, info); err != nil {
		return nil, err
	}
	socket, err := channel.DialNativeSocket(c, info, target, headers, "spark")
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(payload)
	if err == nil {
		err = socket.Write(websocket.TextMessage, encoded)
	}
	if err != nil {
		socket.Close()
		return nil, err
	}
	a.request, a.socket = request, socket
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: &sparkSocketBody{Reader: bytes.NewReader(nil), owner: socket}}, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, _ *http.Response, info *relaycommon.RelayInfo) (any, *types.GatewayError) {
	if a.socket == nil || a.request == nil {
		return nil, sparkResponseError(errors.New("讯飞执行连接不存在。"))
	}
	defer a.socket.Close()
	return executeSparkResponse(c, info, a.request, a.socket)
}
func (a *Adaptor) GetModelList() []string { return append([]string(nil), ModelList...) }
func (a *Adaptor) GetChannelName() string { return ChannelName }

type sparkSocketBody struct {
	*bytes.Reader
	owner *channel.NativeSocketSession
}

func (body *sparkSocketBody) Close() error { return body.owner.Close() }

func sparkCredentials(raw string) (string, string, string, error) {
	parts := strings.Split(raw, "|")
	if len(parts) != 3 {
		return "", "", "", errors.New("讯飞鉴权格式应为 appid|api_secret|api_key。")
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
		if parts[i] == "" || strings.ContainsAny(parts[i], "\r\n\"\\") {
			return "", "", "", errors.New("讯飞鉴权参数无效。")
		}
	}
	return parts[0], parts[1], parts[2], nil
}
