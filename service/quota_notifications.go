package service

import (
	"context"
	"fmt"
	"html"
	"math"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/model"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
)

type quotaNotificationSnapshot struct {
	userID       int
	email        string
	settings     dto.UserSetting
	remaining    int64
	subscription bool
	link         string
}

func quotaWarningThreshold(settings dto.UserSetting) int64 {
	value := settings.QuotaWarningThreshold
	if value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0) && value <= float64(common.MaxWalletQuota) {
		return int64(value)
	}
	return int64(common.QuotaRemindThreshold)
}

func checkAndSendQuotaNotify(info *relaycommon.RelayInfo, delta, reserved int) {
	if info == nil {
		return
	}
	snapshot := quotaNotificationSnapshot{userID: info.UserId, email: info.UserEmail, settings: info.UserSetting,
		remaining: int64(info.UserQuota) - int64(delta) - int64(reserved), link: PaymentReturnURL("/wallet")}
	queueQuotaNotification(snapshot)
}

func checkAndSendSubscriptionQuotaNotify(info *relaycommon.RelayInfo) {
	if info == nil || info.SubscriptionId <= 0 || info.SubscriptionAmountTotal <= 0 {
		return
	}
	snapshot := quotaNotificationSnapshot{userID: info.UserId, email: info.UserEmail, settings: info.UserSetting, subscription: true,
		remaining: info.SubscriptionAmountTotal - info.SubscriptionAmountUsedAfterPreConsume - info.SubscriptionPostDelta, link: PaymentReturnURL("/wallet")}
	queueQuotaNotification(snapshot)
}

// All facts are captured on the request goroutine. The tracked worker retains
// neither Gin state nor a mutable billing session, and exits during shutdown.
func queueQuotaNotification(snapshot quotaNotificationSnapshot) {
	if snapshot.userID <= 0 || snapshot.remaining >= quotaWarningThreshold(snapshot.settings) {
		return
	}
	common.StartBackgroundTask("余额预警通知", func(ctx context.Context) {
		if !snapshot.subscription {
			remaining, err := model.GetUserQuotaContext(ctx, snapshot.userID, true)
			if err != nil {
				common.SysError(fmt.Sprintf("读取余额预警额度失败（user=%d）：%s", snapshot.userID, err))
				return
			}
			snapshot.remaining = int64(remaining)
		}
		if snapshot.remaining >= quotaWarningThreshold(snapshot.settings) || ctx.Err() != nil {
			return
		}
		title := "您的额度即将用尽"
		if snapshot.subscription {
			title = "您的订阅额度即将用尽"
		}
		amount := logger.FormatQuota(int(max(0, snapshot.remaining)))
		content := "{{value}}，当前剩余额度为 {{value}}，请及时充值。"
		values := []any{title, amount}
		if snapshot.settings.NotifyType != dto.NotifyTypeBark && snapshot.settings.NotifyType != dto.NotifyTypeGotify {
			content += "<br/>充值链接：<a href='{{value}}'>{{value}}</a>"
			values = append(values, html.EscapeString(snapshot.link), html.EscapeString(snapshot.link))
		}
		if err := NotifyUserContext(ctx, snapshot.userID, snapshot.email, snapshot.settings, dto.NewNotify(dto.NotifyTypeQuotaExceed, title, content, values)); err != nil {
			common.SysError(fmt.Sprintf("发送余额预警失败（user=%d）：%s", snapshot.userID, err))
		}
	})
}
