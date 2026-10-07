package coze

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCozeStreamReadsLastEventWithoutTrailingBlankLine(t *testing.T) {
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "bot-model"}, RelayFormat: types.RelayFormatOpenAI}
	response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("event: conversation.message.delta\ndata: {\"type\":\"answer\",\"content\":\"hello\"}\n\nevent: conversation.chat.completed\ndata: {\"usage\":{\"input_count\":7,\"output_count\":2,\"token_count\":9}}"))}
	usage, err := cozeChatStreamHandler(c, info, response)
	require.Nil(t, err)
	require.Equal(t, 9, usage.TotalTokens)
	require.Contains(t, writer.Body.String(), "hello")
	require.Contains(t, writer.Body.String(), "[DONE]")
}

func TestCozePollingStopsWhenCallerCancels(t *testing.T) {
	queried := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v3/chat" {
			_, _ = io.WriteString(w, `{"code":0,"data":{"id":"chat&other=value","conversation_id":"conversation/one"}}`)
		} else {
			select {
			case queried <- struct{}{}:
			default:
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"status":"in_progress"}}`)
		}
	}))
	defer server.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "test-key", UpstreamModelName: "bot-model"}}
	result := make(chan error, 1)
	go func() { _, err := (&Adaptor{}).DoRequest(c, info, strings.NewReader(`{}`)); result <- err }()
	select {
	case <-queried:
	case err := <-result:
		t.Fatalf("polling returned early: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("polling did not start")
	}
	cancel()
	select {
	case err := <-result:
		require.True(t, errors.Is(err, context.Canceled), "got %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("polling did not cancel")
	}
}
