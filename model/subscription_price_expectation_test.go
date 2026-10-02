package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func seedBalancePurchaseExpectation(t *testing.T) (int, int, int) {
	t.Helper()
	setupRelayForSubscriptionTest(t)
	require.NoError(t, DB.AutoMigrate(&SubscriptionOrder{}, &RelayForPrepaidCard{}))
	const initialQuota = 10_000_000
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 21).Update("quota", initialQuota).Error)
	plan := SubscriptionPlan{
		Title:         "Price checked plan",
		PriceAmount:   2.5,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationDay,
		DurationValue: 30,
		Enabled:       true,
	}
	require.NoError(t, DB.Create(&plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	return 21, plan.Id, initialQuota
}

func requireNoBalancePurchase(t *testing.T, userID int, initialQuota int) {
	t.Helper()
	var user User
	require.NoError(t, DB.First(&user, userID).Error)
	require.Equal(t, initialQuota, user.Quota)
	var subscriptions, orders int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", userID).Count(&subscriptions).Error)
	require.NoError(t, DB.Model(&SubscriptionOrder{}).Where("user_id = ?", userID).Count(&orders).Error)
	require.Zero(t, subscriptions)
	require.Zero(t, orders)
}

func TestBalancePurchaseRejectsChangedQuotedPriceBeforeDebit(t *testing.T) {
	userID, planID, initialQuota := seedBalancePurchaseExpectation(t)
	// Populate the plan cache before an admin changes the database row. The
	// purchase must read the current transactional row rather than this cache.
	_, err := GetSubscriptionPlanById(planID)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", planID).Update("price_amount", 3.25).Error)
	oldPrice, currentPrice, currency := 2.5, 3.25, "USD"
	err = PurchaseSubscriptionWithBalance(userID, planID, SubscriptionPurchaseExpectation{PriceAmount: &oldPrice, Currency: &currency})
	require.ErrorContains(t, err, "价格已更新")
	requireNoBalancePurchase(t, userID, initialQuota)

	require.NoError(t, PurchaseSubscriptionWithBalance(userID, planID, SubscriptionPurchaseExpectation{PriceAmount: &currentPrice, Currency: &currency}))
	requiredQuota, err := calcSubscriptionBalanceQuota(currentPrice)
	require.NoError(t, err)
	var user User
	require.NoError(t, DB.First(&user, userID).Error)
	require.Equal(t, initialQuota-requiredQuota, user.Quota)
	var order SubscriptionOrder
	require.NoError(t, DB.Where("user_id = ?", userID).First(&order).Error)
	require.Equal(t, currentPrice, order.Money)
}

func TestBalancePurchaseRejectsChangedQuotedCurrencyAndZeroPrice(t *testing.T) {
	userID, planID, initialQuota := seedBalancePurchaseExpectation(t)
	price, zeroPrice, oldCurrency := 2.5, 0.0, "EUR"
	err := PurchaseSubscriptionWithBalance(userID, planID, SubscriptionPurchaseExpectation{PriceAmount: &price, Currency: &oldCurrency})
	require.ErrorContains(t, err, "币种已更新")
	requireNoBalancePurchase(t, userID, initialQuota)
	err = PurchaseSubscriptionWithBalance(userID, planID, SubscriptionPurchaseExpectation{PriceAmount: &zeroPrice})
	require.ErrorContains(t, err, "价格已更新")
	requireNoBalancePurchase(t, userID, initialQuota)
}

func TestBalancePurchaseLegacyRequestRemainsCompatible(t *testing.T) {
	userID, planID, initialQuota := seedBalancePurchaseExpectation(t)
	require.NoError(t, PurchaseSubscriptionWithBalance(userID, planID))
	requiredQuota, err := calcSubscriptionBalanceQuota(2.5)
	require.NoError(t, err)
	var user User
	require.NoError(t, DB.First(&user, userID).Error)
	require.Equal(t, initialQuota-requiredQuota, user.Quota)
}

func TestBalancePurchaseExpectedUSDAllowsLegacyBlankCurrency(t *testing.T) {
	userID, planID, initialQuota := seedBalancePurchaseExpectation(t)
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", planID).Update("currency", "").Error)
	price, currency := 2.5, "USD"
	require.NoError(t, PurchaseSubscriptionWithBalance(userID, planID, SubscriptionPurchaseExpectation{PriceAmount: &price, Currency: &currency}))
	requiredQuota, err := calcSubscriptionBalanceQuota(price)
	require.NoError(t, err)
	var user User
	require.NoError(t, DB.First(&user, userID).Error)
	require.Equal(t, initialQuota-requiredQuota, user.Quota)
}

func TestBalancePurchaseRejectsInternalAndHistoricalPrepaidPlans(t *testing.T) {
	t.Run("internal policy", func(t *testing.T) {
		userID, planID, initialQuota := seedBalancePurchaseExpectation(t)
		require.NoError(t, DB.Create(&RelayForSubscriptionPolicy{PlanID: planID, Code: "internal-price-test", Internal: true}).Error)
		err := PurchaseSubscriptionWithBalance(userID, planID)
		require.ErrorContains(t, err, "仅用于内部发放")
		requireNoBalancePurchase(t, userID, initialQuota)
	})
	t.Run("soft deleted prepaid card", func(t *testing.T) {
		userID, planID, initialQuota := seedBalancePurchaseExpectation(t)
		card := RelayForPrepaidCard{PlanID: planID, Code: "rfp_price_test", CodeHash: "price-test-hash", BatchID: "price-test", Quota: int(common.QuotaPerUnit)}
		require.NoError(t, DB.Create(&card).Error)
		require.NoError(t, DB.Delete(&card).Error)
		err := PurchaseSubscriptionWithBalance(userID, planID)
		require.ErrorContains(t, err, "仅用于内部发放")
		requireNoBalancePurchase(t, userID, initialQuota)
	})
}

func TestPendingThirdPartySubscriptionOrdersRejectProtectedPlanIDs(t *testing.T) {
	for _, provider := range []string{PaymentProviderStripe, PaymentProviderCreem, PaymentProviderEpay, PaymentProviderWaffoPancake} {
		t.Run(provider, func(t *testing.T) {
			userID, planID, _ := seedBalancePurchaseExpectation(t)
			require.NoError(t, DB.Create(&RelayForSubscriptionPolicy{PlanID: planID, Code: "internal-order-test", Internal: true}).Error)
			order := &SubscriptionOrder{UserId: userID, PlanId: planID, Money: 2.5, TradeNo: "protected-" + provider, PaymentProvider: provider, Status: common.TopUpStatusPending}
			require.ErrorContains(t, order.Insert(), "仅用于内部发放")
			var count int64
			require.NoError(t, DB.Model(&SubscriptionOrder{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
	t.Run("soft-deleted prepaid plan", func(t *testing.T) {
		userID, planID, _ := seedBalancePurchaseExpectation(t)
		card := RelayForPrepaidCard{PlanID: planID, Code: "rfp_order_test", CodeHash: "order-test-hash"}
		require.NoError(t, DB.Create(&card).Error)
		require.NoError(t, DB.Delete(&card).Error)
		order := &SubscriptionOrder{UserId: userID, PlanId: planID, Money: 2.5, TradeNo: "protected-prepaid", PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending}
		require.ErrorContains(t, order.Insert(), "仅用于内部发放")
	})
	t.Run("public plan remains orderable", func(t *testing.T) {
		userID, planID, _ := seedBalancePurchaseExpectation(t)
		order := &SubscriptionOrder{UserId: userID, PlanId: planID, Money: 2.5, TradeNo: "public-external", PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending}
		require.NoError(t, order.Insert())
		require.Positive(t, order.Id)
	})
}
