package channel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	hostcommon "github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/logger"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Each attempt owns a reader. A connection-level retry must not close the
// shared replay source or manufacture a GetBody from a consumed reader.
func ApplyUpstreamBodyMetadata(req *http.Request, body io.Reader) {
	if req == nil {
		return
	}
	replay, ok := body.(hostcommon.ReplayableBody)
	if !ok {
		return
	}
	if _, storage := body.(hostcommon.BodyStorage); storage {
		req.Body = io.NopCloser(body)
	}
	req.ContentLength = replay.Size()
	if req.GetBody == nil {
		req.GetBody = replay.NewReader
	}
}

func inboundRequest(c *gin.Context) (*http.Request, error) {
	if c == nil || c.Request == nil {
		return nil, errors.New("gateway client request is missing")
	}
	return c.Request, nil
}

func prepareAPIRequest(a Adaptor, c *gin.Context, info *relaycommon.RelayInfo, body io.Reader, form bool) (*http.Request, error) {
	inbound, err := inboundRequest(c)
	if err != nil {
		return nil, err
	}
	if a == nil || info == nil {
		return nil, errors.New("gateway provider configuration is missing")
	}
	target, err := a.GetRequestURL(info)
	if err != nil {
		return nil, fmt.Errorf("resolve upstream endpoint: %w", err)
	}
	req, err := http.NewRequestWithContext(inbound.Context(), inbound.Method, target, body)
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	ApplyUpstreamBodyMetadata(req, body)
	if form {
		req.Header.Set("Content-Type", inbound.Header.Get("Content-Type"))
	}
	if err := a.SetupRequestHeader(c, &req.Header, info); err != nil {
		closeRequestBody(req)
		return nil, fmt.Errorf("configure upstream headers: %w", err)
	}
	overrides, err := ResolveHeaderOverride(info, c)
	if err != nil {
		closeRequestBody(req)
		return nil, err
	}
	applyHeaderOverrideToRequest(req, overrides)
	return req, nil
}

