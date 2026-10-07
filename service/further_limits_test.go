package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func furtherLimitRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	server := miniredis.RunT(t)
	server.SetTime(time.Unix(1800000000, 0))
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	priorClient, priorEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() { common.RDB, common.RedisEnabled = priorClient, priorEnabled; _ = client.Close() })
	return server
}

func TestFurtherRedisLimitsHoldConcurrencyUntilReleaseAndDoNotRefundRPM(t *testing.T) {
	furtherLimitRedis(t)
	first, err := AcquireFurtherLimits(context.Background(), "user:100:subscription:7", 2, 1)
	require.NoError(t, err)
	t.Cleanup(first.Release)
	_, err = AcquireFurtherLimits(context.Background(), "user:100:subscription:7", 2, 1)
	var limited *FurtherLimitError
	require.ErrorAs(t, err, &limited)
	require.Equal(t, "concurrency", limited.Reason)
	first.Release()
	first.Release()
	second, err := AcquireFurtherLimits(context.Background(), "user:100:subscription:7", 2, 1)
	require.NoError(t, err)
	second.Release()
	_, err = AcquireFurtherLimits(context.Background(), "user:100:subscription:7", 2, 1)
	require.ErrorAs(t, err, &limited)
	require.Equal(t, "rpm", limited.Reason)
}

func TestFurtherLostRedisLeaseCancelsStreamingContext(t *testing.T) {
	server := furtherLimitRedis(t)
	lease, err := AcquireFurtherLimits(context.Background(), "channel:3", 0, 1)
	require.NoError(t, err)
	t.Cleanup(lease.Release)
	server.Del("{further:limits:channel:3}:leases")
	require.False(t, lease.renewOrCancel())
	require.ErrorIs(t, lease.Context().Err(), context.Canceled)
	newLease, err := AcquireFurtherLimits(context.Background(), "channel:3", 0, 1)
	require.NoError(t, err)
	newLease.Release()
}

func TestFurtherRedisOutageDoesNotSwitchToIndependentLocalCounters(t *testing.T) {
	priorClient, priorEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = nil, true
	t.Cleanup(func() { common.RDB, common.RedisEnabled = priorClient, priorEnabled })
	_, err := AcquireFurtherLimits(context.Background(), "channel:4", 10, 2)
	var limited *FurtherLimitError
	require.ErrorAs(t, err, &limited)
	require.Equal(t, "unavailable", limited.Reason)
}

func TestFurtherUnconfiguredNativeRequestDoesNotChangeOrCancelContext(t *testing.T) {
	ctx := context.Background()
	lease, err := AcquireFurtherLimits(ctx, "user:1:subscription:1", 0, 0)
	require.NoError(t, err)
	require.Equal(t, ctx, lease.Context())
	lease.Release()
	require.NoError(t, ctx.Err())
}

func TestFurtherLocalLimitsReserveAtomicallyAcrossParallelRequests(t *testing.T) {
	store := furtherMemoryStore{subjects: make(map[string]*furtherMemorySubject)}
	var wg sync.WaitGroup
	accepted := make(chan string, 24)
	for index := range 24 {
		wg.Go(func() {
			id := fmt.Sprint(index)
			if store.take("user:5", id, 10, 3, 120000) == nil {
				accepted <- id
			}
		})
	}
	wg.Wait()
	close(accepted)
	require.Len(t, accepted, 3)
	for id := range accepted {
		store.release("user:5", id)
		store.release("user:5", id)
	}
	require.Zero(t, store.leases)
	require.NoError(t, store.take("user:5", "next", 10, 3, 180000))
	require.Equal(t, 1, store.subjects["user:5"].count)
}

func TestFurtherModelAccessKeepsNativeModelsPublicAndRequiresExplicitEntitlement(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previous := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previous; _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.FurtherModelAccess{}, &model.UserSubscription{}, &model.FurtherSubscriptionPolicy{}, &model.FurtherUserSubscriptionPolicy{}))
	metadata := model.Model{ModelName: "private-model", NameRule: model.NameRuleExact}
	require.NoError(t, db.Create(&metadata).Error)
	_, err = model.SaveFurtherModelAccess(model.FurtherModelAccess{ModelID: metadata.Id, Model: metadata.ModelName, SubscriptionOnly: true})
	require.NoError(t, err)
	allowed, err := model.FurtherSubscriptionModelAllowed(11, "ordinary-native-model")
	require.NoError(t, err)
	require.True(t, allowed)
	sub := model.UserSubscription{UserId: 11, PlanId: 9, Status: "active", StartTime: common.GetTimestamp() - 1, EndTime: common.GetTimestamp() + 3600}
	require.NoError(t, db.Create(&sub).Error)
	allowed, err = model.FurtherSubscriptionModelAllowed(11, "private-model")
	require.NoError(t, err)
	require.False(t, allowed, "native subscription without explicit model rule does not grant a private model")
	rules, err := common.Marshal([]model.FurtherSubscriptionModelRule{{ModelID: metadata.Id, Model: metadata.ModelName, Multiplier: 1}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.FurtherUserSubscriptionPolicy{SubscriptionID: sub.Id, PlanID: sub.PlanId, DiscountMultiplier: 1, ModelRulesJSON: string(rules)}).Error)
	allowed, err = model.FurtherSubscriptionModelAllowed(11, "private-model")
	require.NoError(t, err)
	require.True(t, allowed)
	require.NoError(t, db.Model(&sub).Update("end_time", common.GetTimestamp()-1).Error)
	allowed, err = model.FurtherSubscriptionModelAllowed(11, "private-model")
	require.NoError(t, err)
	require.False(t, allowed)
}
