package volcengine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type VolcengineTTSRequest struct {
	App     VolcengineTTSApp     `json:"app"`
	User    VolcengineTTSUser    `json:"user"`
	Audio   VolcengineTTSAudio   `json:"audio"`
	Request VolcengineTTSReqInfo `json:"request"`
}

type VolcengineTTSApp struct {
	AppID   string `json:"appid"`
	Token   string `json:"token"`
	Cluster string `json:"cluster"`
}

type VolcengineTTSUser struct {
	UID string `json:"uid"`
}

type VolcengineTTSAudio struct {
	VoiceType        string  `json:"voice_type"`
	Encoding         string  `json:"encoding"`
	SpeedRatio       float64 `json:"speed_ratio"`
	Rate             int     `json:"rate"`
	Bitrate          int     `json:"bitrate,omitempty"`
	LoudnessRatio    float64 `json:"loudness_ratio,omitempty"`
	EnableEmotion    bool    `json:"enable_emotion,omitempty"`
	Emotion          string  `json:"emotion,omitempty"`
	EmotionScale     float64 `json:"emotion_scale,omitempty"`
	ExplicitLanguage string  `json:"explicit_language,omitempty"`
	ContextLanguage  string  `json:"context_language,omitempty"`
}

type VolcengineTTSReqInfo struct {
	ReqID           string                   `json:"reqid"`
	Text            string                   `json:"text"`
	Operation       string                   `json:"operation"`
	Model           string                   `json:"model,omitempty"`
	TextType        string                   `json:"text_type,omitempty"`
	SilenceDuration float64                  `json:"silence_duration,omitempty"`
	WithTimestamp   interface{}              `json:"with_timestamp,omitempty"`
	ExtraParam      *VolcengineTTSExtraParam `json:"extra_param,omitempty"`
}

type VolcengineTTSExtraParam struct {
	DisableMarkdownFilter      bool                      `json:"disable_markdown_filter,omitempty"`
	EnableLatexTn              bool                      `json:"enable_latex_tn,omitempty"`
	MuteCutThreshold           string                    `json:"mute_cut_threshold,omitempty"`
	MuteCutRemainMs            string                    `json:"mute_cut_remain_ms,omitempty"`
	DisableEmojiFilter         bool                      `json:"disable_emoji_filter,omitempty"`
	UnsupportedCharRatioThresh float64                   `json:"unsupported_char_ratio_thresh,omitempty"`
	AigcWatermark              bool                      `json:"aigc_watermark,omitempty"`
	CacheConfig                *VolcengineTTSCacheConfig `json:"cache_config,omitempty"`
}

type VolcengineTTSCacheConfig struct {
	TextType int  `json:"text_type,omitempty"`
	UseCache bool `json:"use_cache,omitempty"`
}

type VolcengineTTSResponse struct {
	ReqID    string                     `json:"reqid"`
	Code     int                        `json:"code"`
	Message  string                     `json:"message"`
	Sequence int                        `json:"sequence"`
	Data     string                     `json:"data"`
	Addition *VolcengineTTSAdditionInfo `json:"addition,omitempty"`
}

type VolcengineTTSAdditionInfo struct {
	Duration string `json:"duration"`
}

var openAIToVolcengineVoiceMap = map[string]string{
	"alloy":   "zh_male_M392_conversation_wvae_bigtts",
	"echo":    "zh_male_wenhao_mars_bigtts",
	"fable":   "zh_female_tianmei_mars_bigtts",
	"onyx":    "zh_male_zhibei_mars_bigtts",
	"nova":    "zh_female_shuangkuaisisi_mars_bigtts",
	"shimmer": "zh_female_cancan_mars_bigtts",
}

var responseFormatToEncodingMap = map[string]string{
	"mp3":  "mp3",
	"opus": "ogg_opus",
	"aac":  "mp3",
	"flac": "mp3",
	"wav":  "wav",
	"pcm":  "pcm",
}

