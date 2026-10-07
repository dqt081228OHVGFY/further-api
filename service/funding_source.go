package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
)

type FundingSource interface {
	Source() string
	PreConsume(amount int) error
	Settle(delta int) error
	Refund() error
}

var (
	ErrInsufficientWalletQuota = errors.New("wallet quota insufficient")
	ErrFundingFinalized        = errors.New("计费预留已结束。")
	ErrFundingAmountInvalid    = errors.New("计费额度超出有效范围。")
)

type WalletFunding struct {
	userId         int
	consumed       int
	mu             sync.Mutex
	settled        bool
	refunded       bool
	refundError    error
	requestContext context.Context
}

func (*WalletFunding) Source() string { return BillingSourceWallet }

func validFundingAmount(amount int) bool {
	return amount >= 0 && amount <= common.MaxWalletQuota
}

// Wallet reservation keeps a cumulative total across provider retries and
// quantity overrides. Each successful debit is part of the same refund.
func (wallet *WalletFunding) reserve(amount int, strict bool) error {
	wallet.mu.Lock()
	defer wallet.mu.Unlock()
	if wallet.settled || wallet.refunded {
		return ErrFundingFinalized
	}
	if !validFundingAmount(amount) || amount > common.MaxWalletQuota-wallet.consumed {
		return ErrFundingAmountInvalid
	}
	if amount == 0 {
		return nil
	}
	if strict {
		reserved, err := model.TryReserveUserQuotaContext(fundingOperationContext(wallet.requestContext), wallet.userId, amount)
		if err != nil {
			return err
		}
		if !reserved {
			return ErrInsufficientWalletQuota
		}
	} else if err := model.DecreaseUserQuotaContext(fundingOperationContext(wallet.requestContext), wallet.userId, amount, false); err != nil {
		return err
	}
	wallet.consumed += amount
	return nil
}

func (wallet *WalletFunding) PreConsume(amount int) error { return wallet.reserve(amount, true) }

func (wallet *WalletFunding) rollbackReserve(amount int) error {
	compensate, cancel := compensationContext()
	defer cancel()
	wallet.mu.Lock()
	defer wallet.mu.Unlock()
	if wallet.settled || wallet.refunded {
		return ErrFundingFinalized
	}
	if amount < 0 || amount > wallet.consumed {
		return ErrFundingAmountInvalid
	}
	if amount == 0 {
		return nil
	}
	if err := model.IncreaseUserQuotaContext(compensate, wallet.userId, amount, false); err != nil {
		return err
	}
	wallet.consumed -= amount
	return nil
}

func (wallet *WalletFunding) Settle(delta int) error {
	commit, cancel := compensationContext()
	defer cancel()
	wallet.mu.Lock()
	defer wallet.mu.Unlock()
	if wallet.refunded {
		return ErrFundingFinalized
	}
	if wallet.settled {
		return nil
	}
	if delta < -wallet.consumed || delta > common.MaxWalletQuota-wallet.consumed {
		return ErrFundingAmountInvalid
	}
	var err error
	if delta > 0 {
		err = model.DecreaseUserQuotaContext(commit, wallet.userId, delta, false)
	}
	if delta < 0 {
		err = model.IncreaseUserQuotaContext(commit, wallet.userId, -delta, false)
	}
	if err != nil {
		return err
	}
	wallet.consumed += delta
	wallet.settled = true
	return nil
}

// Wallet credits lack a persisted request-key idempotency record. Execute one
// credit operation and retain its result; repeating Refund never credits twice.
func (wallet *WalletFunding) Refund() error {
	compensate, cancel := compensationContext()
	defer cancel()
	wallet.mu.Lock()
	defer wallet.mu.Unlock()
	if wallet.settled {
		return nil
	}
	if wallet.refunded {
		return wallet.refundError
	}
	wallet.refunded = true
	if wallet.consumed > 0 {
		wallet.refundError = model.IncreaseUserQuotaContext(compensate, wallet.userId, wallet.consumed, false)
	}
	return wallet.refundError
}

type SubscriptionFunding struct {
	requestId       string
	userId          int
	modelName       string
	amount          int64
	subscriptionId  int
	preConsumed     int64
	AmountTotal     int64
	AmountUsedAfter int64
	PlanId          int
	PlanTitle       string
	furtherDiscount bool
	reservedExtra   int64
	requestContext  context.Context
	mu              sync.Mutex
	settled         bool
	refunded        bool
}

func (*SubscriptionFunding) Source() string { return BillingSourceSubscription }

