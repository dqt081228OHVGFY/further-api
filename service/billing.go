package service

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/logger"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

const (
	BillingSourceWallet       = "wallet"
	BillingSourceSubscription = "subscription"
)

func PreConsumeBilling(c *gin.Context, amount int, info *relaycommon.RelayInfo) *types.GatewayError {
	if info == nil {
		return types.NewErrorWithStatusCode(errors.New("计费请求上下文无效。"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if info.QuotaClamp != nil {
		return types.NewErrorWithStatusCode(info.QuotaClamp, types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if !validFundingAmount(amount) {
		return types.NewErrorWithStatusCode(ErrFundingAmountInvalid, types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	session, failure := NewBillingSession(c, info, amount)
	if failure == nil {
		info.Billing = session
	}
	return failure
}

func SettleBilling(c *gin.Context, info *relaycommon.RelayInfo, actual int) error {
	if info == nil || !validFundingAmount(actual) {
		return ErrFundingAmountInvalid
	}
	reserved := info.FinalPreConsumedQuota
	if info.Billing == nil {
		if delta := actual - reserved; delta != 0 {
			return PostConsumeQuota(info, delta, reserved, true)
		}
		return nil
	}
	reserved = info.Billing.GetPreConsumedQuota()
	if err := info.Billing.Settle(actual); err != nil {
		return err
	}
	if session, ok := info.Billing.(*BillingSession); ok {
		session.mu.Lock()
		if session.notificationPublished {
			session.mu.Unlock()
			return nil
		}
		session.notificationPublished = true
		if session.furtherFinalQuota != nil {
			actual = *session.furtherFinalQuota
		}
		session.mu.Unlock()
	}
	logger.LogInfo(c, fmt.Sprintf("计费结算：预留 %s，实际扣费 %s", logger.FormatQuota(reserved), logger.FormatQuota(actual)))
	if actual > 0 {
		if info.BillingSource == BillingSourceSubscription {
			checkAndSendSubscriptionQuotaNotify(info)
		} else {
			checkAndSendQuotaNotify(info, actual-reserved, reserved)
		}
	}
	return nil
}

func SettleBillingWithFinalQuota(c *gin.Context, info *relaycommon.RelayInfo, actual int) (int, error) {
	err := SettleBilling(c, info, actual)
	if info != nil {
		if session, ok := info.Billing.(*BillingSession); ok {
			if charged, known := session.FinalQuota(); known {
				return charged, err
			}
		}
	}
	return actual, err
}

func BillingReservationCovers(info *relaycommon.RelayInfo, target int) bool {
	if info == nil || info.Billing == nil || target < 0 {
		return false
	}
	if reservation, ok := info.Billing.(interface{ ReservedTargetQuota() int }); ok {
		return reservation.ReservedTargetQuota() >= target
	}
	return info.Billing.GetPreConsumedQuota() >= target
}