func DoApiRequest(a Adaptor, c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (*http.Response, error) {
	req, err := prepareAPIRequest(a, c, info, body, false)
	if err != nil {
		return nil, err
	}
	return doRequest(c, req, info)
}

func DoFormRequest(a Adaptor, c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (*http.Response, error) {
	req, err := prepareAPIRequest(a, c, info, body, true)
	if err != nil {
		return nil, err
	}
	return doRequest(c, req, info)
}

func DoTaskApiRequest(a TaskAdaptor, c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (*http.Response, error) {
	if a == nil || info == nil {
		return nil, errors.New("gateway task provider configuration is missing")
	}
	target, err := a.BuildRequestURL(info)
	if err != nil {
		return nil, fmt.Errorf("resolve task endpoint: %w", err)
	}
	req, err := newTaskAPIRequest(c, target, body)
	if err != nil {
		return nil, err
	}
	if err := a.BuildRequestHeader(c, req, info); err != nil {
		closeRequestBody(req)
		return nil, fmt.Errorf("configure task headers: %w", err)
	}
	return doRequest(c, req, info)
}

func newTaskAPIRequest(c *gin.Context, target string, body io.Reader) (*http.Request, error) {
	inbound, err := inboundRequest(c)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(inbound.Context(), inbound.Method, target, body)
	if err == nil {
		ApplyUpstreamBodyMetadata(req, body)
	}
	return req, err
}

func DoRequest(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) (*http.Response, error) {
	return doRequest(c, req, info)
}

func keepUpstreamRedirectResponse(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}

// Cancellation stays attached until response consumption ends. Releasing the
// response also unregisters its inbound callback, with idempotent body closure.
type responseBodyLease struct {
	io.ReadCloser
	release  func()
	once     sync.Once
	closeErr error
}

func (body *responseBodyLease) Close() error {
	body.once.Do(func() {
		body.closeErr = body.ReadCloser.Close()
		body.release()
	})
	return body.closeErr
}

func closeRequestBody(req *http.Request) {
	if req != nil && req.Body != nil {
		_ = req.Body.Close()
	}
}

// Protocol handlers own response commitment. Waiting for upstream headers
// emits no SSE heartbeat that could turn a later provider error into HTTP 200.
func doRequest(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) (*http.Response, error) {
	inbound, err := inboundRequest(c)
	if err != nil || req == nil || req.URL == nil || info == nil || info.ChannelMeta == nil {
		closeRequestBody(req)
		return nil, types.NewErrorWithStatusCode(errors.New("gateway upstream request is incomplete"), types.ErrorCodeDoRequestFailed, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
	}
	ctx, cancel := context.WithCancelCause(req.Context())
	stop := context.AfterFunc(inbound.Context(), func() { cancel(context.Cause(inbound.Context())) })
	release := func() { stop(); cancel(nil) }
	if inbound.Context().Err() != nil {
		cancel(context.Cause(inbound.Context()))
	}
	req = req.Clone(ctx)
	client, err := service.GetHttpClientWithProxySettings(info.ChannelSetting.Proxy, info.ChannelSetting)
	if err != nil || client == nil {
		release()
		closeRequestBody(req)
		if err == nil {
			err = errors.New("gateway HTTP client is unavailable")
		}
		return nil, types.NewError(err, types.ErrorCodeDoRequestFailed, types.ErrOptionWithSkipRetry())
	}
	// Redirect rules are attempt-scoped; the cached transport remains shared.
	selected := *client
	selected.CheckRedirect = keepUpstreamRedirectResponse
	logger.LogDebug(c, "upstream request: %s", relaycommon.SanitizeURLForLog(req.URL.String()))
	resp, err := selected.Do(req)
	if err != nil {
		cause := context.Cause(ctx)
		release()
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		status := http.StatusBadGateway
		options := []types.GatewayErrorOptions{types.ErrOptionWithHideErrMsg("上游连接失败，请稍后重试。")}
		if cause != nil {
			err, status = cause, http.StatusRequestTimeout
			if errors.Is(cause, context.DeadlineExceeded) {
				status = http.StatusGatewayTimeout
			}
			options = []types.GatewayErrorOptions{types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog(), types.ErrOptionWithHideErrMsg("请求已取消或等待超时。")}
		}
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeDoRequestFailed, status, options...)
	}
	if resp == nil || resp.Body == nil {
		release()
		return nil, types.NewErrorWithStatusCode(errors.New("upstream returned an empty response"), types.ErrorCodeDoRequestFailed, http.StatusBadGateway)
	}
	resp.Body = &responseBodyLease{ReadCloser: resp.Body, release: release}
	if id := resp.Header.Get(hostcommon.RequestIdKey); id != "" {
		c.Set(hostcommon.UpstreamRequestIdKey, id)
	}
	logger.LogDebug(c, "upstream negotiated: %s", resp.Proto)
	return resp, nil
}

func DoWssRequest(a Adaptor, c *gin.Context, info *relaycommon.RelayInfo, _ io.Reader) (*websocket.Conn, error) {
	inbound, err := inboundRequest(c)
	if err != nil {
		return nil, err
	}
	if a == nil || info == nil || info.ChannelMeta == nil {
		return nil, errors.New("gateway WebSocket provider configuration is missing")
	}
	target, err := a.GetRequestURL(info)
	if err != nil {
		return nil, fmt.Errorf("resolve WebSocket endpoint: %w", err)
	}
	headers := make(http.Header)
	if err := a.SetupRequestHeader(c, &headers, info); err != nil {
		return nil, fmt.Errorf("configure WebSocket headers: %w", err)
	}
	overrides, err := ResolveHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	for name, value := range overrides {
		headers.Set(name, value)
	}
	if contentType := inbound.Header.Get("Content-Type"); contentType != "" {
		headers.Set("Content-Type", contentType)
	}
	dialer := *websocket.DefaultDialer
	if info.ChannelSetting.Proxy != "" {
		proxyURL, _, err := hostcommon.ParseProxyURLRuntime(info.ChannelSetting.Proxy)
		if err != nil {
			return nil, err
		}
		dialer.Proxy = http.ProxyURL(proxyURL)
	}
	connection, resp, err := dialer.DialContext(inbound.Context(), toWebSocketURL(target), headers)
	if err == nil {
		return connection, nil
	}
	status := http.StatusBadGateway
	if resp != nil {
		status = resp.StatusCode
		service.CloseResponseBodyGracefully(resp)
	}
	options := []types.GatewayErrorOptions{types.ErrOptionWithHideErrMsg("上游实时连接失败，请稍后重试。")}
	if inbound.Context().Err() != nil {
		err = inbound.Context().Err()
		options = append(options, types.ErrOptionWithSkipRetry())
	}
	return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeDoRequestFailed, status, options...)
}

func toWebSocketURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	}
	return parsed.String()
}
