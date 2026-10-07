package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

// Settlement commits the funding source once, then the token allowance. The
// cached terminal result prevents retries from charging an already committed
// source, or reporting a prior token-ledger failure as a successful settlement.
func (session *BillingSession) Settle(actualQuota int) error {
	session.mu.Lock()
	defer session.mu.Unlock()
	defer session.releaseFurtherLease()
	if !validFundingAmount(actualQuota) {
		return ErrFundingAmountInvalid
	}
	if session.refunded {
		return ErrFundingFinalized
	}
	if session.settled {
		return session.settlementError
	}
	if session.funding == nil || session.relayInfo == nil {
		return errors.New("计费会话缺少资金来源。")
	}
	if source, ok := session.funding.(*SubscriptionFunding); ok && source.furtherDiscount && !session.fundingSettled {
		charged, err := model.SettleFurtherSubscriptionReservation(source.requestId, int64(actualQuota))
		if err != nil {
			return err
		}
		actualQuota = int(charged)
		session.fundingSettled = true
	}
	delta := actualQuota - session.preConsumedQuota
	if !session.fundingSettled {
		if err := session.funding.Settle(delta); err != nil {
			return err
		}
		session.fundingSettled = true
	}
	session.furtherFinalQuota = common.GetPointer(actualQuota)
	if !session.relayInfo.IsPlayground {
		tokenDelta := actualQuota - session.tokenConsumed
		if tokenDelta > 0 {
			session.settlementError = model.DecreaseTokenQuota(session.relayInfo.TokenId, session.relayInfo.TokenKey, tokenDelta)
		} else if tokenDelta < 0 {
			session.settlementError = model.IncreaseTokenQuota(session.relayInfo.TokenId, session.relayInfo.TokenKey, -tokenDelta)
		}
	}
	if session.funding.Source() == BillingSourceSubscription {
		session.relayInfo.SubscriptionPostDelta += int64(delta)
	}
	session.settled = true
	if session.settlementError != nil {
		common.SysError(fmt.Sprintf("token accounting failed after funding commit (user=%d, token=%d): %s", session.relayInfo.UserId, session.relayInfo.TokenId, session.settlementError))
	}
	return session.settlementError
}

func (session *BillingSession) needsRefundLocked() bool {
	if session.settled || session.refunded || session.fundingSettled || session.funding == nil {
		return false
	}
	if session.tokenConsumed > 0 || session.preConsumedQuota > 0 {
		return true
	}
	if subscription, ok := session.funding.(*SubscriptionFunding); ok {
		return subscription.subscriptionId > 0
	}
	return false
}

func (session *BillingSession) NeedsRefund() bool {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.needsRefundLocked()
}

// A failed request becomes terminal before its refund is scheduled. Its
// accounting facts are captured while locked; no Gin or mutable relay state
// is retained by the worker. Accepted refunds participate in server draining.
func (session *BillingSession) Refund(c *gin.Context) {
	session.mu.Lock()
	if session.furtherLease != nil {
		session.furtherLease.Release()
	}
	if session.settled || session.refunded || session.fundingSettled {
		session.mu.Unlock()
		return
	}
	needed := session.needsRefundLocked()
	session.refunded = true
	if !needed || session.relayInfo == nil {
		session.mu.Unlock()
		return
	}
	funding := session.funding
	userID, tokenID, tokenKey := session.relayInfo.UserId, session.relayInfo.TokenId, session.relayInfo.TokenKey
	playground, tokenReserved := session.relayInfo.IsPlayground, session.tokenConsumed
	extra, subscriptionID := session.extraReserved, session.relayInfo.SubscriptionId
	session.mu.Unlock()
	logger.LogInfo(c, fmt.Sprintf("用户 %d 请求未完成，释放计费预留（funding=%s）", userID, funding.Source()))
	refund := func(context.Context) {
		var failures []error
		if err := funding.Refund(); err != nil {
			failures = append(failures, fmt.Errorf("refund funding: %w", err))
		}
		if extra > 0 && funding.Source() == BillingSourceSubscription && subscriptionID > 0 {
			if err := model.PostConsumeUserSubscriptionDelta(subscriptionID, -int64(extra)); err != nil {
				failures = append(failures, fmt.Errorf("refund supplemental reservation: %w", err))
			}
		}
		if tokenReserved > 0 && !playground {
			if err := model.IncreaseTokenQuota(tokenID, tokenKey, tokenReserved); err != nil {
				failures = append(failures, fmt.Errorf("refund token allowance: %w", err))
			}
		}
		failure := errors.Join(failures...)
		session.mu.Lock()
		session.refundError = failure
		session.mu.Unlock()
		if failure != nil {
			common.SysError(fmt.Sprintf("request accounting refund failed (user=%d, token=%d): %s", userID, tokenID, failure))
		}
	}
	if !common.StartBackgroundTask("request accounting refund", refund) {
		refund(context.Background())
	}
}

func (session *BillingSession) GetPreConsumedQuota() int {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.preConsumedQuota
}

// Subscription discount reservations track the undiscounted target separately
// from their actual debit. Submission barriers compare targets in one unit.
func (session *BillingSession) ReservedTargetQuota() int {
	session.mu.Lock()
	defer session.mu.Unlock()
	if source, ok := session.funding.(*SubscriptionFunding); ok && source.furtherDiscount {
		return int(source.amount)
	}
	return session.preConsumedQuota
}

func (session *BillingSession) FinalQuota() (int, bool) {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.furtherFinalQuota == nil {
		return 0, false
	}
	return *session.furtherFinalQuota, true
}

func (session *BillingSession) Reserve(target int) error {
	session.mu.Lock()
	defer session.mu.Unlock()
	if !validFundingAmount(target) {
		return ErrFundingAmountInvalid
	}
	if session.settled || session.refunded || session.fundingSettled {
		return ErrFundingFinalized
	}
	if session.funding == nil || session.relayInfo == nil {
		return errors.New("计费会话缺少资金来源。")
	}
	if source, ok := session.funding.(*SubscriptionFunding); ok && source.furtherDiscount {
		if int64(target) <= source.amount {
			return nil
		}
		original := source.amount
		charged, err := model.ResizeFurtherSubscriptionReservation(source.requestId, int64(target))
		if err != nil {
			return err
		}
		delta := int(charged) - session.preConsumedQuota
		if err := session.reserveToken(delta); err != nil {
			_, rollback := model.ResizeFurtherSubscriptionReservation(source.requestId, original)
			return errors.Join(err, rollback)
		}
		source.amount, source.preConsumed = int64(target), charged
		source.AmountUsedAfter += int64(delta)
		session.preConsumedQuota, session.tokenConsumed = int(charged), session.tokenConsumed+delta
		session.syncRelayInfo()
		return nil
	}
	_, image := session.relayInfo.Request.(*dto.ImageRequest)
	image = image || session.relayInfo.ImageRequestCount > 0
	if target <= session.preConsumedQuota || session.trusted && !image {
		return nil
	}
	delta := target - session.preConsumedQuota
	if err := session.reserveFunding(delta, image); err != nil {
		return err
	}
	if err := session.reserveToken(delta); err != nil {
		session.rollbackFundingReserve(delta)
		return err
	}
	session.preConsumedQuota += delta
	session.tokenConsumed += delta
	session.extraReserved += delta
	if image {
		session.trusted = false
	}
	session.syncRelayInfo()
	return nil
}
