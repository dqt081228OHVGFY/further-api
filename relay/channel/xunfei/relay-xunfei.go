package xunfei

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var sparkVersionPattern = regexp.MustCompile(`^v[0-9]+(?:\.[0-9]+)*$`)

func prepareSparkRequest(request *dto.GeneralOpenAIRequest, app, domain string) (*XunfeiChatRequest, error) {
	if request == nil || len(request.Messages) == 0 {
		return nil, errors.New("讯飞聊天必须包含消息。")
	}
	if len(request.Tools) > 0 || len(request.Functions) > 0 {
		return nil, openai.UnsupportedOperation(ChannelName, "tool calling on the legacy Spark endpoint")
	}
	wire := &XunfeiChatRequest{}
	wire.Header.AppId = app
	wire.Parameter.Chat.Domain = domain
	wire.Parameter.Chat.MaxTokens = request.GetMaxTokens()
	if request.Temperature != nil {
		value := *request.Temperature
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
			return nil, errors.New("讯飞temperature必须在0到1之间。")
		}
		wire.Parameter.Chat.Temperature = &value
	}
	if request.TopK != nil {
		wire.Parameter.Chat.TopK = *request.TopK
	} else if request.N != nil {
		wire.Parameter.Chat.TopK = *request.N
	}
	if wire.Parameter.Chat.TopK < 0 {
		return nil, errors.New("讯飞top_k无效。")
	}
	nativeSystem := domain == "generalv3.5" || domain == "4.0Ultra"
	for _, message := range request.Messages {
		role := message.Role
		if role == "developer" {
			role = "system"
		}
		if role != "system" && role != "user" && role != "assistant" {
			return nil, openai.UnsupportedOperation(ChannelName, "non-text roles")
		}
		text := message.StringContent()
		if role == "system" && !nativeSystem {
			wire.Payload.Message.Text = append(wire.Payload.Message.Text, XunfeiMessage{Role: "user", Content: text}, XunfeiMessage{Role: "assistant", Content: "Okay"})
		} else {
			wire.Payload.Message.Text = append(wire.Payload.Message.Text, XunfeiMessage{Role: role, Content: text})
		}
	}
	return wire, nil
}

func sparkEndpoint(base, version string) (string, error) {
	if base == "" {
		base = "wss://spark-api.xf-yun.com"
	}
	endpoint, err := url.Parse(base)
	if err != nil || endpoint.Hostname() == "" {
		return "", errors.New("讯飞渠道地址无效。")
	}
	switch endpoint.Scheme {
	case "http":
		endpoint.Scheme = "ws"
	case "https":
		endpoint.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", errors.New("讯飞渠道地址必须使用HTTP或WebSocket协议。")
	}
	if endpoint.User != nil || endpoint.Fragment != "" {
		return "", errors.New("讯飞渠道地址不能包含登录信息或片段。")
	}
	if !strings.HasSuffix(strings.TrimRight(endpoint.Path, "/"), "/chat") {
		endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + version + "/chat"
		endpoint.RawPath = ""
	}
	return endpoint.String(), nil
}

func signSparkURLAt(raw, key, secret string, now time.Time) (string, error) {
	endpoint, err := url.Parse(raw)
	if err != nil || endpoint.Hostname() == "" {
		return "", errors.New("讯飞签名地址无效。")
	}
	date := now.UTC().Format(http.TimeFormat)
	canonical := "host: " + endpoint.Host + "\ndate: " + date + "\nGET " + endpoint.EscapedPath() + " HTTP/1.1"
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	authorization := fmt.Sprintf("api_key=\"%s\", algorithm=\"hmac-sha256\", headers=\"host date request-line\", signature=\"%s\"", key, signature)
	query := endpoint.Query()
	query.Set("host", endpoint.Host)
	query.Set("date", date)
	query.Set("authorization", base64.StdEncoding.EncodeToString([]byte(authorization)))
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func apiVersion2domain(version string) string {
	domains := map[string]string{"v1.1": "lite", "v2.1": "generalv2", "v3.1": "generalv3", "v3.5": "generalv3.5", "v4.0": "4.0Ultra"}
	if domain, known := domains[version]; known {
		return domain
	}
	return "general" + version
}
func getAPIVersion(c *gin.Context, model string) string {
	if c != nil && c.Request != nil {
		if version := strings.TrimSpace(c.Query("api-version")); version != "" {
			return version
		}
	}
	if marker := strings.LastIndex(model, "-v"); marker >= 0 {
		return model[marker+1:]
	}
	if c != nil {
		if version := strings.TrimSpace(c.GetString("api_version")); version != "" {
			return version
		}
	}
	return "v1.1"
}

func sparkResponseError(err error) *types.GatewayError {
	status := http.StatusBadGateway
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		status = http.StatusGatewayTimeout
	}
	return types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponse, status)
}

