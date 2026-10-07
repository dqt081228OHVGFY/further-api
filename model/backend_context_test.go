package model

import (
	"context"
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"path/filepath"
	"testing"
	"time"
)

func backendContextDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "context.db")), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLog, previousRedis := DB, LOG_DB, common.RedisEnabled
	DB, LOG_DB, common.RedisEnabled = db, db, false
	connection, err := db.DB()
	require.NoError(t, err)
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() {
		DB, LOG_DB, common.RedisEnabled = previousDB, previousLog, previousRedis
		require.NoError(t, connection.Close())
	})
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}))
	return db
}

func TestBackendContextCanceledPrechargeNeverTouchesDatabase(t *testing.T) {
	db := businessCommandDatabase(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	queries, updates := 0, 0
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:context-query", func(*gorm.DB) { queries++ }))
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:context-update", func(*gorm.DB) { updates++ }))
	_, err := TryReserveUserQuotaContext(ctx, 1, 50)
	require.ErrorIs(t, err, context.Canceled)
	_, err = TryReserveTokenQuotaContext(ctx, 1, "context-token", 50, false)
	require.ErrorIs(t, err, context.Canceled)
	_, err = PreConsumeUserSubscriptionContext(ctx, "context-subscription", 1, "model", 0, 50)
	require.ErrorIs(t, err, context.Canceled)
	_, err = GetUserQuotaContext(ctx, 1, false)
	require.ErrorIs(t, err, context.Canceled)
	_, err = GetTokenByKeyContext(ctx, "context-token", false)
	require.ErrorIs(t, err, context.Canceled)
	_, err = FurtherSubscriptionModelAllowedContext(ctx, 1, "model")
	require.ErrorIs(t, err, context.Canceled)
	_, err = HasActiveUserSubscriptionContext(ctx, 1)
	require.ErrorIs(t, err, context.Canceled)
	_, err = UserActiveSubscriptionsAllowWalletOverflowContext(ctx, 1)
	require.ErrorIs(t, err, context.Canceled)
	_, err = GetSubscriptionPlanInfoByUserSubscriptionIdContext(ctx, 1)
	require.ErrorIs(t, err, context.Canceled)
	_, err = GetFurtherDiscountReservationContext(ctx, "context-discount")
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, queries)
	require.Zero(t, updates)
}

func TestBackendContextRefundUsesIndependentRecoveryContext(t *testing.T) {
	db := businessCommandDatabase(t)
	user := User{Username: "recovery-context", AffCode: "recovery-context", Quota: 100}
	require.NoError(t, db.Create(&user).Error)
	customer, cancel := context.WithCancel(context.Background())
	reserved, err := TryReserveUserQuotaContext(customer, user.Id, 60)
	require.NoError(t, err)
	require.True(t, reserved)
	cancel()
	recovery, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	require.NoError(t, IncreaseUserQuotaContext(recovery, user.Id, 60, true))
	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	require.Equal(t, 100, stored.Quota)
}

func TestBackendContextPersistenceCancellationCompensatesWarmRedis(t *testing.T) {
	db := backendContextDatabase(t)
	useUserCacheMiniRedis(t)
	previousBatch := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = false
	t.Cleanup(func() { common.BatchUpdateEnabled = previousBatch })
	user := User{Username: "context-cache", AffCode: "context-cache", Quota: 100, AuthVersion: 1}
	require.NoError(t, db.Create(&user).Error)
	_, err := GetUserCacheContext(context.Background(), user.Id)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:cancel-ledger-write", func(*gorm.DB) { cancel() }))
	reserved, err := TryReserveUserQuotaContext(ctx, user.Id, 60)
	require.False(t, reserved)
	require.ErrorIs(t, err, context.Canceled)
	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	require.Equal(t, 100, stored.Quota)
	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	require.Equal(t, 100, cached.Quota)
}
