package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var ErrPaymentEvidenceMismatch = errors.New("支付凭据与订单金额或币种不一致")

// PaymentEvidence is populated only after a provider signature is verified.
// Amount excludes tax where a provider returns a separate tax component.
type PaymentEvidence struct {
	Amount, Currency string
}

func validatePaymentEvidence(money float64, currency string, discounts bool, evidence *PaymentEvidence) error {
	if evidence == nil {
		return nil // Internal commands and historical administrative completion.
	}
	if currency != "" && !strings.EqualFold(strings.TrimSpace(evidence.Currency), currency) {
		return ErrPaymentEvidenceMismatch
	}
	if strings.TrimSpace(evidence.Amount) == "" {
		if currency != "" {
			return ErrPaymentEvidenceMismatch
		}
		return nil // Historical providers did not all include an amount.
	}
	paid, err := decimal.NewFromString(strings.TrimSpace(evidence.Amount))
	if err != nil || paid.Sign() < 0 || math.IsNaN(money) || math.IsInf(money, 0) || money < 0 {
		return ErrPaymentEvidenceMismatch
	}
	expected := decimal.NewFromFloat(money).Round(2)
	if paid.Equal(expected) || (discounts && paid.Sign() > 0 && paid.LessThan(expected)) {
		return nil
	}
	return ErrPaymentEvidenceMismatch
}

type GatewayPaymentConfirmation struct {
	TradeNo, Provider, Method, CustomerID, CustomerEmail, Receipt, CallerIP string
	Evidence                                                                *PaymentEvidence
	SubscriptionOnly                                                        bool
}

// CompleteGatewayPaymentContext routes a verified receipt by its stored order.
// Subscription fulfillment and wallet credit each own a transactional decision.
func CompleteGatewayPaymentContext(ctx context.Context, input GatewayPaymentConfirmation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(input.TradeNo) == "" {
		return ErrTopUpNotFound
	}
	if DB.WithContext(ctx).Migrator().HasTable(&SubscriptionOrder{}) {
		err := CompleteSubscriptionOrderWithEvidenceContext(ctx, input.TradeNo, input.Receipt, input.Provider, input.Method, input.Evidence)
		if !errors.Is(err, ErrSubscriptionOrderNotFound) {
			return err
		}
	}
	if input.SubscriptionOnly {
		return ErrSubscriptionOrderNotFound
	}
	result, err := SettleTopUpContext(ctx, TopUpSettlementInput{TradeNo: input.TradeNo, Provider: input.Provider, PaymentMethod: input.Method, CustomerID: input.CustomerID, CustomerEmail: input.CustomerEmail, Evidence: input.Evidence})
	if err == nil && !result.AlreadyDone {
		RecordTopupLog(result.Order.UserId, fmt.Sprintf("充值到账，额度: %s，支付金额: %.2f", logger.FormatQuota(result.CreditedQuota), result.Order.Money), input.CallerIP, result.Order.PaymentMethod, input.Provider)
	}
	return err
}

type GatewayPaymentReference struct {
	UserID       int
	Subscription bool
}

func FindGatewayPaymentReferenceContext(ctx context.Context, tradeNo, provider string) (GatewayPaymentReference, error) {
	if err := ctx.Err(); err != nil {
		return GatewayPaymentReference{}, err
	}
	if strings.TrimSpace(tradeNo) == "" {
		return GatewayPaymentReference{}, ErrTopUpNotFound
	}
	if DB.WithContext(ctx).Migrator().HasTable(&SubscriptionOrder{}) {
		var order SubscriptionOrder
		err := DB.WithContext(ctx).Select("user_id", "payment_provider").Where("trade_no = ?", tradeNo).First(&order).Error
		if err == nil {
			if order.PaymentProvider != provider {
				return GatewayPaymentReference{}, ErrPaymentMethodMismatch
			}
			return GatewayPaymentReference{UserID: order.UserId, Subscription: true}, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return GatewayPaymentReference{}, err
		}
	}
	var order TopUp
	err := DB.WithContext(ctx).Select("user_id", "payment_provider").Where("trade_no = ?", tradeNo).First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return GatewayPaymentReference{}, ErrTopUpNotFound
	}
	if err != nil {
		return GatewayPaymentReference{}, err
	}
	if order.PaymentProvider != provider {
		return GatewayPaymentReference{}, ErrPaymentMethodMismatch
	}
	return GatewayPaymentReference{UserID: order.UserId}, nil
}

func TransitionGatewayPaymentContext(ctx context.Context, tradeNo, provider, status string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if DB.WithContext(ctx).Migrator().HasTable(&SubscriptionOrder{}) {
		err := TransitionSubscriptionOrderContext(ctx, tradeNo, provider, status)
		if !errors.Is(err, ErrSubscriptionOrderNotFound) {
			return err
		}
	}
	return transitionPendingTopUpContext(ctx, tradeNo, provider, status)
}