func executeSparkResponse(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest, socket *channel.NativeSocketSession) (*dto.Usage, *types.GatewayError) {
	if c == nil || c.Request == nil || info == nil {
		return nil, sparkResponseError(errors.New("讯飞响应上下文无效。"))
	}
	usage := &dto.Usage{}
	choices := map[int]*strings.Builder{}
	roles := map[int]bool{}
	reported, complete := false, false
	id, created := helper.GetResponseID(c), common.GetTimestamp()
	model := request.Model
	info.StreamStatus = relaycommon.NewStreamStatus()
	info.StreamStatus.RequireTerminal()
	fail := func(err error) (*dto.Usage, *types.GatewayError) {
		if c.Request.Context().Err() != nil {
			err = c.Request.Context().Err()
			info.StreamStatus.MarkCancelled()
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, err)
		} else {
			info.StreamStatus.MarkFailed("spark_protocol", "upstream_error", 502)
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, err)
		}
		return usage, sparkResponseError(err)
	}
	totalBytes := 0
	for !complete {
		kind, data, err := socket.Read()
		if err != nil {
			return fail(err)
		}
		if kind != websocket.TextMessage && kind != websocket.BinaryMessage {
			return fail(errors.New("讯飞返回了非数据帧。"))
		}
		var frame XunfeiChatResponse
		if err := json.Unmarshal(data, &frame); err != nil {
			return fail(err)
		}
		if frame.Header.Code != 0 {
			return fail(fmt.Errorf("讯飞上游错误 code=%d：%s", frame.Header.Code, frame.Header.Message))
		}
		if frame.Payload.Choices.Status < 0 || frame.Payload.Choices.Status > 2 {
			return fail(errors.New("讯飞返回了无效终态标记。"))
		}
		info.SetFirstResponseTime()
		info.ReceivedResponseCount++
		if frame.Header.Sid != "" {
			id = frame.Header.Sid
			c.Set(common.UpstreamRequestIdKey, frame.Header.Sid)
		}
		value := frame.Payload.Usage.Text
		if value.PromptTokens < 0 || value.CompletionTokens < 0 || value.TotalTokens < 0 || value.PromptTokens > common.MaxQuota || value.CompletionTokens > common.MaxQuota || value.TotalTokens > common.MaxQuota {
			return fail(errors.New("讯飞返回了无效用量。"))
		}
		if value.PromptTokens > 0 || value.CompletionTokens > 0 || value.TotalTokens > 0 {
			*usage = value
			usage.PromptTokensDetails = value.PromptTokensDetails.Clone()
			reported = true
		}
		complete = frame.Payload.Choices.Status == 2 || frame.Header.Status == 2
		chunk := dto.ChatCompletionsStreamResponse{Id: id, Object: "chat.completion.chunk", Created: created, Model: model}
		for _, item := range frame.Payload.Choices.Text {
			if item.Index < 0 || item.Index > 255 {
				return fail(errors.New("讯飞返回了无效候选索引。"))
			}
			totalBytes += len(item.Content)
			if totalBytes > 64<<20 {
				return fail(errors.New("讯飞累计输出超过容量限制。"))
			}
			if choices[item.Index] == nil {
				choices[item.Index] = &strings.Builder{}
			}
			choices[item.Index].WriteString(item.Content)
			part := dto.ChatCompletionsStreamResponseChoice{Index: item.Index}
			part.Delta.SetContentString(item.Content)
			if !roles[item.Index] {
				part.Delta.Role = "assistant"
				roles[item.Index] = true
			}
			chunk.Choices = append(chunk.Choices, part)
		}
		if info.IsStream && len(chunk.Choices) > 0 {
			helper.SetEventStreamHeaders(c)
			encoded, err := json.Marshal(chunk)
			if err != nil {
				return fail(err)
			}
			if err := sendSparkStreamFrame(c, info, encoded); err != nil {
				return fail(err)
			}
		}
	}
	if len(choices) == 0 {
		return fail(io.ErrUnexpectedEOF)
	}
	if !reported {
		usage.PromptTokens = info.GetEstimatePromptTokens()
		for _, text := range choices {
			usage.CompletionTokens += service.CountTextToken(text.String(), model)
		}
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	} else if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	info.StreamStatus.MarkCompleted()
	info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	indices := make([]int, 0, len(choices))
	for index := range choices {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	if info.IsStream {
		final := dto.ChatCompletionsStreamResponse{Id: id, Object: "chat.completion.chunk", Created: created, Model: model}
		for _, index := range indices {
			reason := constant.FinishReasonStop
			final.Choices = append(final.Choices, dto.ChatCompletionsStreamResponseChoice{Index: index, FinishReason: &reason})
		}
		if info.ShouldIncludeUsage {
			final.Usage = usage
		}
		helper.SetEventStreamHeaders(c)
		encoded, err := json.Marshal(final)
		if err != nil {
			return fail(err)
		}
		if info.RelayFormat == types.RelayFormatOpenAI || info.RelayFormat == "" {
			if err := sendSparkStreamFrame(c, info, encoded); err != nil {
				return fail(err)
			}
			if err := writeSparkSSE(c, []byte("[DONE]")); err != nil {
				return fail(err)
			}
		} else if err := openai.FinalizeChatStream(c, info, string(encoded), id, created, model, "", usage, true); err != nil {
			return fail(err)
		}
		return usage, nil
	}
	response := &dto.OpenAITextResponse{Id: id, Object: "chat.completion", Created: created, Model: model, Usage: *usage}
	for _, index := range indices {
		response.Choices = append(response.Choices, dto.OpenAITextResponseChoice{Index: index, Message: dto.Message{Role: "assistant", Content: choices[index].String()}, FinishReason: constant.FinishReasonStop})
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return fail(err)
	}
	if info.RelayFormat != types.RelayFormatOpenAI && info.RelayFormat != "" {
		converted, err := service.ConvertResponse(c, info, info.RelayFormat, response)
		if err != nil {
			return fail(err)
		}
		encoded, err = json.Marshal(converted.Value)
		if err != nil {
			return fail(err)
		}
	}
	if err := c.Request.Context().Err(); err != nil {
		return fail(err)
	}
	c.Header("Content-Type", "application/json")
	c.Header("Content-Length", strconv.Itoa(len(encoded)))
	if _, err := c.Writer.Write(encoded); err != nil {
		return fail(err)
	}
	return usage, nil
}

func sendSparkStreamFrame(c *gin.Context, info *relaycommon.RelayInfo, data []byte) error {
	if info.RelayFormat == types.RelayFormatOpenAI || info.RelayFormat == "" {
		info.IncrSendResponseCount()
		return writeSparkSSE(c, data)
	}
	return openai.HandleStreamFormat(c, info, string(data), false, false)
}

func writeSparkSSE(c *gin.Context, data []byte) error {
	if err := c.Request.Context().Err(); err != nil {
		return err
	}
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(30 * time.Second))
	defer func() { _ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{}) }()
	frame := append([]byte("data: "), data...)
	frame = append(frame, '\n', '\n')
	if _, err := c.Writer.Write(frame); err != nil {
		return err
	}
	return helper.FlushWriter(c)
}
