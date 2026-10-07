package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayRetryRespectsEveryRequestBoundaryForChannelErrors(t *testing.T) {
	for _, test := range []struct {
		name      string
		remaining int
		cancel    bool
		commit    bool
		skip      bool
		reason    string
	}{
		{"exhausted", 0, false, false, false, "attempt_budget_exhausted"},
		{"explicit skip", 2, false, false, true, "non_retryable_error"},
		{"client left", 2, true, false, false, "request_canceled"},
		{"stream started", 2, false, true, false, "response_committed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
			if test.cancel {
				cancel()
			}
			if test.commit {
				_, _ = c.Writer.WriteString("data: first\n\n")
			}
			var options []types.GatewayErrorOptions
			if test.skip {
				options = append(options, types.ErrOptionWithSkipRetry())
			}
			err := types.NewError(errors.New("channel failed"), types.ErrorCodeChannelNoAvailableKey, options...)
			decision := DecideRelayRetry(c, err, test.remaining)
			require.Equal(t, "stop", decision.Action)
			require.Equal(t, test.reason, decision.Reason)
		})
	}
}

func TestBufferedResponsePreservesRepeatedHeadersAndInstanceOwnership(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Writer.Header().Set(common.RequestIdKey, "local-id")
	source := &http.Response{StatusCode: http.StatusOK, Header: http.Header{
		"Connection": []string{"keep-alive, X-Transient"}, "X-Transient": []string{"connection-value"},
		"X-Trace": []string{"a", "b"}, "X-Empty": nil, "Content-Length": []string{"900"},
		"Content-Encoding": []string{"gzip"}, "Set-Cookie": []string{"provider-session=secret"},
		common.RequestIdKey: []string{"upstream-id"},
	}}
	IOCopyBytesGracefully(c, source, []byte(`{"ok":true}`))
	require.Equal(t, []string{"a", "b"}, c.Writer.Header().Values("X-Trace"))
	require.Equal(t, "local-id", c.Writer.Header().Get(common.RequestIdKey))
	require.Equal(t, "upstream-id", c.GetString(common.UpstreamRequestIdKey))
	require.Equal(t, "11", c.Writer.Header().Get("Content-Length"))
	for _, name := range []string{"Connection", "X-Transient", "X-Empty", "Content-Encoding", "Set-Cookie"} {
		require.Empty(t, c.Writer.Header().Values(name), name)
	}
}

type boundedErrorBody struct {
	consumed int
	closed   bool
}

func (body *boundedErrorBody) Read(target []byte) (int, error) {
	for i := range target {
		target[i] = 'x'
	}
	body.consumed += len(target)
	return len(target), nil
}
func (body *boundedErrorBody) Close() error { body.closed = true; return nil }

func TestUpstreamFailureReaderCannotAllocateUnboundedProviderBody(t *testing.T) {
	body := &boundedErrorBody{}
	result := RelayErrorHandler(context.Background(), &http.Response{StatusCode: http.StatusBadGateway, Body: body}, false)
	require.NotNil(t, result)
	require.True(t, body.closed)
	require.Equal(t, maxUpstreamErrorBodyBytes+1, body.consumed)
	require.Less(t, len(result.Error()), 100)
}

func TestUpstreamFailureReaderStopsOnClientCancellation(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan *types.GatewayError, 1)
	go func() {
		done <- RelayErrorHandler(ctx, &http.Response{StatusCode: http.StatusBadGateway, Body: reader}, false)
	}()
	cancel()
	select {
	case result := <-done:
		require.ErrorIs(t, result, context.Canceled)
		require.True(t, types.IsSkipRetryError(result))
	case <-time.After(2 * time.Second):
		t.Fatal("provider failure reader ignored cancellation")
	}
	_, err := io.WriteString(writer, "late data")
	require.Error(t, err)
}

func TestStatusMappingRejectsInvalidHTTPStatuses(t *testing.T) {
	for _, value := range []string{"0", "-1", "101", "600", "999999999999999999999999999999", "503.5", `"NaN"`} {
		err := &types.GatewayError{StatusCode: http.StatusTooManyRequests}
		ResetStatusCode(err, `{"429":`+value+`}`)
		require.Equal(t, http.StatusTooManyRequests, err.StatusCode)
	}
	require.NotPanics(t, func() { TaskErrorFromAPIError(&types.GatewayError{StatusCode: http.StatusBadRequest}) })
	require.NotPanics(t, func() { TaskErrorWrapper(nil, "test", http.StatusBadRequest) })
	require.NotPanics(t, func() { RelayErrorHandler(nil, nil, false) })
	require.False(t, ShouldCopyUpstreamHeader(nil, "X-Empty", nil))
	require.False(t, ShouldCopyUpstreamHeader(nil, "X-Test", []string{strings.Repeat("x", 2) + "\r\nInvalid: header"}))
}
