package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/model"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

type relayRetryBoundary struct {
	remaining     int
	canceled      bool
	committed     bool
	strictSession bool
	sessionSource string
	pinned        bool
}

func retryDecision(action, reason, source string) PolicyDecision {
	return PolicyDecision{Action: action, Reason: reason, Source: source}
}

// A retry can only start while its caller is waiting for the first response.
// These lifecycle limits also apply to locally generated channel errors.
func decideRetryWithinBoundary(err *types.GatewayError, boundary relayRetryBoundary) PolicyDecision {
	if err == nil {
		return retryDecision("stop", "request_completed", "system")
	}
	if boundary.canceled || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return retryDecision("stop", "request_canceled", "request")
	}
	if boundary.committed {
		return retryDecision("stop", "response_committed", "request")
	}
	if boundary.strictSession {
		return retryDecision("stop", "strict_session", boundary.sessionSource)
	}
	if boundary.pinned {
		return retryDecision("stop", "pinned_channel", "channel_constraint")
	}
	if types.IsSkipRetryError(err) {
		return retryDecision("stop", "non_retryable_error", "system")
	}
	if boundary.remaining <= 0 {
		return retryDecision("stop", "attempt_budget_exhausted", "global")
	}
	status := err.StatusCode
	if status >= http.StatusOK && status < http.StatusMultipleChoices ||
		operation_setting.IsAlwaysSkipRetryCode(err.GetErrorCode()) ||
		operation_setting.IsAlwaysSkipRetryStatusCode(status) {
		return retryDecision("stop", "system_retry_exclusion", "system")
	}
	if types.IsChannelError(err) {
		return retryDecision("retry", "channel_error", "system")
	}
	if status < 100 || status > 599 {
		return retryDecision("retry", "unrecognized_status", "system")
	}
	if operation_setting.ShouldRetryByStatusCode(status) {
		return retryDecision("retry", "retry_status_matched", "global")
	}
	return retryDecision("stop", "status_not_retryable", "global")
}

// Snapshot request state at the boundary; the policy itself has no Gin writes.
func DecideRelayRetry(c *gin.Context, err *types.GatewayError, retryTimes int) PolicyDecision {
	boundary := relayRetryBoundary{remaining: retryTimes, sessionSource: "session_rule"}
	if c != nil {
		boundary.canceled = c.Request != nil && c.Request.Context().Err() != nil
		boundary.committed = c.Writer != nil && c.Writer.Written()
		boundary.strictSession = ShouldSkipRetryAfterChannelAffinityFailure(c)
		boundary.pinned = GetChannelConstraints(c).SuppressesRetry()
		if boundary.strictSession {
			if source := RequestPolicy(c).SessionModeSource; source != "" {
				boundary.sessionSource = source
			}
		}
	}
	return decideRetryWithinBoundary(err, boundary)
}

func ShouldRetryRelayError(c *gin.Context, gatewayErr *types.GatewayError, retryTimes int) bool {
	return DecideRelayRetry(c, gatewayErr, retryTimes).Action == "retry"
}

func ProcessChannelError(c *gin.Context, channelError types.ChannelError, err *types.GatewayError, relayInfo *relaycommon.RelayInfo) {
	if err == nil || c == nil {
		return
	}
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, common.LocalLogPreview(err.MaskSensitiveErrorWithStatusCode())))
	if ShouldDisableChannel(err) && channelError.AutoBan {
		reason := err.MaskSensitiveErrorWithStatusCode()
		gopool.Go(func() {
			DisableChannel(channelError, reason)
		})
	}

	if constant.ErrorLogEnabled && types.IsRecordErrorLog(err) {
		userId := c.GetInt("id")
		tokenName := c.GetString("token_name")
		modelName := c.GetString("original_model")
		tokenId := c.GetInt("token_id")
		userGroup := c.GetString("group")
		other := model.NewLogOther()
		if c.Request != nil && c.Request.URL != nil {
			other.SetPublic("request_path", c.Request.URL.Path)
		}
		other.SetPublic("error_type", err.GetErrorType())
		other.SetPublic("error_code", err.GetErrorCode())
		other.SetPublic("status_code", err.StatusCode)
		AppendRelayLogAdminInfo(c, relayInfo, other)
		AppendResponseModelLogInfo(relayInfo, other)
		AppendTaskPluginContextAuditInfo(c, other)
		startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
		if startTime.IsZero() {
			startTime = time.Now()
		}
		useTimeSeconds := int(time.Since(startTime).Seconds())
		model.RecordErrorLog(c, userId, channelError.ChannelId, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(), tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), userGroup, other)
	}
}
