package channel

import (
	"context"
	"errors"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/pkg/wsmanager"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// A native protocol has one reader and one response writer. Cancellation only
// closes its socket; closing the owner joins that callback before returning.
type NativeSocketSession struct {
	Conn         *websocket.Conn
	ctx          context.Context
	idle         time.Duration
	stop         func() bool
	callbackDone chan struct{}
	unregister   func()
	socketOnce   sync.Once
	ownerOnce    sync.Once
	closeError   error
}

func DialNativeSocket(c *gin.Context, info *relaycommon.RelayInfo, target string, headers http.Header, kind string) (*NativeSocketSession, error) {
	if c == nil || c.Request == nil || info == nil || !info.HasChannelMeta() {
		return nil, errors.New("原生WebSocket请求上下文无效。")
	}
	ctx := c.Request.Context()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	headers = headers.Clone()
	if headers == nil {
		headers = make(http.Header)
	}
	overrides, err := ResolveHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	for name, value := range overrides {
		headers.Set(name, value)
	}
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = 15 * time.Second
	if info.ChannelSetting.Proxy != "" {
		proxy, _, err := common.ParseProxyURLRuntime(info.ChannelSetting.Proxy)
		if err != nil {
			return nil, err
		}
		dialer.Proxy = http.ProxyURL(proxy)
	}
	socket, handshake, err := dialer.DialContext(ctx, toWebSocketURL(target), headers)
	if err != nil {
		status := http.StatusBadGateway
		if handshake != nil {
			status = handshake.StatusCode
			if handshake.Body != nil {
				_ = handshake.Body.Close()
			}
		}
		options := []types.GatewayErrorOptions{types.ErrOptionWithHideErrMsg("上游实时连接暂时不可用。")}
		if ctx.Err() != nil {
			err = ctx.Err()
			options = append(options, types.ErrOptionWithSkipRetry())
		}
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeDoRequestFailed, status, options...)
	}
	socket.SetReadLimit(32<<20 + 8192)
	idle := 5 * time.Minute
	if seconds := constant.StreamingTimeout; seconds > 0 && int64(seconds) <= math.MaxInt64/int64(time.Second) {
		idle = time.Duration(seconds) * time.Second
	}
	owner := &NativeSocketSession{Conn: socket, ctx: ctx, idle: idle, callbackDone: make(chan struct{})}
	owner.stop = context.AfterFunc(ctx, func() {
		owner.closeSocket()
		close(owner.callbackDone)
	})
	owner.unregister = wsmanager.Register(info.ChannelId, kind, func(string) { owner.closeSocket() })
	return owner, nil
}

func (owner *NativeSocketSession) closeSocket() {
	owner.socketOnce.Do(func() { owner.closeError = owner.Conn.Close() })
}

func (owner *NativeSocketSession) Close() error {
	if owner == nil {
		return nil
	}
	owner.ownerOnce.Do(func() {
		owner.closeSocket()
		if !owner.stop() {
			<-owner.callbackDone
		}
		owner.unregister()
	})
	return owner.closeError
}

func (owner *NativeSocketSession) Read() (int, []byte, error) {
	if err := owner.ctx.Err(); err != nil {
		return 0, nil, err
	}
	deadline := time.Now().Add(owner.idle)
	if parent, present := owner.ctx.Deadline(); present && parent.Before(deadline) {
		deadline = parent
	}
	if err := owner.Conn.SetReadDeadline(deadline); err != nil {
		return 0, nil, err
	}
	kind, data, err := owner.Conn.ReadMessage()
	if owner.ctx.Err() != nil {
		err = owner.ctx.Err()
	}
	return kind, data, err
}

func (owner *NativeSocketSession) Write(kind int, data []byte) error {
	if err := owner.ctx.Err(); err != nil {
		return err
	}
	deadline := time.Now().Add(30 * time.Second)
	if parent, present := owner.ctx.Deadline(); present && parent.Before(deadline) {
		deadline = parent
	}
	if err := owner.Conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	return owner.Conn.WriteMessage(kind, data)
}