func (subscription *SubscriptionFunding) PreConsume(_ int) error {
	subscription.mu.Lock()
	defer subscription.mu.Unlock()
	if subscription.settled || subscription.refunded {
		return ErrFundingFinalized
	}
	if subscription.subscriptionId > 0 {
		return nil
	}
	if subscription.userId <= 0 || strings.TrimSpace(subscription.requestId) == "" ||
		subscription.amount <= 0 || subscription.amount > int64(common.MaxWalletQuota) {
		return ErrFundingAmountInvalid
	}
	reservation, err := model.PreConsumeUserSubscriptionContext(fundingOperationContext(subscription.requestContext), subscription.requestId, subscription.userId, subscription.modelName, 0, subscription.amount)
	if err != nil {
		return err
	}
	discount, err := model.GetFurtherDiscountReservationContext(fundingOperationContext(subscription.requestContext), subscription.requestId)
	if err != nil {
		compensate, cancel := compensationContext()
		rollback := model.RefundSubscriptionPreConsumeContext(compensate, subscription.requestId)
		cancel()
		return errors.Join(err, rollback)
	}
	subscription.subscriptionId = reservation.UserSubscriptionId
	subscription.preConsumed = reservation.PreConsumed
	subscription.AmountTotal = reservation.AmountTotal
	subscription.AmountUsedAfter = reservation.AmountUsedAfter
	subscription.furtherDiscount = discount != nil
	if plan, err := model.GetSubscriptionPlanInfoByUserSubscriptionIdContext(fundingOperationContext(subscription.requestContext), reservation.UserSubscriptionId); err == nil && plan != nil {
		subscription.PlanId, subscription.PlanTitle = plan.PlanId, plan.PlanTitle
	}
	return nil
}

func (subscription *SubscriptionFunding) Settle(delta int) error {
	commit, cancel := compensationContext()
	defer cancel()
	subscription.mu.Lock()
	defer subscription.mu.Unlock()
	if subscription.refunded {
		return ErrFundingFinalized
	}
	if subscription.settled {
		return nil
	}
	reserved := subscription.preConsumed + subscription.reservedExtra
	if int64(delta) < -reserved || int64(delta) > int64(common.MaxWalletQuota)-reserved {
		return ErrFundingAmountInvalid
	}
	if delta != 0 {
		if err := model.PostConsumeUserSubscriptionDeltaContext(commit, subscription.subscriptionId, int64(delta)); err != nil {
			return err
		}
	}
	subscription.settled = true
	return nil
}

func (subscription *SubscriptionFunding) reserveExtra(amount int) error {
	subscription.mu.Lock()
	defer subscription.mu.Unlock()
	if subscription.refunded || subscription.settled {
		return ErrFundingFinalized
	}
	reserved := subscription.preConsumed + subscription.reservedExtra
	if amount < 0 || int64(amount) > int64(common.MaxWalletQuota)-reserved {
		return ErrFundingAmountInvalid
	}
	if err := model.PostConsumeUserSubscriptionDeltaContext(fundingOperationContext(subscription.requestContext), subscription.subscriptionId, int64(amount)); err != nil {
		return err
	}
	subscription.reservedExtra += int64(amount)
	return nil
}

func (subscription *SubscriptionFunding) rollbackExtra(amount int) error {
	compensate, cancel := compensationContext()
	defer cancel()
	subscription.mu.Lock()
	defer subscription.mu.Unlock()
	if subscription.refunded || subscription.settled {
		return ErrFundingFinalized
	}
	if amount < 0 || int64(amount) > subscription.reservedExtra {
		return ErrFundingAmountInvalid
	}
	if err := model.PostConsumeUserSubscriptionDeltaContext(compensate, subscription.subscriptionId, -int64(amount)); err != nil {
		return err
	}
	subscription.reservedExtra -= int64(amount)
	return nil
}

func (subscription *SubscriptionFunding) Refund() error {
	compensate, cancel := compensationContext()
	defer cancel()
	subscription.mu.Lock()
	defer subscription.mu.Unlock()
	if subscription.settled || subscription.refunded || subscription.subscriptionId <= 0 {
		return nil
	}
	// The persisted request-key rollback releases both quota and savings
	// reservation metadata, independently of the caller's token reservation.
	if err := refundWithRetryContext(compensate, func() error { return model.RefundSubscriptionPreConsumeContext(compensate, subscription.requestId) }); err != nil {
		return fmt.Errorf("refund subscription reservation: %w", err)
	}
	subscription.refunded = true
	return nil
}

func refundWithRetry(fn func() error) error {
	return refundWithRetryContext(context.Background(), fn)
}

// Only request-key-idempotent subscription rollbacks may be retried.
func refundWithRetryContext(ctx context.Context, fn func() error) error {
	if fn == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var last error
	for attempt := range 3 {
		if err := ctx.Err(); err != nil {
			return errors.Join(last, err)
		}
		last = fn()
		if last == nil {
			return nil
		}
		if attempt == 2 {
			break
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return errors.Join(last, ctx.Err())
		case <-timer.C:
		}
	}
	return last
}

func fundingOperationContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
