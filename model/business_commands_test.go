package model

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func businessCommandDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB, previousRedis := DB, LOG_DB, common.RedisEnabled
	DB, LOG_DB, common.RedisEnabled = db, db, false
	connection, err := db.DB()
	require.NoError(t, err)
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() {
		DB, LOG_DB, common.RedisEnabled = previousDB, previousLogDB, previousRedis
		require.NoError(t, connection.Close())
	})
	require.NoError(t, db.AutoMigrate(&User{}, &UserSession{}, &Token{}, &Channel{}, &Ability{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionPreConsumeRecord{}, &FurtherSubscriptionPolicy{}, &FurtherUserSubscriptionPolicy{}, &FurtherDiscountReservation{}, &FurtherSubscriptionPreference{}, &FurtherSubscriptionHidden{}))
	return db
}

func TestTokenConfigurationPreservesUsageCommittedAfterItsBaseline(t *testing.T) {
	db := businessCommandDatabase(t)
	token := Token{UserId: 71, Key: "configuration-accounting-key", Name: "before", RemainQuota: 1000, ExpiredTime: -1, Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(&token).Error)
	baseline := token.RemainQuota
	next := token
	next.Name, next.RemainQuota, next.Key = "after", 1500, "must-not-rotate"
	require.NoError(t, db.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]any{"remain_quota": 700, "used_quota": 300, "accessed_time": 77, "status": common.TokenStatusDisabled}).Error)
	require.NoError(t, UpdateTokenConfiguration(context.Background(), &next, baseline))
	require.Equal(t, 1200, next.RemainQuota)
	require.Equal(t, 300, next.UsedQuota)
	require.EqualValues(t, 77, next.AccessedTime)
	require.Equal(t, common.TokenStatusDisabled, next.Status)
	require.Equal(t, token.Key, next.Key)
	next.UserId++
	require.ErrorIs(t, UpdateTokenConfiguration(context.Background(), &next, baseline), gorm.ErrRecordNotFound)
}

func TestChannelCatalogRollsBackConfigurationWhenRoutingIndexFails(t *testing.T) {
	db := businessCommandDatabase(t)
	channel := Channel{Key: "channel-index-key", Name: "before", Models: "old-model", Group: "default", UsedQuota: 91}
	require.NoError(t, channel.Insert())
	desired := channel
	desired.Name, desired.Models, desired.UsedQuota = "after", "new-model", 100000
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:routing-index-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			tx.AddError(errors.New("routing index unavailable"))
		}
	}))
	err := UpdateChannelCatalog(context.Background(), &desired, map[string]any{"name": "after", "models": "new-model", "used_quota": 100000})
	require.ErrorContains(t, err, "routing index unavailable")
	var stored Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	require.Equal(t, "before", stored.Name)
	require.Equal(t, "old-model", stored.Models)
	require.EqualValues(t, 91, stored.UsedQuota)
	var ability Ability
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&ability).Error)
	require.Equal(t, "old-model", ability.Model)
}

func TestSubscriptionWorkspaceExcludesFutureEntitlements(t *testing.T) {
	db := businessCommandDatabase(t)
	user := User{Username: "workspace-subscription", AffCode: "workspace-subscription"}
	require.NoError(t, db.Create(&user).Error)
	plan := SubscriptionPlan{Title: "Workspace plan"}
	require.NoError(t, db.Create(&plan).Error)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&[]UserSubscription{
		{UserId: user.Id, PlanId: plan.Id, Status: "active", StartTime: now - 10, EndTime: now + 100},
		{UserId: user.Id, PlanId: plan.Id, Status: "active", StartTime: now + 10, EndTime: now + 100},
	}).Error)
	workspace, err := ReadSubscriptionWorkspace(context.Background(), user.Id)
	require.NoError(t, err)
	require.Len(t, workspace.AllSubscriptions, 2)
	require.Len(t, workspace.Subscriptions, 1)
	require.Equal(t, plan.Title, workspace.Subscriptions[0].PlanTitle)
	_, err = ReadSubscriptionWorkspace(context.Background(), 999999)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestSubscriptionOneSecondResetSkipsAllMissedPeriods(t *testing.T) {
	db := businessCommandDatabase(t)
	now := time.Now().Unix()
	plan := SubscriptionPlan{Title: "Frequent reset", QuotaResetPeriod: SubscriptionResetCustom, QuotaResetCustomSeconds: 1}
	require.NoError(t, db.Create(&plan).Error)
	subscription := UserSubscription{UserId: 72, PlanId: plan.Id, Status: "active", AmountTotal: 1000, AmountUsed: 700, StartTime: now - 86400*365, EndTime: now + 100, LastResetTime: now - 86400*365, NextResetTime: now - 86400*365 + 1}
	require.NoError(t, db.Create(&subscription).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return maybeResetUserSubscriptionWithPlanTx(tx, &subscription, &plan, now)
	}))
	require.Zero(t, subscription.AmountUsed)
	require.Equal(t, now, subscription.LastResetTime)
	require.Equal(t, now+1, subscription.NextResetTime)
}

