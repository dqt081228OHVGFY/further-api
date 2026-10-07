package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/model"
)

const (
	codexCredentialRefreshTickInterval = 10 * time.Minute
	codexCredentialRefreshThreshold    = 24 * time.Hour
	codexCredentialRefreshBatchSize    = 200
	codexCredentialRefreshTimeout      = 15 * time.Second
)

var (
	codexCredentialRefreshOnce    sync.Once
	codexCredentialRefreshRunning atomic.Bool
)

func shouldAutoRefreshCodexChannelStatus(status int) bool {
	return status == common.ChannelStatusEnabled || status == common.ChannelStatusAutoDisabled
}

func StartCodexCredentialAutoRefreshTask() {
	codexCredentialRefreshOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}

		logger.LogInfo(common.BackgroundContext(), fmt.Sprintf("codex credential auto-refresh task started: tick=%s threshold=%s", codexCredentialRefreshTickInterval, codexCredentialRefreshThreshold))
		startPeriodicTask("codex credential refresh", codexCredentialRefreshTickInterval, true, func(context.Context) {
			runCodexCredentialAutoRefreshOnce()
		})
	})
}

func runCodexCredentialAutoRefreshOnce() {
	if !codexCredentialRefreshRunning.CompareAndSwap(false, true) {
		return
	}
	defer codexCredentialRefreshRunning.Store(false)

	ctx := common.BackgroundContext()
	now := time.Now()

	var refreshed int
	var scanned int

	offset := 0
	for {
		if ctx.Err() != nil {
			return
		}
		var channels []*model.Channel
		err := model.DB.WithContext(ctx).
			Select("id", "name", "key", "status", "channel_info").
			Where("type = ? AND (status = ? OR status = ?)",
				constant.ChannelTypeCodex,
				common.ChannelStatusEnabled,
				common.ChannelStatusAutoDisabled,
			).
			Order("id asc").
			Limit(codexCredentialRefreshBatchSize).
			Offset(offset).
			Find(&channels).Error
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("codex credential auto-refresh: query channels failed: %v", err))
			return
		}
		if len(channels) == 0 {
			break
		}
		offset += codexCredentialRefreshBatchSize

		for _, ch := range channels {
			if ctx.Err() != nil {
				return
			}
			if ch == nil {
				continue
			}
			scanned++
			if ch.ChannelInfo.IsMultiKey {
				continue
			}

			rawKey := strings.TrimSpace(ch.Key)
			if rawKey == "" {
				continue
			}

			oauthKey, err := parseCodexOAuthKey(rawKey)
			if err != nil {
				continue
			}

			refreshToken := strings.TrimSpace(oauthKey.RefreshToken)
			if refreshToken == "" {
				continue
			}

			expiredAtRaw := strings.TrimSpace(oauthKey.Expired)
			expiredAt, err := time.Parse(time.RFC3339, expiredAtRaw)
			if err == nil && !expiredAt.IsZero() && expiredAt.Sub(now) > codexCredentialRefreshThreshold {
				continue
			}

			refreshCtx, cancel := context.WithTimeout(ctx, codexCredentialRefreshTimeout)
			newKey, _, err := RefreshCodexChannelCredential(refreshCtx, ch.Id, CodexCredentialRefreshOptions{ResetCaches: false})
			cancel()
			if err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("codex credential auto-refresh: channel_id=%d name=%s refresh failed: %v", ch.Id, ch.Name, err))
				continue
			}

			refreshed++
			logger.LogInfo(ctx, fmt.Sprintf("codex credential auto-refresh: channel_id=%d name=%s refreshed, expires_at=%s", ch.Id, ch.Name, newKey.Expired))
		}
	}

	if refreshed > 0 {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.LogWarn(ctx, fmt.Sprintf("codex credential auto-refresh: InitChannelCache panic: %v", r))
				}
			}()
			model.InitChannelCache()
		}()
	}

	if common.DebugEnabled {
		logger.LogDebug(ctx, "codex credential auto-refresh: scanned=%d refreshed=%d", scanned, refreshed)
	}
}
