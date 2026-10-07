package xunfei

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func sparkTestContext(t *testing.T, base string, streaming bool) (*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest, *httptest.ResponseRecorder) {
	t.Helper()
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	request := &dto.GeneralOpenAIRequest{Model: "SparkDesk-v3.5", Stream: &streaming, Messages: []dto.Message{{Role: "user", Content: "hello"}}}
	info := &relaycommon.RelayInfo{OriginModelName: request.Model, RelayFormat: types.RelayFormatOpenAI,
		RelayMode: relayconstant.RelayModeChatCompletions, IsStream: streaming, ShouldIncludeUsage: true,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 1, ChannelType: constant.ChannelTypeXunfei, ChannelBaseUrl: base, ApiKey: "app|test-secret|test-key"}}
	return c, info, request, writer
}

func sparkFrame(text string, status, output int) XunfeiChatResponse {
	var frame XunfeiChatResponse
	frame.Header.Sid = "spark-local"
	frame.Payload.Choices.Status = status
	frame.Payload.Usage.Text = dto.Usage{PromptTokens: 8, CompletionTokens: output, TotalTokens: 8 + output}
	if text != "" {
		frame.Payload.Choices.Text = []XunfeiChatResponseTextItem{{Index: 0, Content: text}}
	}
	return frame
}

func TestSparkSignedURLPreservesQueryAndUsesOfficialCanonicalSignature(t *testing.T) {
	now := time.Date(2026, 10, 4, 1, 2, 3, 0, time.UTC)
	raw, err := signSparkURLAt("wss://spark.example/v3.5/chat?region=cn", "test-key", "test-secret", now)
	require.NoError(t, err)
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, "cn", parsed.Query().Get("region"))
	require.Equal(t, now.Format(http.TimeFormat), parsed.Query().Get("date"))
	authorization, err := base64.StdEncoding.DecodeString(parsed.Query().Get("authorization"))
	require.NoError(t, err)
	mac := hmac.New(sha256.New, []byte("test-secret"))
	_, _ = mac.Write([]byte("host: spark.example\ndate: " + now.Format(http.TimeFormat) + "\nGET /v3.5/chat HTTP/1.1"))
	require.Contains(t, string(authorization), "api_key=\"test-key\"")
	require.Contains(t, string(authorization), base64.StdEncoding.EncodeToString(mac.Sum(nil)))
}

func TestSparkBufferedResponseUsesLatestUsageAndEveryChoice(t *testing.T) {
	seen := make(chan XunfeiChatRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		socket, err := (&websocket.Upgrader{}).Upgrade(w, request, nil)
		if err != nil {
			return
		}
		defer socket.Close()
		var submitted XunfeiChatRequest
		if socket.ReadJSON(&submitted) != nil {
			return
		}
		seen <- submitted
		first := sparkFrame("hello", 0, 2)
		first.Payload.Choices.Text = append(first.Payload.Choices.Text, XunfeiChatResponseTextItem{Index: 1, Content: "bonjour"})
		second := sparkFrame("!", 1, 3)
		second.Payload.Choices.Text = append(second.Payload.Choices.Text, XunfeiChatResponseTextItem{Index: 1, Content: "!"})
		for _, frame := range []XunfeiChatResponse{first, second, sparkFrame("", 2, 4)} {
			if socket.WriteJSON(frame) != nil {
				return
			}
		}
	}))
	defer server.Close()
	c, info, request, writer := sparkTestContext(t, server.URL, false)
	adapter := &Adaptor{}
	adapter.Init(info)
	converted, err := adapter.ConvertOpenAIRequest(c, info, request)
	require.NoError(t, err)
	body, err := json.Marshal(converted)
	require.NoError(t, err)
	raw, err := adapter.DoRequest(c, info, strings.NewReader(string(body)))
	require.NoError(t, err)
	response := raw.(*http.Response)
	defer response.Body.Close()
	usage, failure := adapter.DoResponse(c, response, info)
	require.Nil(t, failure)
	require.Equal(t, 12, usage.(*dto.Usage).TotalTokens)
	var output dto.OpenAITextResponse
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &output))
	require.Len(t, output.Choices, 2)
	require.Equal(t, "hello!", output.Choices[0].Message.StringContent())
	require.Equal(t, "bonjour!", output.Choices[1].Message.StringContent())
	require.Equal(t, 8, output.Usage.PromptTokens)
	require.Equal(t, 4, output.Usage.CompletionTokens)
	submitted := <-seen
	require.Equal(t, "app", submitted.Header.AppId)
	require.Equal(t, "generalv3.5", submitted.Parameter.Chat.Domain)
}

