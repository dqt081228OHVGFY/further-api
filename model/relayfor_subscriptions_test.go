package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRelayForSubscriptionTest(t *testing.T) {
	setupRelayForStatsTest(t)
	initCol()
	require.NoError(t, DB.AutoMigrate(&RelayForSubscriptionPolicy{}, &RelayForUserSubscriptionPolicy{}, &RelayForSubscriptionPreference{}, &RelayForSubscriptionHidden{}, &RelayForBenefitCampaign{}, &RelayForBenefitGrant{}, &RelayForDiscountReservation{}, &SubscriptionPreConsumeRecord{}, &Model{}))
	sqlDB, err := DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, DB.Create(&User{Id: 21, Username: "subscriber", Password: "hash", Status: common.UserStatusEnabled, Group: "default", Quota: 100}).Error)
}

func relayForTestPlan(t *testing.T, code string, multiplier float64, cap int64, parent int) int {
	t.Helper()
	id, err := SaveRelayForSubscriptionPlan(0, RelayForPlanInput{Code: code, Name: code, DurationDays: 30, Quota: 1000, Status: 1, DiscountMultiplier: multiplier, DiscountSavingsLimit: cap, UpgradeFromPlanID: parent})
	require.NoError(t, err)
	return id
}

func relayForTestGrant(t *testing.T, planID int) *UserSubscription {
	t.Helper()
	var sub *UserSubscription
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		sub, err = GrantRelayForSubscriptionTx(tx, 21, planID, "test")
		return err
	})
	require.NoError(t, err)
	return sub
}

func TestRelayForDiscountBudgetReservationSettlementAndRefundAreIdempotent(t *testing.T) {
	setupRelayForSubscriptionTest(t)
	planID := relayForTestPlan(t, "half", 0.5, 60, 0)
	sub := relayForTestGrant(t, planID)
	first, err := PreConsumeUserSubscription("discount-first", 21, "model", 0, 100)
	require.NoError(t, err)
	assert.EqualValues(t, 50, first.PreConsumed)
	second, err := PreConsumeUserSubscription("discount-second", 21, "model", 0, 100)
	require.NoError(t, err)
	assert.EqualValues(t, 90, second.PreConsumed)
	charged, err := SettleRelayForSubscriptionReservation("discount-first", 80)
	require.NoError(t, err)
	assert.EqualValues(t, 40, charged)
	charged, err = SettleRelayForSubscriptionReservation("discount-first", 80)
	require.NoError(t, err)
	assert.EqualValues(t, 40, charged)
	require.NoError(t, RefundSubscriptionPreConsume("discount-first"))
	require.NoError(t, RefundSubscriptionPreConsume("discount-second"))
	require.NoError(t, RefundSubscriptionPreConsume("discount-second"))
	var policy RelayForUserSubscriptionPolicy
	require.NoError(t, DB.First(&policy, "subscription_id = ?", sub.Id).Error)
	assert.EqualValues(t, 40, policy.DiscountSavingsUsed)
	assert.Zero(t, policy.DiscountSavingsReserved)
	third, err := PreConsumeUserSubscription("discount-third", 21, "model", 0, 100)
	require.NoError(t, err)
	assert.EqualValues(t, 80, third.PreConsumed)
	charged, err = ResizeRelayForSubscriptionReservation("discount-third", 120)
	require.NoError(t, err)
	assert.EqualValues(t, 100, charged)
	charged, err = SettleRelayForSubscriptionReservation("discount-third", 70)
	require.NoError(t, err)
	assert.EqualValues(t, 50, charged)
	require.NoError(t, DB.First(&policy, "subscription_id = ?", sub.Id).Error)
	assert.EqualValues(t, 60, policy.DiscountSavingsUsed)
	assert.Zero(t, policy.DiscountSavingsReserved)
	var stored UserSubscription
	require.NoError(t, DB.First(&stored, sub.Id).Error)
	assert.EqualValues(t, 90, stored.AmountUsed)
}

func TestRelayForBenefitsCreditOnceRestoreExtendAndUseTimedQuota(t *testing.T) {
	setupRelayForSubscriptionTest(t)
	planID := relayForTestPlan(t, "benefits", 1, 0, 0)
	sub := relayForTestGrant(t, planID)
	require.NoError(t, DB.Model(sub).Update("amount_used", 300).Error)
	_, err := CreateRelayForBenefitCampaign(RelayForBenefitCampaign{PlanID: planID, Name: "benefit", PermanentQuota: 20, TimedQuota: 70, RestoreQuotaPercent: 10, ExtendSubscriptionDays: 2})
	require.NoError(t, err)
	var grant RelayForBenefitGrant
	require.NoError(t, DB.First(&grant).Error)
	_, err = ClaimRelayForBenefit(22, grant.ID)
	require.Error(t, err)
	_, err = ClaimRelayForBenefit(21, grant.ID)
	require.NoError(t, err)
	_, err = ClaimRelayForBenefit(21, grant.ID)
	require.NoError(t, err)
	var user User
	require.NoError(t, DB.First(&user, 21).Error)
	assert.Equal(t, 120, user.Quota)
	var refreshed UserSubscription
	require.NoError(t, DB.First(&refreshed, sub.Id).Error)
	assert.EqualValues(t, 200, refreshed.AmountUsed)
	assert.EqualValues(t, sub.EndTime+2*86400, refreshed.EndTime)
	var timed UserSubscription
	require.NoError(t, DB.Where("source = ?", "relayfor_benefit_timed").First(&timed).Error)
	assert.EqualValues(t, 70, timed.AmountTotal)
	require.NoError(t, SetRelayForBillingPriority(21, true))
	reservation, err := PreConsumeUserSubscription("timed-priority", 21, "model", 0, 50)
	require.NoError(t, err)
	assert.Equal(t, timed.Id, reservation.UserSubscriptionId)
	require.NoError(t, RefundSubscriptionPreConsume("timed-priority"))
}

func TestRelayForUpgradeChainAndAbandonmentUseRealSnapshots(t *testing.T) {
	setupRelayForSubscriptionTest(t)
	baseID := relayForTestPlan(t, "base", 1, 0, 0)
	advancedID := relayForTestPlan(t, "advanced", 0.8, 0, baseID)
	err := DB.Transaction(func(tx *gorm.DB) error { _, err := GrantRelayForSubscriptionTx(tx, 21, advancedID, "test"); return err })
	require.Error(t, err)
	base := relayForTestGrant(t, baseID)
	advanced := relayForTestGrant(t, advancedID)
	active, err := GetRelayForActiveEntitlements(21, "")
	require.NoError(t, err)
	require.Len(t, active, 2)
	result, err := AbandonRelayForSubscription(21, base.Id, "convert")
	require.NoError(t, err)
	assert.EqualValues(t, 2000, result["result"].(map[string]any)["converted_quota"])
	var latest UserSubscription
	require.NoError(t, DB.First(&latest, advanced.Id).Error)
	assert.Equal(t, "cancelled", latest.Status)
	var converted []UserSubscription
	require.NoError(t, DB.Where("source = ?", "relayfor_converted").Find(&converted).Error)
	require.Len(t, converted, 2)
	for _, row := range converted {
		assert.Greater(t, row.EndTime, time.Now().Unix())
		assert.EqualValues(t, 1000, row.AmountTotal)
	}
	_, err = AbandonRelayForSubscription(21, base.Id, "convert")
	require.Error(t, err)
}
