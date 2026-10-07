package model

import (
	"context"
	"errors"
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"math"
	"strings"
)

type GatewayTopUpCreate struct {
	TradeNo, PaymentProvider, PaymentMethod, WalletUnits, PaymentCurrency string
	Money                                                                 float64
	AcceptsDiscounts                                                      bool
}

func CreateGatewayTopUpContext(ctx context.Context, userID int, input GatewayTopUpCreate) (*TopUp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if userID <= 0 || input.Money <= 0 || math.IsNaN(input.Money) || math.IsInf(input.Money, 0) {
		return nil, ErrInvalidTopUpQuota
	}
	switch input.PaymentProvider {
	case PaymentProviderEpay, PaymentProviderStripe, PaymentProviderCreem, PaymentProviderWaffo, PaymentProviderWaffoPancake:
	default:
		return nil, ErrPaymentMethodMismatch
	}
	units, err := decimal.NewFromString(strings.TrimSpace(input.WalletUnits))
	if err != nil || units.Sign() <= 0 {
		return nil, ErrInvalidTopUpQuota
	}
	if common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
		return nil, ErrInvalidTopUpQuota
	}
	quotaDecimal := units.Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Ceil()
	if quotaDecimal.GreaterThan(decimal.NewFromInt(common.MaxWalletQuota)) {
		return nil, ErrTopUpQuotaLimitExceeded
	}
	quota := int(quotaDecimal.IntPart())
	if quota <= 0 {
		return nil, ErrInvalidTopUpQuota
	}
	if input.TradeNo == "" {
		input.TradeNo = "further-" + common.GetRandomString(24)
	}
	currency := strings.ToUpper(strings.TrimSpace(input.PaymentCurrency))
	if len(input.TradeNo) > 255 || len(input.PaymentMethod) > 50 || len(currency) > 8 {
		return nil, errors.New("支付订单参数无效")
	}
	order := TopUp{UserId: userID, TradeNo: input.TradeNo, PaymentProvider: input.PaymentProvider, PaymentMethod: input.PaymentMethod, PaymentCurrency: currency, AcceptsDiscounts: input.AcceptsDiscounts, WalletUnits: units.String(), CreditedQuota: quota, Amount: units.IntPart(), Money: input.Money, CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending}
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var account User
		if err := lockForUpdate(tx).Select("id", "quota", "status").First(&account, userID).Error; err != nil {
			return err
		}
		if account.Status != common.UserStatusEnabled {
			return ErrUserSessionInactive
		}
		if account.Quota > common.MaxWalletQuota-quota {
			return ErrTopUpQuotaLimitExceeded
		}
		return tx.Create(&order).Error
	})
	if err != nil {
		return nil, err
	}
	return &order, nil
}