func TestSparkErrorAfterPartialStreamDoesNotEmitSuccessfulTerminal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		socket, err := (&websocket.Upgrader{}).Upgrade(w, request, nil)
		if err != nil {
			return
		}
		defer socket.Close()
		_, _, _ = socket.ReadMessage()
		_ = socket.WriteJSON(sparkFrame("partial", 1, 1))
		var rejected XunfeiChatResponse
		rejected.Header.Code, rejected.Header.Message = 10013, "provider rejected request"
		_ = socket.WriteJSON(rejected)
	}))
	defer server.Close()
	c, info, request, writer := sparkTestContext(t, server.URL, true)
	adapter := &Adaptor{}
	_, err := adapter.ConvertOpenAIRequest(c, info, request)
	require.NoError(t, err)
	encoded, err := json.Marshal(request)
	require.NoError(t, err)
	raw, err := adapter.DoRequest(c, info, strings.NewReader(string(encoded)))
	require.NoError(t, err)
	_, failure := adapter.DoResponse(c, raw.(*http.Response), info)
	require.NotNil(t, failure)
	require.Contains(t, writer.Body.String(), "partial")
	require.NotContains(t, writer.Body.String(), "[DONE]")
	require.NotContains(t, writer.Body.String(), "\"finish_reason\":\"stop\"")
	require.True(t, info.StreamStatus.ResponseFailed())
}

func TestSparkCancellationClosesSocketAndReturnsJoinedOwner(t *testing.T) {
	ready, closed := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		socket, err := (&websocket.Upgrader{}).Upgrade(w, request, nil)
		if err != nil {
			return
		}
		defer socket.Close()
		_, _, _ = socket.ReadMessage()
		close(ready)
		_, _, _ = socket.ReadMessage()
		close(closed)
	}))
	defer server.Close()
	c, info, request, _ := sparkTestContext(t, server.URL, true)
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	adapter := &Adaptor{}
	_, err := adapter.ConvertOpenAIRequest(c, info, request)
	require.NoError(t, err)
	body, err := json.Marshal(request)
	require.NoError(t, err)
	raw, err := adapter.DoRequest(c, info, strings.NewReader(string(body)))
	require.NoError(t, err)
	<-ready
	completed := make(chan *types.GatewayError, 1)
	go func() { _, failure := adapter.DoResponse(c, raw.(*http.Response), info); completed <- failure }()
	cancel()
	select {
	case failure := <-completed:
		require.ErrorIs(t, failure, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled Spark response did not stop")
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled Spark socket remained open")
	}
}

type sparkFailedWriter struct{ *httptest.ResponseRecorder }

func (writer sparkFailedWriter) Write([]byte) (int, error) {
	return 0, errors.New("downstream write failed")
}

func TestSparkDownstreamWriteFailurePropagatesAndStopsResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		socket, err := (&websocket.Upgrader{}).Upgrade(w, request, nil)
		if err != nil {
			return
		}
		defer socket.Close()
		_, _, _ = socket.ReadMessage()
		_ = socket.WriteJSON(sparkFrame("output", 2, 1))
	}))
	defer server.Close()
	c, info, request, _ := sparkTestContext(t, server.URL, true)
	failed, _ := gin.CreateTestContext(sparkFailedWriter{httptest.NewRecorder()})
	failed.Request = c.Request
	adapter := &Adaptor{}
	_, err := adapter.ConvertOpenAIRequest(failed, info, request)
	require.NoError(t, err)
	body, err := json.Marshal(request)
	require.NoError(t, err)
	raw, err := adapter.DoRequest(failed, info, strings.NewReader(string(body)))
	require.NoError(t, err)
	_, failure := adapter.DoResponse(failed, raw.(*http.Response), info)
	require.ErrorContains(t, failure, "downstream write failed")
	require.True(t, info.StreamStatus.ResponseFailed())
}
