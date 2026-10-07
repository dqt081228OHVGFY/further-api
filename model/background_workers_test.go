package model

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestModelSyncWorkersStopBeforeTheirNextInterval(t *testing.T) {
	for _, worker := range []struct {
		name string
		run  func(int)
	}{{"options", SyncOptions}, {"channels", SyncChannelCache}} {
		t.Run(worker.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			previous := common.BackgroundContext()
			common.SetBackgroundContext(ctx)
			defer common.SetBackgroundContext(previous)
			defer cancel()
			done := make(chan struct{})
			go func() {
				worker.run(3600)
				close(done)
			}()
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("worker waited for its full sync interval after cancellation")
			}
		})
	}
}

func resetQuotaStatisticsCache(t *testing.T) {
	t.Helper()
	CacheQuotaDataLock.Lock()
	previous := CacheQuotaData
	CacheQuotaData = make(map[string]*QuotaData)
	CacheQuotaDataLock.Unlock()
	t.Cleanup(func() {
		CacheQuotaDataLock.Lock()
		CacheQuotaData = previous
		CacheQuotaDataLock.Unlock()
	})
}

func TestFlushQuotaDataCacheRetainsFailedWritesForRetry(t *testing.T) {
	truncateTables(t)
	resetQuotaStatisticsCache(t)
	LogQuotaData(QuotaDataLogParams{UserID: 100, Username: "retry", ModelName: "model", CreatedAt: 3600, Quota: 7, TokenUsed: 11})
	const callback = "test:quota-create-failure"
	errWrite := errors.New("statistics persistence unavailable")
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "quota_data" {
			tx.AddError(errWrite)
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Create().Remove(callback) })
	require.ErrorIs(t, FlushQuotaDataCache(), errWrite)
	require.Len(t, CacheQuotaData, 1)
	for _, bucket := range CacheQuotaData {
		require.Zero(t, bucket.Id, "a failed insert must not mutate the retry bucket")
	}
	LogQuotaData(QuotaDataLogParams{UserID: 100, Username: "retry", ModelName: "model", CreatedAt: 3600, Quota: 5, TokenUsed: 3})
	require.NoError(t, DB.Callback().Create().Remove(callback))
	require.NoError(t, FlushQuotaDataCache())
	require.Empty(t, CacheQuotaData)
	var row QuotaData
	require.NoError(t, DB.Where("username = ?", "retry").First(&row).Error)
	require.Equal(t, 2, row.Count)
	require.Equal(t, 12, row.Quota)
	require.Equal(t, 14, row.TokenUsed)
	require.NoError(t, FlushQuotaDataCache())
	require.NoError(t, DB.Where("id = ?", row.Id).First(&row).Error)
	require.Equal(t, 12, row.Quota, "a repeated final flush must not duplicate accounting")
}

func TestFlushQuotaDataCacheRetainsFailedUpdatesForRetry(t *testing.T) {
	truncateTables(t)
	resetQuotaStatisticsCache(t)
	require.NoError(t, DB.Create(&QuotaData{UserID: 101, Username: "update-retry", ModelName: "model", CreatedAt: 3600, Count: 1, Quota: 10, TokenUsed: 4}).Error)
	LogQuotaData(QuotaDataLogParams{UserID: 101, Username: "update-retry", ModelName: "model", CreatedAt: 3600, Quota: 3, TokenUsed: 2})
	const callback = "test:quota-update-failure"
	errWrite := errors.New("statistics update unavailable")
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "quota_data" {
			tx.AddError(errWrite)
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Update().Remove(callback) })
	require.ErrorIs(t, FlushQuotaDataCache(), errWrite)
	require.Len(t, CacheQuotaData, 1)
	require.NoError(t, DB.Callback().Update().Remove(callback))
	require.NoError(t, FlushQuotaDataCache())
	var row QuotaData
	require.NoError(t, DB.Where("username = ?", "update-retry").First(&row).Error)
	require.Equal(t, 2, row.Count)
	require.Equal(t, 13, row.Quota)
	require.Equal(t, 6, row.TokenUsed)
}

func TestAuthArtifactCleanupQueriesHonorCancellation(t *testing.T) {
	truncateTables(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, deleteExpiredUserSessionsBeforeContext(ctx, time.Now().Unix(), time.Now().Unix(), time.Now().Unix()), context.Canceled)
	require.ErrorIs(t, deleteRevokedUserSessionsBeforeContext(ctx, time.Now().Unix(), time.Now().Unix()), context.Canceled)
	require.ErrorIs(t, DeleteExpiredAuthFlowsContext(ctx, time.Now()), context.Canceled)
}

func TestSubscriptionMaintenanceHonorsCancellation(t *testing.T) {
	truncateTables(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ExpireDueSubscriptionsContext(ctx, 200)
	require.ErrorIs(t, err, context.Canceled)
	_, err = ResetDueSubscriptionsContext(ctx, 200)
	require.ErrorIs(t, err, context.Canceled)
	_, err = CleanupSubscriptionPreConsumeRecordsContext(ctx, 3600)
	require.ErrorIs(t, err, context.Canceled)
}