func parseVolcengineAuth(raw string) (string, string, error) {
	app, token, found := strings.Cut(raw, "|")
	app, token = strings.TrimSpace(app), strings.TrimSpace(token)
	if !found || app == "" || token == "" || strings.Contains(token, "|") || strings.ContainsAny(app+token, "\r\n") {
		return "", "", errors.New("火山语音鉴权格式应为 appid|access_token。")
	}
	return app, token, nil
}
func mapVoiceType(voice string) string {
	if native, known := openAIToVolcengineVoiceMap[voice]; known {
		return native
	}
	return voice
}
func mapEncoding(format string) string {
	switch format {
	case "ogg_opus", "mp3", "wav", "pcm":
		return format
	}
	if mapped, known := responseFormatToEncodingMap[format]; known {
		return mapped
	}
	return "mp3"
}
func getContentTypeByEncoding(encoding string) string {
	switch encoding {
	case "mp3":
		return "audio/mpeg"
	case "ogg_opus":
		return "audio/ogg"
	case "wav":
		return "audio/wav"
	case "pcm":
		return "audio/pcm"
	}
	return "application/octet-stream"
}
func generateRequestID() string { return uuid.NewString() }

func prepareVolcSpeech(info *relaycommon.RelayInfo, source dto.AudioRequest) (*VolcengineTTSRequest, error) {
	app, token, err := parseVolcengineAuth(info.ApiKey)
	if err != nil {
		return nil, err
	}
	speed := 1.0
	if source.Speed != nil {
		speed = *source.Speed
	}
	request := &VolcengineTTSRequest{App: VolcengineTTSApp{AppID: app, Token: token, Cluster: "volcano_tts"},
		User:    VolcengineTTSUser{UID: fmt.Sprintf("further-api-user-%d", info.UserId)},
		Audio:   VolcengineTTSAudio{VoiceType: mapVoiceType(source.Voice), Encoding: mapEncoding(source.ResponseFormat), SpeedRatio: speed, Rate: 24000},
		Request: VolcengineTTSReqInfo{ReqID: generateRequestID(), Text: source.Input, Operation: "submit", Model: info.UpstreamModelName}}
	if len(source.Metadata) > 0 {
		if err := json.Unmarshal(source.Metadata, request); err != nil {
			return nil, fmt.Errorf("火山语音metadata无效：%w", err)
		}
	}
	request.App.AppID, request.App.Token = app, token
	request.User.UID = fmt.Sprintf("further-api-user-%d", info.UserId)
	request.Request.Text, request.Request.ReqID = source.Input, generateRequestID()
	if request.Request.Model == "" {
		request.Request.Model = info.OriginModelName
	}
	request.Audio.Encoding = mapEncoding(request.Audio.Encoding)
	if err := validateVolcSpeech(request); err != nil {
		return nil, err
	}
	return request, nil
}
func validateVolcSpeech(request *VolcengineTTSRequest) error {
	if request == nil || strings.TrimSpace(request.Request.Text) == "" || request.App.AppID == "" || request.App.Token == "" {
		return errors.New("火山语音请求缺少文本或鉴权。")
	}
	if request.Request.Operation != "submit" && request.Request.Operation != "query" {
		return errors.New("火山语音operation必须为submit或query。")
	}
	if request.Audio.SpeedRatio == 0 {
		request.Audio.SpeedRatio = 1
	}
	if math.IsNaN(request.Audio.SpeedRatio) || math.IsInf(request.Audio.SpeedRatio, 0) || request.Audio.SpeedRatio < 0.2 || request.Audio.SpeedRatio > 3 {
		return errors.New("火山语音speed必须在0.2到3之间。")
	}
	if request.Audio.Rate <= 0 {
		request.Audio.Rate = 24000
	}
	return nil
}
func volcSpeechUsage(info *relaycommon.RelayInfo) *dto.Usage {
	input := max(0, info.GetEstimatePromptTokens())
	return &dto.Usage{PromptTokens: input, TotalTokens: input}
}
func volcSpeechError(err error) *types.GatewayError {
	return types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponse, http.StatusBadGateway)
}

func writeVolcAudio(c *gin.Context, encoding string, data []byte) error {
	if c == nil || c.Request == nil || c.Writer == nil {
		return errors.New("语音输出上下文无效。")
	}
	if err := c.Request.Context().Err(); err != nil {
		return err
	}
	c.Header("Content-Type", getContentTypeByEncoding(encoding))
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(30 * time.Second))
	defer func() { _ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{}) }()
	if _, err := c.Writer.Write(data); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}
func handleTTSResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo, encoding string) (any, *types.GatewayError) {
	if c == nil || c.Request == nil || response == nil || response.Body == nil || info == nil {
		return nil, volcSpeechError(errors.New("语音HTTP响应内容不存在。"))
	}
	defer response.Body.Close()
	done := make(chan struct{})
	stop := context.AfterFunc(c.Request.Context(), func() { response.Body.Close(); close(done) })
	defer func() {
		if !stop() {
			<-done
		}
	}()
	const limit = 256 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, volcSpeechError(err)
	}
	if err := c.Request.Context().Err(); err != nil {
		return nil, volcSpeechError(err)
	}
	if len(body) > limit {
		return nil, volcSpeechError(errors.New("语音HTTP响应超过容量限制。"))
	}
	media, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	audio := body
	if !strings.HasPrefix(media, "audio/") && media != "application/octet-stream" {
		var result VolcengineTTSResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, volcSpeechError(err)
		}
		if result.Code != 3000 {
			return nil, types.NewErrorWithStatusCode(fmt.Errorf("火山语音错误 code=%d：%s", result.Code, result.Message), types.ErrorCodeBadResponse, 400)
		}
		audio, err = base64.StdEncoding.DecodeString(result.Data)
		if err != nil {
			return nil, volcSpeechError(err)
		}
	}
	if len(audio) == 0 {
		return nil, volcSpeechError(errors.New("语音服务未返回音频。"))
	}
	if err := writeVolcAudio(c, encoding, audio); err != nil {
		return nil, volcSpeechError(err)
	}
	info.SetFirstResponseTime()
	return volcSpeechUsage(info), nil
}

func consumeVolcSpeech(c *gin.Context, info *relaycommon.RelayInfo, socket *channel.NativeSocketSession, encoding string) (any, *types.GatewayError) {
	if c == nil || c.Request == nil || info == nil || socket == nil {
		return nil, volcSpeechError(errors.New("语音WebSocket上下文无效。"))
	}
	info.StreamStatus = relaycommon.NewStreamStatus()
	info.StreamStatus.RequireTerminal()
	totalBytes := 0
	fail := func(err error) (any, *types.GatewayError) {
		if c.Request.Context().Err() != nil {
			err = c.Request.Context().Err()
			info.StreamStatus.MarkCancelled()
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, err)
		} else {
			info.StreamStatus.MarkFailed("speech_protocol", "upstream_error", 502)
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, err)
		}
		if totalBytes > 0 {
			return volcSpeechUsage(info), volcSpeechError(err)
		}
		return nil, volcSpeechError(err)
	}
	for {
		kind, frame, err := socket.Read()
		if err != nil {
			return fail(err)
		}
		if kind != websocket.BinaryMessage && kind != websocket.TextMessage {
			return fail(errors.New("语音上游返回非数据帧。"))
		}
		message, err := NewMessageFromBytes(frame)
		if err != nil {
			return fail(err)
		}
		info.ReceivedResponseCount++
		if message.MsgType == MsgTypeError {
			return fail(fmt.Errorf("火山语音错误 code=%d：%s", message.ErrorCode, string(message.Payload)))
		}
		if message.EventType == EventType_ConnectionFailed || message.EventType == EventType_SessionFailed {
			return fail(errors.New("火山语音会话创建失败。"))
		}
		terminal := message.Sequence < 0 || message.MsgTypeFlag == MsgTypeFlagBits(2) || message.EventType == EventType_TTSEnded || message.EventType == EventType_SessionFinished
		if message.MsgType == MsgTypeAudioOnlyServer && len(message.Payload) > 0 {
			totalBytes += len(message.Payload)
			if totalBytes > 256<<20 {
				return fail(errors.New("累计语音输出超过容量限制。"))
			}
			if err := writeVolcAudio(c, encoding, message.Payload); err != nil {
				return fail(err)
			}
			info.SetFirstResponseTime()
		}
		if terminal {
			if totalBytes == 0 {
				return fail(errors.New("语音终态没有音频内容。"))
			}
			info.StreamStatus.MarkCompleted()
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
			return volcSpeechUsage(info), nil
		}
	}
}
