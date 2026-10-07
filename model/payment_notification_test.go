package model

import (
	"context"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/stretchr/testify/require"
)

func TestVerifiedPaymentRejectsAmountCurrencyAndProviderBeforeCredit(t *testing.T) {
	db := businessCommandDatabase(t)
	require.NoError(t, db.AutoMigrate(&TopUp{}))
	user := User{Username: "verified-payment", AffCode: "verified-payment"}
	require.NoError(t, db.Create(&user).Error)
	order, err := CreateGatewayTopUpContext(context.Background(), user.Id, GatewayTopUpCreate{PaymentProvider: PaymentProviderEpay, WalletUnits: "2", Money: 14, PaymentCurrency: "CNY"})
	require.NoError(t, err)
	for _, input := range []GatewayPaymentConfirmation{
		{TradeNo: order.TradeNo, Provider: PaymentProviderStripe, Evidence: &PaymentEvidence{Amount: "14", Currency: "CNY"}},
		{TradeNo: order.TradeNo, Provider: PaymentProviderEpay, Evidence: &PaymentEvidence{Amount: "1", Currency: "CNY"}},
		{TradeNo: order.TradeNo, Provider: PaymentProviderEpay, Evidence: &PaymentEvidence{Amount: "14", Currency: "USD"}},
		{TradeNo: order.TradeNo, Provider: PaymentProviderEpay, Evidence: &PaymentEvidence{Currency: "CNY"}},
	} {
		require.Error(t, CompleteGatewayPaymentContext(context.Background(), input))
	}
	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	require.Zero(t, stored.Quota)
	require.Equal(t, common.TopUpStatusPending, GetTopUpByTradeNo(order.TradeNo).Status)
	input := GatewayPaymentConfirmation{TradeNo: order.TradeNo, Provider: PaymentProviderEpay, Evidence: &PaymentEvidence{Amount: "14.00", Currency: "cny"}}
	require.NoError(t, CompleteGatewayPaymentContext(context.Background(), input))
	require.NoError(t, CompleteGatewayPaymentContext(context.Background(), input))
	require.NoError(t, db.First(&stored, user.Id).Error)
	require.Equal(t, order.CreditedQuota, stored.Quota)
}

func TestVerifiedPaymentCancellationLeavesPendingOrder(t *testing.T) {
	db := businessCommandDatabase(t)
	require.NoError(t, db.AutoMigrate(&TopUp{}))
	user := User{Username: "payment-cancel", AffCode: "payment-cancel"}
	require.NoError(t, db.Create(&user).Error)
	order, err := CreateGatewayTopUpContext(context.Background(), user.Id, GatewayTopUpCreate{PaymentProvider: PaymentProviderWaffo, WalletUnits: "1", Money: 1, PaymentCurrency: "USD"})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, TransitionGatewayPaymentContext(ctx, order.TradeNo, PaymentProviderWaffo, common.TopUpStatusFailed), context.Canceled)
	require.ErrorIs(t, CompleteGatewayPaymentContext(ctx, GatewayPaymentConfirmation{TradeNo: order.TradeNo, Provider: PaymentProviderWaffo, Evidence: &PaymentEvidence{Amount: "1", Currency: "USD"}}), context.Canceled)
	require.Equal(t, common.TopUpStatusPending, GetTopUpByTradeNo(order.TradeNo).Status)
}

func TestVerifiedSubscriptionProofRejectsBeforeEntitlementAndLedger(t *testing.T) {
	truncateTables(t)
	user := insertUserForPaymentGuardTest(t, 7201, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 7301)
	InvalidateSubscriptionPlanCache(plan.Id)
	tradeNo := "further-subscription-proof"
	insertSubscriptionOrderForPaymentGuardTest(t, tradeNo, user.Id, plan.Id, PaymentProviderStripe)
	input := GatewayPaymentConfirmation{TradeNo: tradeNo, Provider: PaymentProviderStripe, Evidence: &PaymentEvidence{Amount: "1", Currency: "USD"}}
	require.ErrorIs(t, CompleteGatewayPaymentContext(context.Background(), input), ErrPaymentEvidenceMismatch)
	require.Zero(t, countUserSubscriptionsForPaymentGuardTest(t, user.Id))
	require.Nil(t, GetTopUpByTradeNo(tradeNo))
	input.Evidence.Amount = "9.99"
	require.NoError(t, CompleteGatewayPaymentContext(context.Background(), input))
	require.NoError(t, CompleteGatewayPaymentContext(context.Background(), input))
	require.Equal(t, int64(1), countUserSubscriptionsForPaymentGuardTest(t, user.Id))
	require.Zero(t, getUserQuotaForPaymentGuardTest(t, user.Id))
	InvalidateSubscriptionPlanCache(plan.Id)
}
