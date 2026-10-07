package model

import (
	"context"
	"errors"
	"math"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var ErrTopUpIsSubscription = errors.New("payment belongs to a subscription order")

type topUpSettlementRequest struct {
	tradeNo       string
	provider      string
	paymentMethod string
	customerID    string
	customerEmail string
	evidence      *PaymentEvidence
}

type topUpSettlement struct {
	order       TopUp
	quota       int
	alreadyDone bool
}

// Pending orders created through Insert freeze the credited quota. Historical
// rows without a snapshot retain the provider's original amount convention.
func topUpCreditSnapshot(order *TopUp) (int, string, error) {
	if order.CreditedQuota != 0 {
		if _, err := topUpQuotaMaxCurrent(order.CreditedQuota); err != nil {
			return 0, "", err
		}
		return order.CreditedQuota, order.WalletUnits, nil
	}
	if common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
		return 0, "", ErrInvalidTopUpQuota
	}
	unitQuota := decimal.NewFromFloat(common.QuotaPerUnit)
	var quota decimal.Decimal
	switch order.PaymentProvider {
	case PaymentProviderStripe:
		if math.IsNaN(order.Money) || math.IsInf(order.Money, 0) {
			return 0, "", ErrInvalidTopUpQuota
		}
		quota = decimal.NewFromFloat(order.Money).Mul(unitQuota)
	case PaymentProviderCreem:
		quota = decimal.NewFromInt(order.Amount)
	default:
		quota = decimal.NewFromInt(order.Amount).Mul(unitQuota)
	}
	credited, err := common.WalletQuotaFromDecimalStrict(quota)
	if err != nil || credited <= 0 {
		return 0, "", ErrInvalidTopUpQuota
	}
	return credited, quota.Div(unitQuota).String(), nil
}

func loadTopUpForSettlementTx(tx *gorm.DB, tradeNo, provider string) (*TopUp, error) {
	var order TopUp
	if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTopUpNotFound
		}
		return nil, err
	}
	if provider != "" && order.PaymentProvider != provider {
		return nil, ErrPaymentMethodMismatch
	}
	return &order, nil
}

// settleTopUp commits the wallet increment, provider metadata and terminal
// order state together. Only the winning transition publishes cache/log effects.
func settleTopUpContext(ctx context.Context, request topUpSettlementRequest) (*topUpSettlement, error) {
	if request.tradeNo == "" {
		return nil, ErrTopUpNotFound
	}
	var settlement topUpSettlement
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		order, err := loadTopUpForSettlementTx(tx, request.tradeNo, request.provider)
		if err != nil {
			return err
		}
		if tx.Migrator().HasTable(&SubscriptionOrder{}) {
			var count int64
			if err := tx.Model(&SubscriptionOrder{}).Where("trade_no = ?", request.tradeNo).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrTopUpIsSubscription
			}
		}
		settlement.order = *order
		if order.Status == common.TopUpStatusSuccess {
			settlement.alreadyDone = true
			return nil
		}
		if order.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}
		if err := validatePaymentEvidence(order.Money, order.PaymentCurrency, order.AcceptsDiscounts, request.evidence); err != nil {
			return err
		}
		quota, units, err := topUpCreditSnapshot(order)
		if err != nil {
			return err
		}
		updates := map[string]any{}
		if request.customerID != "" {
			updates["stripe_customer"] = request.customerID
		}
		if request.customerEmail != "" {
			// A payment receipt cannot replace an established account identity.
			updates["email"] = gorm.Expr("CASE WHEN email = '' THEN ? ELSE email END", request.customerEmail)
		}
		if err := creditTopUpQuota(tx, order.UserId, quota, updates); err != nil {
			return err
		}
		if request.paymentMethod != "" {
			order.PaymentMethod = request.paymentMethod
		}
		order.CreditedQuota, order.WalletUnits = quota, units
		order.Status, order.CompleteTime = common.TopUpStatusSuccess, common.GetTimestamp()
		result := tx.Model(&TopUp{}).Where("id = ? AND status = ?", order.Id, common.TopUpStatusPending).
			Updates(map[string]any{"status": order.Status, "complete_time": order.CompleteTime,
				"payment_method": order.PaymentMethod, "credited_quota": quota, "wallet_units": units})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrTopUpStatusInvalid
		}
		settlement.order, settlement.quota = *order, quota
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !settlement.alreadyDone {
		syncCreditUserQuotaCache(settlement.order.UserId, settlement.quota, "payment settlement")
	}
	return &settlement, nil
}

func transitionPendingTopUpContext(ctx context.Context, tradeNo, provider, targetStatus string) error {
	if targetStatus != common.TopUpStatusFailed && targetStatus != common.TopUpStatusExpired {
		return ErrTopUpStatusInvalid
	}
	if tradeNo == "" {
		return ErrTopUpNotFound
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		order, err := loadTopUpForSettlementTx(tx, tradeNo, provider)
		if err != nil {
			return err
		}
		if order.Status == targetStatus {
			return nil
		}
		if order.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}
		result := tx.Model(&TopUp{}).Where("id = ? AND status = ?", order.Id, common.TopUpStatusPending).
			Updates(map[string]any{"status": targetStatus, "complete_time": common.GetTimestamp()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrTopUpStatusInvalid
		}
		return nil
	})
}

func transitionPendingTopUp(tradeNo, provider, targetStatus string) error {
	return transitionPendingTopUpContext(context.Background(), tradeNo, provider, targetStatus)
}

func settleTopUp(request topUpSettlementRequest) (*topUpSettlement, error) {
	return settleTopUpContext(context.Background(), request)
}
