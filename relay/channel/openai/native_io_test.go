package openai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type brokenMediaBody struct{}

func (brokenMediaBody) Read([]byte) (int, error) { return 0, errors.New("source read failed") }
func (brokenMediaBody) Close() error             { return nil }

type brokenMediaWriter struct{ gin.ResponseWriter }

func (brokenMediaWriter) Write([]byte) (int, error) { return 0, errors.New("client write failed") }

func TestSpeechStreamUsesNestedUsageAndAudioModality(t *testing.T) {
	body := "data: {\"type\":\"speech.audio.done\",\"usage\":{\"input_tokens\":3,\"output_tokens\":7,\"total_tokens\":10}}\n\ndata: [DONE]\n\n"
	c, recorder, response, info := newImageTestContext(t, body, "text/event-stream", true)
	usage, failure := OpenaiTTSHandler(c, response, info)
	require.Nil(t, failure)
	require.Equal(t, 3, usage.PromptTokens)
	require.Equal(t, 7, usage.CompletionTokens)
	require.Equal(t, 7, usage.CompletionTokenDetails.AudioTokens)
	require.Equal(t, 3, usage.PromptTokensDetails.TextTokens)
	require.Contains(t, recorder.Body.String(), "speech.audio.done")
}
func TestSpeechReadFailureDoesNotCommitASuccessResponse(t *testing.T) {
	c, recorder, response, info := newImageTestContext(t, "", "audio/mpeg", false)
	response.Body = brokenMediaBody{}
	usage, failure := OpenaiTTSHandler(c, response, info)
	require.NotNil(t, failure)
	require.Nil(t, usage)
	require.False(t, c.Writer.Written())
	require.Empty(t, recorder.Body.String())
}
func TestTranscriptionWriteFailureIsReturnedAndKeepsServerUsage(t *testing.T) {
	c, _, response, info := newImageTestContext(t, `{"text":"ok","usage":{"input_tokens":23,"output_tokens":2,"total_tokens":25}}`, "application/json", false)
	c.Writer = brokenMediaWriter{c.Writer}
	failure, usage := OpenaiSTTHandler(c, response, info, "json")
	require.NotNil(t, failure)
	require.Equal(t, 23, usage.PromptTokensDetails.AudioTokens)
	require.Equal(t, 2, usage.CompletionTokenDetails.TextTokens)
}

func TestRealtimeReportedUsageReplacesEstimateAndPreservesNextInput(t *testing.T) {
	ledger := newRealtimeLedger()
	ledger.pending = dto.RealtimeUsage{InputTokens: 80, InputTokenDetails: dto.InputTokenDetails{TextTokens: 80}}
	estimate := ledger.response("first")
	estimate.OutputTokens = 30
	ledger.pending = dto.RealtimeUsage{InputTokens: 12, InputTokenDetails: dto.InputTokenDetails{TextTokens: 12}}
	event := dto.RealtimeEvent{Response: &dto.RealtimeResponse{Usage: &dto.RealtimeUsage{InputTokens: 2, OutputTokens: 3}}}
	info := &relaycommon.RelayInfo{UsePrice: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-4o"}}
	require.NoError(t, ledger.finish(nil, info, event, "first"))
	require.NoError(t, ledger.finish(nil, info, event, "first"))
	require.Equal(t, 5, ledger.observed().TotalTokens)
	require.Equal(t, 12, ledger.pending.InputTokens)
	require.Equal(t, 12, ledger.response("next").InputTokens)
}
func TestRealtimeAnonymousFallbackIsNotAddedTwice(t *testing.T) {
	ledger := newRealtimeLedger()
	ledger.pending = dto.RealtimeUsage{InputTokens: 5, InputTokenDetails: dto.InputTokenDetails{TextTokens: 5}}
	info := &relaycommon.RelayInfo{UsePrice: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-4o"}}
	require.NoError(t, ledger.finish(nil, info, dto.RealtimeEvent{EventId: "event-one", Response: &dto.RealtimeResponse{}}, ""))
	require.Equal(t, 5, ledger.observed().TotalTokens)
	require.Empty(t, ledger.responses)
}

func nativeWebsocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()
	accepted := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		socket, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			accepted <- socket
		}
	}))
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	var peer *websocket.Conn
	select {
	case peer = <-accepted:
	case <-time.After(3 * time.Second):
		t.Fatal("websocket fixture did not connect")
	}
	t.Cleanup(func() { _ = peer.Close(); _ = client.Close(); server.Close() })
	return peer, client
}
func TestRealtimeOwnerJoinsBothReadersOnRequestCancellation(t *testing.T) {
	gatewayClient, client := nativeWebsocketPair(t)
	upstream, gatewayUpstream := nativeWebsocketPair(t)
	_ = client
	_ = upstream
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil).WithContext(ctx)
	info := &relaycommon.RelayInfo{ClientWs: gatewayClient, TargetWs: gatewayUpstream, UsePrice: true, ChannelMeta: &relaycommon.ChannelMeta{}}
	completed := make(chan struct{})
	go func() { _, _ = OpenaiRealtimeHandler(c, info); close(completed) }()
	cancel()
	select {
	case <-completed:
	case <-time.After(3 * time.Second):
		t.Fatal("realtime socket readers survived request cancellation")
	}
}
func TestRealtimeSocketReportsOneResponseEvenWhenCompletionIsReplayed(t *testing.T) {
	gatewayClient, client := nativeWebsocketPair(t)
	upstream, gatewayUpstream := nativeWebsocketPair(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
	info := &relaycommon.RelayInfo{ClientWs: gatewayClient, TargetWs: gatewayUpstream, UsePrice: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-4o"}}
	finished := make(chan *dto.RealtimeUsage, 1)
	go func() {
		failure, usage := OpenaiRealtimeHandler(c, info)
		if failure != nil {
			t.Errorf("unexpected realtime error: %v", failure)
		}
		finished <- usage
	}()
	events := []string{`{"type":"response.created","response":{"id":"first"}}`, `{"type":"response.text.delta","response_id":"first","delta":"hello"}`, `{"type":"response.done","response":{"id":"first","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}`}
	events = append(events, events[2])
	for _, event := range events {
		require.NoError(t, upstream.WriteMessage(websocket.TextMessage, []byte(event)))
		require.NoError(t, client.SetReadDeadline(time.Now().Add(3*time.Second)))
		_, body, err := client.ReadMessage()
		require.NoError(t, err)
		require.JSONEq(t, event, string(body))
	}
	require.NoError(t, client.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second)))
	select {
	case usage := <-finished:
		require.NotNil(t, usage)
		require.Equal(t, 5, usage.TotalTokens)
	case <-time.After(3 * time.Second):
		t.Fatal("realtime session did not close")
	}
}
