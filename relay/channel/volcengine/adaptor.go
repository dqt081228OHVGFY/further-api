package volcengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	gatewayconstant "github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/claude"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	contextKeyTTSRequest     = "volcengine_tts_request"
	contextKeyResponseFormat = "response_format"
)

type Adaptor struct {
	speech *VolcengineTTSRequest
	socket *channel.NativeSocketSession
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) { openai.InitializeProtocol(info) }
func volcBase(info *relaycommon.RelayInfo) string {
	if info.ChannelBaseUrl != "" {
		return strings.TrimRight(info.ChannelBaseUrl, "/")
	}
	return strings.TrimRight(gatewayconstant.GetChannelBaseURL(gatewayconstant.ChannelTypeVolcEngine), "/")
}
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil || !info.HasChannelMeta() {
		return "", errors.New("火山渠道配置为空。")
	}
	base := volcBase(info)
	special, planned := gatewayconstant.ChannelSpecialBases[base]
	if info.RelayMode == constant.RelayModeAudioSpeech {
		operation := "submit"
		if a.speech != nil {
			operation = a.speech.Request.Operation
		}
		return volcSpeechEndpoint(base, operation)
	}
	if info.RelayFormat == types.RelayFormatClaude && planned && special.ClaudeBaseURL != "" {
		return strings.TrimRight(special.ClaudeBaseURL, "/") + "/v1/messages", nil
	}
	paths := map[int]string{constant.RelayModeChatCompletions: "/chat/completions", constant.RelayModeEmbeddings: "/embeddings",
		constant.RelayModeImagesGenerations: "/images/generations", constant.RelayModeImagesEdits: "/images/generations",
		constant.RelayModeRerank: "/rerank", constant.RelayModeResponses: "/responses"}
	path, known := paths[info.RelayMode]
	if !known {
		return "", openai.UnsupportedOperation(ChannelName, fmt.Sprintf("relay mode %d", info.RelayMode))
	}
	if path == "/chat/completions" && planned && special.OpenAIBaseURL != "" {
		return strings.TrimRight(special.OpenAIBaseURL, "/") + path, nil
	}
	if path == "/chat/completions" && strings.HasPrefix(info.UpstreamModelName, "bot") {
		path = "/bots/chat/completions"
	}
	return base + "/api/v3" + path, nil
}
func volcSpeechEndpoint(base, operation string) (string, error) {
	if base == strings.TrimRight(gatewayconstant.GetChannelBaseURL(gatewayconstant.ChannelTypeVolcEngine), "/") {
		if operation == "query" {
			return "https://openspeech.bytedance.com/api/v1/tts", nil
		}
		return "wss://openspeech.bytedance.com/api/v1/tts/ws_binary", nil
	}
	endpoint, err := url.Parse(base)
	if err != nil || endpoint.Hostname() == "" {
		return "", errors.New("火山语音地址无效。")
	}
	switch endpoint.Scheme {
	case "ws", "wss":
		if endpoint.Path == "" || endpoint.Path == "/" {
			endpoint.Path = "/api/v1/tts/ws_binary"
		}
		return endpoint.String(), nil
	case "http", "https":
		if strings.HasSuffix(endpoint.Path, "/api/v1/tts") || strings.HasSuffix(endpoint.Path, "/v1/audio/speech") {
			return endpoint.String(), nil
		}
		if strings.HasSuffix(endpoint.Path, "/ws_binary") {
			if endpoint.Scheme == "https" {
				endpoint.Scheme = "wss"
			} else {
				endpoint.Scheme = "ws"
			}
			return endpoint.String(), nil
		}
		endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/v1/audio/speech"
		endpoint.RawPath = ""
		return endpoint.String(), nil
	}
	return "", errors.New("火山语音地址协议无效。")
}
func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	if header == nil || info == nil || !info.HasChannelMeta() {
		return errors.New("火山请求头上下文无效。")
	}
	channel.SetupApiRequestHeader(info, c, header)
	if info.RelayMode == constant.RelayModeAudioSpeech {
		_, token, err := parseVolcengineAuth(info.ApiKey)
		if err != nil {
			return err
		}
		header.Set("Authorization", "Bearer;"+token)
		header.Set("Content-Type", "application/json")
	} else {
		header.Set("Authorization", "Bearer "+info.ApiKey)
	}
	return nil
}
func (a *Adaptor) ConvertOpenAIRequest(_ *gin.Context, info *relaycommon.RelayInfo, source *dto.GeneralOpenAIRequest) (any, error) {
	if info == nil || !info.HasChannelMeta() || source == nil {
		return nil, errors.New("火山聊天请求为空。")
	}
	request := *source
	request.Messages = append([]dto.Message(nil), source.Messages...)
	if strings.HasPrefix(info.UpstreamModelName, "deepseek") && strings.HasSuffix(info.UpstreamModelName, "-thinking") &&
		!model_setting.ShouldPreserveThinkingSuffix(info.OriginModelName) && !model_setting.ShouldPreserveThinkingSuffix(info.UpstreamModelName) {
		info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-thinking")
		request.Model = info.UpstreamModelName
		request.THINKING = json.RawMessage("{\"type\":\"enabled\"}")
	}
	return &request, nil
}
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, source *dto.ClaudeRequest) (any, error) {
	if info == nil || source == nil {
		return nil, errors.New("火山Messages请求为空。")
	}
	if _, planned := gatewayconstant.ChannelSpecialBases[volcBase(info)]; planned {
		return claude.NativeMessages(c, info, source)
	}
	request, err := openai.ChatFrom(c, info, source)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, request)
}
func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, source *dto.GeminiChatRequest) (any, error) {
	request, err := openai.ChatFrom(c, info, source)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, request)
}
func (a *Adaptor) ConvertEmbeddingRequest(_ *gin.Context, _ *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}
func (a *Adaptor) ConvertRerankRequest(_ *gin.Context, _ int, request dto.RerankRequest) (any, error) {
	return request, nil
}
func (a *Adaptor) ConvertOpenAIResponsesRequest(_ *gin.Context, _ *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return request, nil
}
func (a *Adaptor) ConvertImageRequest(_ *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info == nil || info.RelayMode != constant.RelayModeImagesGenerations && info.RelayMode != constant.RelayModeImagesEdits {
		return nil, openai.UnsupportedOperation(ChannelName, "image mode")
	}
	return request, nil
}
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	if info == nil || !info.HasChannelMeta() || info.RelayMode != constant.RelayModeAudioSpeech {
		return nil, openai.UnsupportedOperation(ChannelName, "audio transcription")
	}
	speech, err := prepareVolcSpeech(info, request)
	if err != nil {
		return nil, err
	}
	a.speech = speech
	if c != nil {
		c.Set(contextKeyTTSRequest, *speech)
		c.Set(contextKeyResponseFormat, speech.Audio.Encoding)
	}
	endpoint, err := a.GetRequestURL(info)
	if err != nil {
		return nil, err
	}
	info.IsStream = strings.HasPrefix(endpoint, "ws://") || strings.HasPrefix(endpoint, "wss://")
	encoded, err := json.Marshal(speech)
	return bytes.NewReader(encoded), err
}
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (any, error) {
	if info == nil || !info.HasChannelMeta() {
		return nil, errors.New("火山执行上下文无效。")
	}
	if info.RelayMode != constant.RelayModeAudioSpeech {
		return channel.DoApiRequest(a, c, info, body)
	}
	if a.speech != nil && body != nil {
		data, err := io.ReadAll(io.LimitReader(body, 4<<20+1))
		if err != nil {
			return nil, err
		}
		if len(data) > 4<<20 {
			return nil, errors.New("火山语音请求超过容量限制。")
		}
		var prepared VolcengineTTSRequest
		if err := json.Unmarshal(data, &prepared); err != nil {
			return nil, err
		}
		if err := validateVolcSpeech(&prepared); err != nil {
			return nil, err
		}
		a.speech = &prepared
		body = bytes.NewReader(data)
	}
	endpoint, err := a.GetRequestURL(info)
	if err != nil {
		return nil, err
	}
	websocketMode := strings.HasPrefix(endpoint, "ws://") || strings.HasPrefix(endpoint, "wss://")
	info.IsStream = websocketMode
	if !websocketMode {
		return channel.DoApiRequest(a, c, info, body)
	}
	if a.speech == nil {
		return nil, errors.New("火山语音请求尚未转换。")
	}
	headers := make(http.Header)
	if err := a.SetupRequestHeader(c, &headers, info); err != nil {
		return nil, err
	}
	socket, err := channel.DialNativeSocket(c, info, endpoint, headers, "speech")
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(a.speech)
	message, _ := NewMessage(MsgTypeFullClientRequest, MsgTypeFlagNoSeq)
	message.Payload = encoded
	frame, frameErr := message.Marshal()
	if err == nil {
		err = frameErr
	}
	if err == nil {
		err = socket.Write(websocket.BinaryMessage, frame)
	}
	if err != nil {
		socket.Close()
		return nil, err
	}
	a.socket = socket
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: &volcSocketBody{Reader: bytes.NewReader(nil), owner: socket}}, nil
}
func (a *Adaptor) DoResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (any, *types.GatewayError) {
	if info == nil || !info.HasChannelMeta() {
		return nil, volcSpeechError(errors.New("火山响应上下文无效。"))
	}
	if info.RelayMode == constant.RelayModeAudioSpeech {
		encoding := "mp3"
		if a.speech != nil {
			encoding = a.speech.Audio.Encoding
		} else if c != nil {
			encoding = mapEncoding(c.GetString(contextKeyResponseFormat))
		}
		if a.socket != nil {
			defer a.socket.Close()
			return consumeVolcSpeech(c, info, a.socket, encoding)
		}
		return handleTTSResponse(c, response, info, encoding)
	}
	if info.RelayFormat == types.RelayFormatClaude {
		if special, known := gatewayconstant.ChannelSpecialBases[volcBase(info)]; known && special.ClaudeBaseURL != "" {
			return claude.MessagesResponse(c, response, info)
		}
	}
	return openai.ProtocolResponse(c, response, info, "")
}
func (a *Adaptor) GetModelList() []string { return append([]string(nil), ModelList...) }
func (a *Adaptor) GetChannelName() string { return ChannelName }

type volcSocketBody struct {
	*bytes.Reader
	owner *channel.NativeSocketSession
}

func (body *volcSocketBody) Close() error { return body.owner.Close() }
