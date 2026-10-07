package model

import (
	"context"
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGatewayCheckoutFreezesDecimalCreditBeforeProviderSettingsChange(t *testing.T) {
	db := businessCommandDatabase(t)
	require.NoError(t, db.AutoMigrate(&TopUp{}))
	previousUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = previousUnit })
	user := User{Username: "checkout-unit", AffCode: "checkout-unit"}
	require.NoError(t, db.Create(&user).Error)
	order, err := CreateGatewayTopUpContext(context.Background(), user.Id, GatewayTopUpCreate{PaymentProvider: PaymentProviderStripe, WalletUnits: "19.95", Money: 19.95})
	require.NoError(t, err)
	require.Equal(t, 9975000, order.CreditedQuota)
	require.Equal(t, "19.95", order.WalletUnits)
	require.Contains(t, order.TradeNo, "further-")
	common.QuotaPerUnit = 1000000
	result, err := SettleTopUpContext(context.Background(), TopUpSettlementInput{TradeNo: order.TradeNo, Provider: PaymentProviderStripe})
	require.NoError(t, err)
	require.Equal(t, 9975000, result.CreditedQuota)
	repeat, err := SettleTopUpContext(context.Background(), TopUpSettlementInput{TradeNo: order.TradeNo, Provider: PaymentProviderStripe})
	require.NoError(t, err)
	require.True(t, repeat.AlreadyDone)
	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	require.Equal(t, 9975000, stored.Quota)
}

func TestGatewayCheckoutRejectsCanceledOrFullWalletWithoutOrder(t *testing.T) {
	db := businessCommandDatabase(t)
	require.NoError(t, db.AutoMigrate(&TopUp{}))
	user := User{Username: "checkout-full", AffCode: "checkout-full", Quota: common.MaxWalletQuota}
	require.NoError(t, db.Create(&user).Error)
	_, err := CreateGatewayTopUpContext(context.Background(), user.Id, GatewayTopUpCreate{PaymentProvider: PaymentProviderEpay, WalletUnits: "1", Money: 1})
	require.ErrorIs(t, err, ErrTopUpQuotaLimitExceeded)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = CreateGatewayTopUpContext(ctx, user.Id, GatewayTopUpCreate{PaymentProvider: PaymentProviderEpay, WalletUnits: "1", Money: 1})
	require.ErrorIs(t, err, context.Canceled)
	var count int64
	require.NoError(t, db.Model(&TopUp{}).Count(&count).Error)
	require.Zero(t, count)
}
