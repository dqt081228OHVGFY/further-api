package service

import (
	"context"
	"fmt"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
)

const authArtifactCleanupInterval = time.Hour

// StartAuthArtifactCleanup removes expired dashboard Sessions and old
// one-time authentication flows. Only the master instance performs cleanup.
func StartAuthArtifactCleanup() {
	if !common.IsMasterNode {
		return
	}
	startPeriodicTask("authentication artifact cleanup", authArtifactCleanupInterval, true, func(ctx context.Context) {
		cleanupAuthArtifactsContext(ctx)
	})
}

func cleanupAuthArtifacts() {
	cleanupAuthArtifactsContext(context.Background())
}

func cleanupAuthArtifactsContext(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	now := time.Now()
	count, err := model.CountUserSessionsCreatedSinceContext(ctx, 0, now.Add(-time.Hour).Unix())
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		common.SysError("failed to count hourly user session issuance: " + err.Error())
	} else if count > int64(common.UserSessionHourlyAlertThreshold) {
		common.SysError(fmt.Sprintf(
			"hourly user session issuance exceeded alert threshold: count=%d threshold=%d window_seconds=%d",
			count,
			common.UserSessionHourlyAlertThreshold,
			int64(time.Hour/time.Second),
		))
	}
	if err := model.DeleteExpiredUserSessionsContext(ctx, now.Unix()); err != nil && ctx.Err() == nil {
		common.SysError("failed to delete expired user sessions: " + err.Error())
	}
	if err := model.DeleteOldRevokedUserSessionsContext(ctx, now.Unix()); err != nil && ctx.Err() == nil {
		common.SysError("failed to delete old revoked user sessions: " + err.Error())
	}
	if err := model.DeleteExpiredAuthFlowsContext(ctx, now); err != nil && ctx.Err() == nil {
		common.SysError("failed to delete expired authentication flows: " + err.Error())
	}
}
