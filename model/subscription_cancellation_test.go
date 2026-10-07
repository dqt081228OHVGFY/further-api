package model

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedCancelableAdminSubscription(t *testing.T) (*gorm.DB, User, SubscriptionPlan, UserSubscription) {
	t.Helper()
	// A canceled database transaction may discard its connection. Use a
	// durable temporary database so rollback is checked across reconnects.
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "subscriptions.db")), &gorm.Config{})
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
	require.NoError(t, db.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}, &FurtherSubscriptionPolicy{}, &FurtherUserSubscriptionPolicy{}, &FurtherSubscriptionHidden{}))
	account := User{Username: "cancel-subscription-admin", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default"}
	plan := SubscriptionPlan{Title: "取消请求测试套餐", Enabled: true, DurationUnit: SubscriptionDurationDay, DurationValue: 1, TotalAmount: 1000, QuotaResetPeriod: SubscriptionResetDaily}
	require.NoError(t, db.Create(&account).Error)
	require.NoError(t, db.Create(&plan).Error)
	now := time.Now().Unix()
	subscription := UserSubscription{UserId: account.Id, PlanId: plan.Id, AmountTotal: 1000, AmountUsed: 300, StartTime: now - 60, EndTime: now + 3600, Status: "active", LastResetTime: now - 60, NextResetTime: now + 300}
	require.NoError(t, db.Create(&subscription).Error)
	return db, account, plan, subscription
}

func TestCanceledAdminSubscriptionCommandsLeaveExistingEntitlementsUntouched(t *testing.T) {
	db, account, plan, subscription := seedCancelableAdminSubscription(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := AdminBindSubscriptionContext(ctx, account.Id, plan.Id, "")
	require.ErrorIs(t, err, context.Canceled)
	_, err = GetAllUserSubscriptionsContext(ctx, account.Id)
	require.ErrorIs(t, err, context.Canceled)
	_, err = GetAllActiveUserSubscriptionsContext(ctx, account.Id)
	require.ErrorIs(t, err, context.Canceled)
	_, err = AdminResetUserSubscriptionsByPlanContext(ctx, account.Id, plan.Id, true)
	require.ErrorIs(t, err, context.Canceled)
	_, err = AdminResetPlanSubscriptionsContext(ctx, plan.Id, true)
	require.ErrorIs(t, err, context.Canceled)
	_, err = AdminInvalidateUserSubscriptionContext(ctx, subscription.Id)
	require.ErrorIs(t, err, context.Canceled)
	_, err = AdminDeleteUserSubscriptionContext(ctx, subscription.Id)
	require.ErrorIs(t, err, context.Canceled)
	var persisted UserSubscription
	require.NoError(t, db.First(&persisted, subscription.Id).Error)
	require.Equal(t, subscription, persisted)
	var count int64
	require.NoError(t, db.Model(&UserSubscription{}).Where("user_id = ?", account.Id).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestAdminSubscriptionCancellationRollsBackWritesBeforeCommit(t *testing.T) {
	for _, action := range []string{"grant", "reset", "invalidate", "delete"} {
		t.Run(action, func(t *testing.T) {
			db, account, plan, subscription := seedCancelableAdminSubscription(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cancelAfterSubscriptionWrite := func(tx *gorm.DB) {
				if tx.Statement.Table == "user_subscriptions" && tx.Error == nil {
					cancel()
				}
			}
			// Cancel after SQL changed the transaction's rows, so this also
			// proves rollback rather than only rejecting an early cancellation.
			switch action {
			case "grant":
				require.NoError(t, db.Callback().Create().After("gorm:create").Register("test:cancel-subscription-write", cancelAfterSubscriptionWrite))
			case "reset", "invalidate":
				require.NoError(t, db.Callback().Update().After("gorm:update").Register("test:cancel-subscription-write", cancelAfterSubscriptionWrite))
			case "delete":
				require.NoError(t, db.Callback().Delete().After("gorm:delete").Register("test:cancel-subscription-write", cancelAfterSubscriptionWrite))
			}
			var err error
			switch action {
			case "grant":
				_, err = AdminBindSubscriptionContext(ctx, account.Id, plan.Id, "")
			case "reset":
				_, err = AdminResetUserSubscriptionsByPlanContext(ctx, account.Id, plan.Id, true)
			case "invalidate":
				_, err = AdminInvalidateUserSubscriptionContext(ctx, subscription.Id)
			case "delete":
				_, err = AdminDeleteUserSubscriptionContext(ctx, subscription.Id)
			}
			require.Error(t, err)
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			var persisted UserSubscription
			require.NoError(t, db.First(&persisted, subscription.Id).Error)
			require.Equal(t, subscription, persisted)
			var count int64
			require.NoError(t, db.Model(&UserSubscription{}).Where("user_id = ?", account.Id).Count(&count).Error)
			require.EqualValues(t, 1, count)
		})
	}
}
