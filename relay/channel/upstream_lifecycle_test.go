package channel

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func awaitUpstreamSignal(t *testing.T, signal <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for " + label)
	}
}

func TestDoRequestBindsUnboundProviderRequestToClientCancellation(t *testing.T) {
	service.InitHttpClient()
	started, canceled := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	defer upstream.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	req, err := http.NewRequest(http.MethodPost, upstream.URL, bytes.NewBufferString("{}"))
	require.NoError(t, err)
	result := make(chan error, 1)
	go func() {
		_, err := DoRequest(c, req, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}})
		result <- err
	}()
	awaitUpstreamSignal(t, started, "provider request")
	cancel()
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
		var gatewayErr *types.GatewayError
		require.True(t, errors.As(err, &gatewayErr))
		require.True(t, types.IsSkipRetryError(gatewayErr))
	case <-time.After(3 * time.Second):
		t.Fatal("provider request continued after client cancellation")
	}
	awaitUpstreamSignal(t, canceled, "provider cancellation")
}

func TestDoRequestCancellationRemainsAttachedToResponseBody(t *testing.T) {
	service.InitHttpClient()
	canceled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(canceled)
	}))
	defer upstream.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil).WithContext(ctx)
	req, err := http.NewRequest(http.MethodGet, upstream.URL, nil)
	require.NoError(t, err)
	response, err := DoRequest(c, req, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}})
	require.NoError(t, err)
	defer response.Body.Close()
	cancel()
	_, err = io.ReadAll(response.Body)
	require.Error(t, err)
	awaitUpstreamSignal(t, canceled, "provider response cancellation")
	require.NoError(t, response.Body.Close())
	require.NoError(t, response.Body.Close())
}

func TestStreamingProviderFailureDoesNotCommitDownstreamHeartbeat(t *testing.T) {
	service.InitHttpClient()
	setting := operation_setting.GetGeneralSetting()
	previousEnabled, previousSeconds := setting.PingIntervalEnabled, setting.PingIntervalSeconds
	setting.PingIntervalEnabled, setting.PingIntervalSeconds = true, 1
	t.Cleanup(func() { setting.PingIntervalEnabled, setting.PingIntervalSeconds = previousEnabled, previousSeconds })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(1100 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"message":"upstream busy"}}`)
	}))
	defer upstream.Close()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	req, err := http.NewRequest(http.MethodGet, upstream.URL, nil)
	require.NoError(t, err)
	response, err := DoRequest(c, req, &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{}})
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
	require.False(t, c.Writer.Written())
	require.Empty(t, recorder.Body.String())
	require.Empty(t, c.Writer.Header().Get("Content-Type"))
}

func TestHeaderRulesRetainRegexCaseAndSuppressConnectionNominatedFields(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("X-Codex-Trace", "trace")
	c.Request.Header.Set("X-Connection-Only", "private")
	c.Request.Header.Set("Connection", "keep-alive, X-Connection-Only")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{HeadersOverride: map[string]any{"regex:^X-[A-Z]+-": ""}}}
	headers, err := ResolveHeaderOverride(info, c)
	require.NoError(t, err)
	require.Equal(t, "trace", headers["x-codex-trace"])
	require.NotContains(t, headers, "x-connection-only")
	info.HeadersOverride = map[string]any{"X-Custom": "value\r\nAuthorization: injected"}
	_, err = ResolveHeaderOverride(info, c)
	require.Error(t, err)
}
