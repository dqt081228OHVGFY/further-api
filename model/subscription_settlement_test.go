package model

import (
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionSettlementUsesCurrentPlanSnapshot(t *testing.T) {
	truncateTables(t)
	user := insertUserForPaymentGuardTest(t, 6201, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 6301)
	InvalidateSubscriptionPlanCache(plan.Id)
	_, err := GetSubscriptionPlanById(plan.Id)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Update("total_amount", 2500).Error)
	insertSubscriptionOrderForPaymentGuardTest(t, "subscription-current-plan", user.Id, plan.Id, PaymentProviderStripe)

	require.NoError(t, CompleteSubscriptionOrder("subscription-current-plan", "", PaymentProviderStripe, ""))
	var subscription UserSubscription
	require.NoError(t, DB.Where("user_id = ?", user.Id).First(&subscription).Error)
	require.Equal(t, int64(2500), subscription.AmountTotal)
	require.Equal(t, PaymentProviderStripe, GetTopUpByTradeNo("subscription-current-plan").PaymentProvider)
	InvalidateSubscriptionPlanCache(plan.Id)
}

func TestAdminBindSubscriptionUsesCurrentPlanSnapshot(t *testing.T) {
	truncateTables(t)
	user := insertUserForPaymentGuardTest(t, 6202, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 6302)
	InvalidateSubscriptionPlanCache(plan.Id)
	_, err := GetSubscriptionPlanById(plan.Id)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Update("total_amount", 3000).Error)

	_, err = AdminBindSubscription(user.Id, plan.Id, "test")
	require.NoError(t, err)
	var subscription UserSubscription
	require.NoError(t, DB.Where("user_id = ?", user.Id).First(&subscription).Error)
	require.Equal(t, int64(3000), subscription.AmountTotal)
	InvalidateSubscriptionPlanCache(plan.Id)
}

func TestSubscriptionSettlementRejectsConflictingTopUpLedger(t *testing.T) {
	for _, tc := range []struct {
		name     string
		ownerID  int
		amount   int64
		provider string
		wantErr  error
	}{
		{name: "different owner", ownerID: 6204, provider: PaymentProviderStripe, wantErr: ErrSubscriptionLedgerConflict},
		{name: "wallet credit", ownerID: 6203, amount: 1, provider: PaymentProviderStripe, wantErr: ErrSubscriptionLedgerConflict},
		{name: "different provider", ownerID: 6203, provider: PaymentProviderCreem, wantErr: ErrPaymentMethodMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			user := insertUserForPaymentGuardTest(t, 6203, 0)
			plan := insertSubscriptionPlanForPaymentGuardTest(t, 6303)
			InvalidateSubscriptionPlanCache(plan.Id)
			tradeNo := "subscription-ledger-collision"
			insertSubscriptionOrderForPaymentGuardTest(t, tradeNo, user.Id, plan.Id, PaymentProviderStripe)
			ledger := TopUp{UserId: tc.ownerID, TradeNo: tradeNo, Amount: tc.amount,
				Money: 9.99, PaymentMethod: PaymentProviderStripe, PaymentProvider: tc.provider,
				Status: common.TopUpStatusPending}
			require.NoError(t, DB.Create(&ledger).Error)

			err := CompleteSubscriptionOrder(tradeNo, "", PaymentProviderStripe, "")
			require.ErrorIs(t, err, tc.wantErr)
			require.Zero(t, countUserSubscriptionsForPaymentGuardTest(t, user.Id))
			require.Equal(t, common.TopUpStatusPending, GetSubscriptionOrderByTradeNo(tradeNo).Status)
			require.Equal(t, common.TopUpStatusPending, GetTopUpByTradeNo(tradeNo).Status)
			InvalidateSubscriptionPlanCache(plan.Id)
		})
	}
}

func TestSubscriptionSettlementAcceptsMatchingLedgerPlaceholder(t *testing.T) {
	truncateTables(t)
	user := insertUserForPaymentGuardTest(t, 6205, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 6305)
	InvalidateSubscriptionPlanCache(plan.Id)
	tradeNo := "subscription-ledger-placeholder"
	insertSubscriptionOrderForPaymentGuardTest(t, tradeNo, user.Id, plan.Id, PaymentProviderStripe)
	ledger := TopUp{UserId: user.Id, TradeNo: tradeNo, Money: 9.99,
		PaymentMethod: PaymentProviderStripe, PaymentProvider: PaymentProviderStripe,
		Status: common.TopUpStatusPending}
	require.NoError(t, DB.Create(&ledger).Error)

	require.NoError(t, CompleteSubscriptionOrder(tradeNo, "", PaymentProviderStripe, "card"))
	require.Equal(t, int64(1), countUserSubscriptionsForPaymentGuardTest(t, user.Id))
	require.Equal(t, common.TopUpStatusSuccess, GetTopUpByTradeNo(tradeNo).Status)
	require.Equal(t, "card", GetTopUpByTradeNo(tradeNo).PaymentMethod)
	require.Equal(t, "card", GetSubscriptionOrderByTradeNo(tradeNo).PaymentMethod)
	InvalidateSubscriptionPlanCache(plan.Id)
}

func TestPendingTopUpStatusCannotCompleteWithoutCredit(t *testing.T) {
	truncateTables(t)
	user := insertUserForPaymentGuardTest(t, 6206, 0)
	insertTopUpForPaymentGuardTest(t, "topup-no-direct-success", user.Id, PaymentProviderStripe)

	err := UpdatePendingTopUpStatus("topup-no-direct-success", PaymentProviderStripe, common.TopUpStatusSuccess)
	require.ErrorIs(t, err, ErrTopUpStatusInvalid)
	require.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, "topup-no-direct-success"))
	require.Zero(t, getUserQuotaForPaymentGuardTest(t, user.Id))
}