func TestSubscriptionReservationNeverReplaysForAnotherOwner(t *testing.T) {
	db := businessCommandDatabase(t)
	owner := User{Username: "reservation-owner", AffCode: "reservation-owner"}
	other := User{Username: "reservation-other", AffCode: "reservation-other"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&other).Error)
	plan := SubscriptionPlan{Title: "Reserved", DurationUnit: SubscriptionDurationDay, DurationValue: 1, TotalAmount: 500}
	require.NoError(t, db.Create(&plan).Error)
	_, err := AdminBindSubscription(owner.Id, plan.Id, "")
	require.NoError(t, err)
	first, err := PreConsumeUserSubscription("owned-reservation", owner.Id, "test-model", 0, 100)
	require.NoError(t, err)
	_, err = PreConsumeUserSubscription("owned-reservation", other.Id, "test-model", 0, 100)
	require.ErrorContains(t, err, "another user")
	replay, err := PreConsumeUserSubscription("owned-reservation", owner.Id, "test-model", 0, 100)
	require.NoError(t, err)
	require.Equal(t, first.PreConsumed, replay.PreConsumed)
	var subscription UserSubscription
	require.NoError(t, db.First(&subscription, first.UserSubscriptionId).Error)
	require.EqualValues(t, 100, subscription.AmountUsed)
}

func TestSubscriptionCatalogRejectsInvalidDurationBeforeCreatingDraft(t *testing.T) {
	db := businessCommandDatabase(t)
	_, err := SaveSubscriptionCatalog(context.Background(), 0, SubscriptionPlanInput{Title: "Invalid period", DurationUnit: SubscriptionDurationCustom, CustomSeconds: -1})
	require.Error(t, err)
	var count int64
	require.NoError(t, db.Model(&SubscriptionPlan{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestReferralConversionRejectsWalletOverflowWithoutLosingReferralBalance(t *testing.T) {
	db := businessCommandDatabase(t)
	amount := max(int(common.QuotaPerUnit), 1)
	user := User{Username: "referral-overflow", AffCode: "referral-overflow", Quota: common.MaxWalletQuota - amount + 1, AffQuota: amount}
	require.NoError(t, db.Create(&user).Error)
	require.ErrorIs(t, user.TransferAffQuotaToQuota(amount), ErrWalletQuotaLimitExceeded)
	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	require.Equal(t, user.Quota, stored.Quota)
	require.Equal(t, amount, stored.AffQuota)
}

func TestSubscriptionCleanupRetainsLiveDiscountsAndExpiresOrdinaryOldRecords(t *testing.T) {
	db := businessCommandDatabase(t)
	records := []SubscriptionPreConsumeRecord{
		{RequestId: "retention-ordinary", UserId: 73, Status: "consumed"},
		{RequestId: "retention-live-discount", UserId: 73, Status: "consumed"},
		{RequestId: "retention-final", UserId: 73, Status: "settled"},
	}
	require.NoError(t, db.Create(&records).Error)
	require.NoError(t, db.Exec("UPDATE subscription_pre_consume_records SET updated_at = ?", time.Now().Unix()-86400).Error)
	require.NoError(t, db.Create(&FurtherDiscountReservation{RequestID: "retention-live-discount", UserID: 73, Status: "reserved"}).Error)
	deleted, err := CleanupSubscriptionPreConsumeRecords(3600)
	require.NoError(t, err)
	require.EqualValues(t, 2, deleted)
	var remaining []SubscriptionPreConsumeRecord
	require.NoError(t, db.Find(&remaining).Error)
	require.Len(t, remaining, 1)
	require.Equal(t, "retention-live-discount", remaining[0].RequestId)
}
