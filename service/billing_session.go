package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/model"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

type BillingSession struct {
	relayInfo             *relaycommon.RelayInfo
	funding               FundingSource
	preConsumedQuota      int
	tokenConsumed         int
	extraReserved         int
	trusted               bool
	fundingSettled        bool
	settled               bool
	refunded              bool
	settlementError       error
	refundError           error
	mu                    sync.Mutex
	furtherLease          *FurtherLimitLease
	furtherFinalQuota     *int
	furtherLeaseDeferred  bool
	realtimeMu            sync.Mutex
	realtimeBaseQuota     int
	requestContext        context.Context
	usagePublished        bool
	notificationPublished bool
}

func admissionContext(c *gin.Context) context.Context {
	if c != nil && c.Request != nil {
		return c.Request.Context()
	}
	return context.Background()
}

func billingFailure(err error, code types.ErrorCode, status int) *types.GatewayError {
	options := []types.GatewayErrorOptions{types.ErrOptionWithSkipRetry()}
	if code == types.ErrorCodeInsufficientUserQuota || code == types.ErrorCodePreConsumeTokenQuotaFailed {
		options = append(options, types.ErrOptionWithNoRecordErrorLog())
	}
	return types.NewErrorWithStatusCode(err, code, status, options...)
}

func fundingAdmissionFailure(err error) *types.GatewayError {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return billingFailure(err, types.ErrorCodeDoRequestFailed, http.StatusRequestTimeout)
	case errors.Is(err, ErrInsufficientWalletQuota), errors.Is(err, model.ErrNoActiveSubscription), errors.Is(err, model.ErrSubscriptionQuota):
		return billingFailure(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden)
	default:
		return billingFailure(err, types.ErrorCodeUpdateDataError, http.StatusServiceUnavailable)
	}
}

func compensationContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Second)
}

func reserveTokenAllowance(ctx context.Context, info *relaycommon.RelayInfo, quota int) error {
	if !validFundingAmount(quota) {
		return ErrFundingAmountInvalid
	}
	if info.IsPlayground || quota == 0 {
		return nil
	}
	reserved, err := model.TryReserveTokenQuotaContext(ctx, info.TokenId, info.TokenKey, quota, info.TokenUnlimited)
	if err != nil {
		return err
	}
	if !reserved {
		return billingFailure(errors.New("访问令牌剩余额度不足。"), types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden)
	}
	return nil
}

func (session *BillingSession) shouldTrust(c *gin.Context) bool {
	if session.relayInfo == nil || session.funding == nil || session.relayInfo.ForcePreConsume || session.funding.Source() != BillingSourceWallet {
		return false
	}
	threshold := operation_setting.GetQuotaSetting().TrustQuotaUSD * common.QuotaPerUnit
	if threshold <= 0 || math.IsNaN(threshold) || math.IsInf(threshold, 0) {
		return false
	}
	tokenTrusted := session.relayInfo.TokenUnlimited
	if !tokenTrusted && c != nil {
		tokenTrusted = float64(c.GetInt("token_quota")) > threshold
	}
	return tokenTrusted && float64(session.relayInfo.UserQuota) > threshold
}

func (session *BillingSession) syncRelayInfo() {
	info := session.relayInfo
	info.FinalPreConsumedQuota, info.BillingSource = session.preConsumedQuota, session.funding.Source()
	if source, ok := session.funding.(*SubscriptionFunding); ok {
		info.SubscriptionId = source.subscriptionId
		info.SubscriptionPreConsumed = source.preConsumed + int64(session.extraReserved)
		info.SubscriptionPostDelta = 0
		info.SubscriptionAmountTotal = source.AmountTotal
		info.SubscriptionAmountUsedAfterPreConsume = source.AmountUsedAfter + int64(session.extraReserved)
		info.SubscriptionPlanId, info.SubscriptionPlanTitle = source.PlanId, source.PlanTitle
	} else {
		info.SubscriptionId, info.SubscriptionPreConsumed, info.SubscriptionPostDelta = 0, 0, 0
		info.SubscriptionAmountTotal, info.SubscriptionAmountUsedAfterPreConsume = 0, 0
		info.SubscriptionPlanId, info.SubscriptionPlanTitle = 0, ""
	}
}

func (session *BillingSession) rollbackAdmission() error {
	ctx, cancel := compensationContext()
	defer cancel()
	var failures []error
	if session.funding != nil {
		if err := session.funding.Refund(); err != nil {
			failures = append(failures, err)
		}
	}
	if session.tokenConsumed > 0 && !session.relayInfo.IsPlayground {
		if err := model.IncreaseTokenQuotaContext(ctx, session.relayInfo.TokenId, session.relayInfo.TokenKey, session.tokenConsumed); err != nil {
			failures = append(failures, err)
		}
	}
	session.tokenConsumed, session.preConsumedQuota = 0, 0
	if session.furtherLease != nil {
		session.furtherLease.Release()
	}
	return errors.Join(failures...)
}

