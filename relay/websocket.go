package relay

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/dqt081228OHVGFY/further-api/pkg/wsmanager"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func WssHelper(c *gin.Context, info *relaycommon.RelayInfo) *types.GatewayError {
	attempt, failure := beginProtocolAttempt(c, info)
	if failure != nil {
		return failure
	}
	if info.ClientWs == nil {
		return types.NewErrorWithStatusCode(errors.New("Realtime客户端连接不存在。"), types.ErrorCodeInvalidRequest, 400, types.ErrOptionWithSkipRetry())
	}
	if failure := attempt.bindAdaptor(); failure != nil {
		return failure
	}
	raw, err := attempt.adaptor.DoRequest(c, info, nil)
	if err != nil {
		return attempt.mapError(types.NewError(err, types.ErrorCodeDoRequestFailed))
	}
	upstream, valid := raw.(*websocket.Conn)
	if !valid || upstream == nil {
		return types.NewErrorWithStatusCode(fmt.Errorf("Realtime上游连接类型无效：%T", raw), types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	info.TargetWs = upstream
	var closeOnce sync.Once
	closePeers := func(code int, reason string) {
		closeOnce.Do(func() {
			deadline := time.Now().Add(time.Second)
			frame := websocket.FormatCloseMessage(code, reason)
			_ = info.ClientWs.WriteControl(websocket.CloseMessage, frame, deadline)
			_ = upstream.WriteControl(websocket.CloseMessage, frame, deadline)
			_ = info.ClientWs.Close()
			_ = upstream.Close()
		})
	}
	unregister := wsmanager.Register(info.ChannelId, wsmanager.KindRealtime, func(reason string) { closePeers(websocket.ClosePolicyViolation, reason) })
	defer unregister()
	canceled := make(chan struct{})
	stop := context.AfterFunc(c.Request.Context(), func() { closePeers(websocket.CloseGoingAway, "request ended"); close(canceled) })
	defer func() {
		closePeers(websocket.CloseNormalClosure, "")
		if !stop() {
			<-canceled
		}
	}()
	usage, failure := attempt.adaptor.DoResponse(c, nil, info)
	return finishRealtimeAttempt(c, info, usage, attempt.mapError(failure))
}

func finishRealtimeAttempt(c *gin.Context, info *relaycommon.RelayInfo, raw any, failure *types.GatewayError) *types.GatewayError {
	var usage *dto.RealtimeUsage
	switch value := raw.(type) {
	case *dto.RealtimeUsage:
		usage = value
	case dto.RealtimeUsage:
		usage = &value
	case nil:
	default:
		if failure == nil {
			failure = types.NewErrorWithStatusCode(fmt.Errorf("Realtime用量类型无效：%T", raw), types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
		}
	}
	if usage == nil {
		if failure != nil {
			return failure
		}
		return types.NewErrorWithStatusCode(errors.New("Realtime未返回用量快照。"), types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	consumed := usage.TotalTokens > 0 || usage.InputTokens > 0 || usage.OutputTokens > 0 || usage.InputTokenDetails.TextTokens > 0 || usage.InputTokenDetails.AudioTokens > 0 || usage.OutputTokenDetails.TextTokens > 0 || usage.OutputTokenDetails.AudioTokens > 0
	if failure == nil || consumed {
		copy := *usage
		copy.InputTokenDetails = usage.InputTokenDetails.Clone()
		if copy.InputTokens == 0 {
			copy.InputTokens = copy.InputTokenDetails.TextTokens + copy.InputTokenDetails.AudioTokens
		}
		if copy.OutputTokens == 0 {
			copy.OutputTokens = copy.OutputTokenDetails.TextTokens + copy.OutputTokenDetails.AudioTokens
		}
		if copy.TotalTokens == 0 {
			copy.TotalTokens = copy.InputTokens + copy.OutputTokens
		}
		note := ""
		if failure != nil {
			note = "Realtime连接已中断，保留已发生用量的结算。"
		}
		service.PostWssConsumeQuota(c, info, info.GetBillingModelName(), &copy, note)
	}
	return failure
}
