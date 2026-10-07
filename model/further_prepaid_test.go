package model

import (
	"errors"
	"sync"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func prepaidFixture(t *testing.T) (User, []FurtherPrepaidCard) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&FurtherPrepaidCard{}, &SubscriptionPreConsumeRecord{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&FurtherPrepaidCard{}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&FurtherPrepaidCard{}).Error)
		require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&SubscriptionPreConsumeRecord{}).Error)
	})
	user := User{Username: "prepaid-owner", AffCode: "prepaid_owner", Group: "default", LinuxDOId: "verified-linuxdo-owner", Status: common.UserStatusEnabled, Quota: 100}
	require.NoError(t, DB.Create(&user).Error)
	cards, err := GenerateFurtherPrepaidCards(FurtherPrepaidGenerateInput{Name: "测试额度卡", UserGroup: "default", Count: 2, Quota: 1000, ValidityDays: 30, CreatedBy: user.Id})
	require.NoError(t, err)
	return user, cards
}

func TestFurtherPrepaidCreditsExpiringLedgerAndMergesOwnerToken(t *testing.T) {
	user, cards := prepaidFixture(t)
	first, err := RedeemFurtherPrepaidCard(user.Id, cards[0].Code)
	require.NoError(t, err)
	require.Equal(t, false, first["merged"])
	require.NotEmpty(t, first["api_key"])
	second, err := RedeemFurtherPrepaidCard(user.Id, cards[1].Code)
	require.NoError(t, err)
	require.Equal(t, true, second["merged"])
	require.NotContains(t, second, "api_key", "a subsequent redemption must not expose an existing credential")
	require.Equal(t, first["token_id"], second["token_id"])
	var persisted User
	require.NoError(t, DB.First(&persisted, user.Id).Error)
	require.Equal(t, 100, persisted.Quota, "prepaid credit must not become permanent wallet funds")
	var token Token
	require.NoError(t, DB.First(&token, first["token_id"]).Error)
	require.Equal(t, user.Id, token.UserId)
	require.Equal(t, 2000, token.RemainQuota)
	require.False(t, token.UnlimitedQuota)
	require.Greater(t, token.ExpiredTime, common.GetTimestamp())
	consumed, err := PreConsumeUserSubscription("prepaid-real-ledger-use", user.Id, "test-model", 0, 300)
	require.NoError(t, err)
	require.Equal(t, int64(300), consumed.PreConsumed)
	balance, err := GetFurtherPrepaidBalance(user.Id)
	require.NoError(t, err)
	require.Equal(t, int64(1700), balance["total_remaining"])
	require.NotEqual(t, first["api_key"], balance["key_hint"])
	var plan SubscriptionPlan
	require.NoError(t, DB.First(&plan, cards[0].PlanID).Error)
	require.False(t, plan.Enabled, "fulfillment plans must not be purchasable")
}

func TestFurtherPrepaidConcurrentRedemptionCreditsExactlyOneOwner(t *testing.T) {
	user, cards := prepaidFixture(t)
	other := User{Username: "prepaid-other", AffCode: "prepaid_other", Group: "default", LinuxDOId: "verified-linuxdo-other", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(&other).Error)
	var wg sync.WaitGroup
	errorsFound := make(chan error, 2)
	for _, userID := range []int{user.Id, other.Id} {
		wg.Go(func() {
			_, err := RedeemFurtherPrepaidCard(userID, cards[0].Code)
			errorsFound <- err
		})
	}
	wg.Wait()
	close(errorsFound)
	successes := 0
	for err := range errorsFound {
		if err == nil {
			successes++
		}
	}
	require.Equal(t, 1, successes)
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("source = ?", "further_prepaid").Count(&count).Error)
	require.Equal(t, int64(1), count)
	require.NoError(t, DB.Model(&Token{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestFurtherPrepaidRejectsUnboundWrongGroupAndExpiredClaimsWithoutConsuming(t *testing.T) {
	user, cards := prepaidFixture(t)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("linux_do_id", "").Error)
	_, err := RedeemFurtherPrepaidCard(user.Id, cards[0].Code)
	require.ErrorContains(t, err, "绑定 LinuxDO")
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]any{"linux_do_id": "verified-linuxdo-owner", "group": "pay"}).Error)
	_, err = RedeemFurtherPrepaidCard(user.Id, cards[0].Code)
	require.ErrorContains(t, err, "用户组不一致")
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("group", "default").Error)
	require.NoError(t, DB.Model(&FurtherPrepaidCard{}).Where("id = ?", cards[0].ID).Update("claim_expires_at", common.GetTimestamp()-1).Error)
	_, err = RedeemFurtherPrepaidCard(user.Id, cards[0].Code)
	require.ErrorContains(t, err, "过兑换期限")
	var persisted FurtherPrepaidCard
	require.NoError(t, DB.First(&persisted, cards[0].ID).Error)
	require.Equal(t, 1, persisted.Status)
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("source = ?", "further_prepaid").Count(&count).Error)
	require.Zero(t, count)
}

func TestFurtherPrepaidRollsBackGrantAndClaimWhenTokenCreationFails(t *testing.T) {
	user, cards := prepaidFixture(t)
	const hook = "test:prepaid-token-failure"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" {
			tx.AddError(errors.New("token storage unavailable"))
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Create().Remove(hook) })
	_, err := RedeemFurtherPrepaidCard(user.Id, cards[0].Code)
	require.ErrorContains(t, err, "token storage unavailable")
	var persisted FurtherPrepaidCard
	require.NoError(t, DB.First(&persisted, cards[0].ID).Error)
	require.Equal(t, 1, persisted.Status)
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("source = ?", "further_prepaid").Count(&count).Error)
	require.Zero(t, count)
}

func TestFurtherPrepaidCleanupDoesNotDeleteFundsAndExpiredFundsCannotBeConsumed(t *testing.T) {
	user, cards := prepaidFixture(t)
	result, err := RedeemFurtherPrepaidCard(user.Id, cards[0].Code)
	require.NoError(t, err)
	_, err = DeleteFurtherPrepaidCards(cards[0].ID, "", false)
	require.Error(t, err)
	deleted, err := DeleteFurtherPrepaidCards(0, "", true)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
	balance, err := GetFurtherPrepaidBalance(user.Id)
	require.NoError(t, err)
	require.Equal(t, int64(1000), balance["total_remaining"])
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", result["subscription_id"]).Update("end_time", common.GetTimestamp()-1).Error)
	balance, err = GetFurtherPrepaidBalance(user.Id)
	require.NoError(t, err)
	require.Zero(t, balance["total_remaining"])
	_, err = PreConsumeUserSubscription("prepaid-expired-no-funds", user.Id, "test-model", 0, 1)
	require.Error(t, err)
}

func TestFurtherPrepaidBatchListingAndUnusedDeletionUseRealCounts(t *testing.T) {
	user, cards := prepaidFixture(t)
	_, err := RedeemFurtherPrepaidCard(user.Id, cards[0].Code)
	require.NoError(t, err)
	result, err := ListFurtherPrepaidCards(1, 25, "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), result["total"])
	batch := result["items"].([]map[string]any)[0]
	require.Equal(t, 1, batch["unused_count"])
	require.Equal(t, 1, batch["redeemed_count"])
	deleted, err := DeleteFurtherPrepaidCards(0, cards[0].BatchID, false)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
	_, err = RedeemFurtherPrepaidCard(user.Id, cards[1].Code)
	require.Error(t, err)
}