func (session *BillingSession) admitWallet(c *gin.Context, ctx context.Context, amount int) *types.GatewayError {
	source := session.funding.(*WalletFunding)
	session.trusted = session.shouldTrust(c)
	reserve := amount
	if session.trusted {
		reserve = 0
	}
	if err := reserveTokenAllowance(ctx, session.relayInfo, reserve); err != nil {
		return billingFailure(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden)
	}
	session.tokenConsumed = reserve
	if err := source.PreConsume(reserve); err != nil {
		rollback := session.rollbackAdmission()
		if rollback != nil {
			return billingFailure(errors.Join(err, rollback), types.ErrorCodeUpdateDataError, http.StatusServiceUnavailable)
		}
		return fundingAdmissionFailure(err)
	}
	session.preConsumedQuota = reserve
	if err := ctx.Err(); err != nil {
		return billingFailure(errors.Join(err, session.rollbackAdmission()), types.ErrorCodeDoRequestFailed, http.StatusRequestTimeout)
	}
	session.syncRelayInfo()
	return nil
}

func (session *BillingSession) admitSubscription(c *gin.Context, ctx context.Context) *types.GatewayError {
	source := session.funding.(*SubscriptionFunding)
	if err := source.PreConsume(int(source.amount)); err != nil {
		return fundingAdmissionFailure(err)
	}
	if session.relayInfo.ForcePreConsume && source.furtherDiscount {
		reservation, err := model.GetFurtherDiscountReservationContext(ctx, source.requestId)
		if err != nil || reservation != nil && reservation.Multiplier != 1 {
			rollback := session.rollbackAdmission()
			if rollback != nil {
				return billingFailure(errors.Join(err, rollback), types.ErrorCodeUpdateDataError, http.StatusServiceUnavailable)
			}
			if err != nil {
				return fundingAdmissionFailure(err)
			}
			return billingFailure(errors.New("当前套餐折扣暂不支持异步任务计费，请使用钱包计费或无折扣套餐。"), types.ErrorCodeInvalidRequest, http.StatusBadRequest)
		}
	}
	charged := int(source.preConsumed)
	if err := reserveTokenAllowance(ctx, session.relayInfo, charged); err != nil {
		rollback := session.rollbackAdmission()
		return billingFailure(errors.Join(err, rollback), types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden)
	}
	session.tokenConsumed, session.preConsumedQuota = charged, charged
	session.syncRelayInfo()
	lease, err := AcquireFurtherSubscriptionLimits(session.requestContext, session.relayInfo.UserId, source.subscriptionId)
	if err != nil {
		rollback := session.rollbackAdmission()
		status := http.StatusServiceUnavailable
		var limit *FurtherLimitError
		if errors.As(err, &limit) && (limit.Reason == "rpm" || limit.Reason == "concurrency") {
			status = http.StatusTooManyRequests
			if c != nil {
				c.Header("Retry-After", strconv.Itoa(max(1, limit.RetryAfter)))
			}
		}
		return billingFailure(errors.Join(err, rollback), types.ErrorCodeInvalidRequest, status)
	}
	session.furtherLease = lease
	if err := ctx.Err(); err != nil {
		return billingFailure(errors.Join(err, session.rollbackAdmission()), types.ErrorCodeDoRequestFailed, http.StatusRequestTimeout)
	}
	if c != nil && c.Request != nil {
		if c.GetBool("further_stats_active") && !strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
			callbacks, _ := c.Get("further_request_end_callbacks")
			items, _ := callbacks.([]func())
			c.Set("further_request_end_callbacks", append(items, lease.Release))
			session.furtherLeaseDeferred = true
		}
		c.Request = c.Request.WithContext(lease.Context())
	}
	session.requestContext = lease.Context()
	return nil
}

type billingFundingCandidate string

const walletCandidate billingFundingCandidate = "wallet"
const subscriptionCandidate billingFundingCandidate = "subscription"

func billingFundingSequence(ctx context.Context, info *relaycommon.RelayInfo) ([]billingFundingCandidate, bool, *types.GatewayError) {
	switch common.NormalizeBillingPreference(info.UserSetting.BillingPreference) {
	case "wallet_only":
		return []billingFundingCandidate{walletCandidate}, false, nil
	case "subscription_only":
		return []billingFundingCandidate{subscriptionCandidate}, false, nil
	case "wallet_first":
		return []billingFundingCandidate{walletCandidate, subscriptionCandidate}, false, nil
	default:
		active, err := model.HasActiveUserSubscriptionContext(ctx, info.UserId)
		if err != nil {
			return nil, false, fundingAdmissionFailure(err)
		}
		if !active {
			return []billingFundingCandidate{walletCandidate}, false, nil
		}
		return []billingFundingCandidate{subscriptionCandidate, walletCandidate}, true, nil
	}
}

// Funding candidates are explicit and fallback is restricted to capacity
// failures. Cancellation, database errors and token failures never change source.
func NewBillingSession(c *gin.Context, info *relaycommon.RelayInfo, amount int) (*BillingSession, *types.GatewayError) {
	if info == nil {
		return nil, billingFailure(errors.New("计费请求信息缺失。"), types.ErrorCodeInvalidRequest, http.StatusBadRequest)
	}
	if !validFundingAmount(amount) {
		return nil, billingFailure(ErrFundingAmountInvalid, types.ErrorCodeModelPriceError, http.StatusBadRequest)
	}
	parent := admissionContext(c)
	if err := parent.Err(); err != nil {
		return nil, fundingAdmissionFailure(err)
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	allowed, err := model.FurtherSubscriptionModelAllowedContext(ctx, info.UserId, info.OriginModelName)
	if err != nil {
		return nil, fundingAdmissionFailure(err)
	}
	if !allowed {
		return nil, billingFailure(errors.New("此模型需要有效的套餐授权。"), types.ErrorCodeAccessDenied, http.StatusForbidden)
	}
	candidates, overflowCheck, failure := billingFundingSequence(ctx, info)
	if failure != nil {
		return nil, failure
	}
	for index, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, fundingAdmissionFailure(err)
		}
		session := &BillingSession{relayInfo: info, requestContext: parent}
		if candidate == walletCandidate {
			balance, err := model.GetUserQuotaContext(ctx, info.UserId, false)
			if err != nil {
				return nil, fundingAdmissionFailure(err)
			}
			if balance <= 0 || balance < amount {
				failure = billingFailure(fmt.Errorf("钱包额度不足，剩余 %s，需要 %s。", logger.FormatQuota(balance), logger.FormatQuota(amount)), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden)
			} else {
				info.UserQuota = balance
				session.funding = &WalletFunding{userId: info.UserId, requestContext: ctx}
				failure = session.admitWallet(c, ctx, amount)
			}
		} else {
			session.funding = &SubscriptionFunding{requestId: info.RequestId, userId: info.UserId, modelName: info.OriginModelName, amount: int64(max(1, amount)), requestContext: ctx}
			failure = session.admitSubscription(c, ctx)
		}
		if failure == nil {
			switch source := session.funding.(type) {
			case *WalletFunding:
				source.requestContext = session.requestContext
			case *SubscriptionFunding:
				source.requestContext = session.requestContext
			}
			return session, nil
		}
		if failure.GetErrorCode() != types.ErrorCodeInsufficientUserQuota || index == len(candidates)-1 {
			return nil, failure
		}
		if candidate == subscriptionCandidate && overflowCheck {
			allow, err := model.UserActiveSubscriptionsAllowWalletOverflowContext(ctx, info.UserId)
			if err != nil {
				return nil, fundingAdmissionFailure(err)
			}
			if !allow {
				return nil, failure
			}
		}
	}
	return nil, failure
}

func (session *BillingSession) reserveFunding(amount int, strict bool) error {
	switch source := session.funding.(type) {
	case *WalletFunding:
		if err := source.reserve(amount, strict); err != nil {
			return fundingAdmissionFailure(err)
		}
	case *SubscriptionFunding:
		if err := source.reserveExtra(amount); err != nil {
			return fundingAdmissionFailure(err)
		}
	default:
		return errors.New("计费资金来源无效。")
	}
	return nil
}

func (session *BillingSession) rollbackFundingReserve(amount int) {
	var err error
	switch source := session.funding.(type) {
	case *WalletFunding:
		err = source.rollbackReserve(amount)
	case *SubscriptionFunding:
		err = source.rollbackExtra(amount)
	}
	if err != nil {
		common.SysError("补充计费预留回滚失败：" + err.Error())
	}
}

func (session *BillingSession) reserveToken(amount int) error {
	ctx := session.requestContext
	if ctx == nil {
		ctx = context.Background()
	}
	if err := reserveTokenAllowance(ctx, session.relayInfo, amount); err != nil {
		return billingFailure(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden)
	}
	return nil
}

func (session *BillingSession) reserveRealtimeUsage(amount int) error {
	session.realtimeMu.Lock()
	defer session.realtimeMu.Unlock()
	if amount < 0 || amount > common.MaxWalletQuota-session.realtimeBaseQuota {
		return ErrFundingAmountInvalid
	}
	target := session.realtimeBaseQuota + amount
	session.mu.Lock()
	session.trusted = false
	session.mu.Unlock()
	if err := session.Reserve(target); err != nil {
		return err
	}
	session.realtimeBaseQuota = target
	return nil
}

func (session *BillingSession) releaseFurtherLease() {
	if session.furtherLease != nil && !session.furtherLeaseDeferred {
		session.furtherLease.Release()
	}
}
