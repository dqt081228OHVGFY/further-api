package model

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetAllUserSubscriptionsPreservesPlanTitlesAndMarksInternalFunding(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&FurtherSubscriptionPolicy{}, &FurtherPrepaidCard{}))

	publicPlan := SubscriptionPlan{Title: "Retired VIP"}
	internalPlan := SubscriptionPlan{Title: "Internal benefit"}
	prepaidPlan := SubscriptionPlan{Title: "Prepaid card quota"}
	for _, plan := range []*SubscriptionPlan{&publicPlan, &internalPlan, &prepaidPlan} {
		require.NoError(t, DB.Create(plan).Error)
		require.NoError(t, DB.Model(plan).Update("enabled", false).Error)
	}
	planIDs := []int{publicPlan.Id, internalPlan.Id, prepaidPlan.Id}
	t.Cleanup(func() {
		DB.Unscoped().Where("plan_id IN ?", planIDs).Delete(&FurtherPrepaidCard{})
		DB.Where("plan_id IN ?", planIDs).Delete(&FurtherSubscriptionPolicy{})
	})
	require.NoError(t, DB.Create(&FurtherSubscriptionPolicy{
		PlanID: internalPlan.Id, Code: "summary-internal", Internal: true,
	}).Error)
	card := FurtherPrepaidCard{Code: "summary-prepaid", CodeHash: "summary-prepaid-hash", PlanID: prepaidPlan.Id, Status: 1}
	require.NoError(t, DB.Create(&card).Error)
	require.NoError(t, DB.Delete(&card).Error) // Historical cards still identify the funding plan.

	now := time.Now().Unix()
	subs := []UserSubscription{
		{UserId: 80101, PlanId: publicPlan.Id, Status: "expired", EndTime: now - 300},
		{UserId: 80101, PlanId: publicPlan.Id, Status: "active", EndTime: now + 300},
		{UserId: 80101, PlanId: internalPlan.Id, Status: "active", EndTime: now + 200},
		{UserId: 80101, PlanId: prepaidPlan.Id, Status: "active", Source: "further_prepaid", EndTime: now + 100},
		{UserId: 80101, PlanId: 999991, Status: "expired", EndTime: now - 400},
		{UserId: 80102, PlanId: publicPlan.Id, Status: "active", EndTime: now + 400},
	}
	require.NoError(t, DB.Create(&subs).Error)

	summaries, err := GetAllUserSubscriptions(80101)
	require.NoError(t, err)
	require.Len(t, summaries, 5)
	byPlan := make(map[int][]SubscriptionSummary)
	for _, summary := range summaries {
		byPlan[summary.Subscription.PlanId] = append(byPlan[summary.Subscription.PlanId], summary)
		assert.Equal(t, 80101, summary.Subscription.UserId)
	}
	require.Len(t, byPlan[publicPlan.Id], 2)
	for _, summary := range byPlan[publicPlan.Id] {
		assert.Equal(t, "Retired VIP", summary.PlanTitle)
		assert.False(t, summary.PlanInternal, "disabled public plans must remain visible")
	}
	assert.Equal(t, "Internal benefit", byPlan[internalPlan.Id][0].PlanTitle)
	assert.True(t, byPlan[internalPlan.Id][0].PlanInternal)
	assert.Equal(t, "Prepaid card quota", byPlan[prepaidPlan.Id][0].PlanTitle)
	assert.True(t, byPlan[prepaidPlan.Id][0].PlanInternal)
	assert.Empty(t, byPlan[999991][0].PlanTitle)
	assert.False(t, byPlan[999991][0].PlanInternal)

	encoded, err := json.Marshal(byPlan[prepaidPlan.Id][0])
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"plan_title":"Prepaid card quota"`)
	assert.Contains(t, string(encoded), `"plan_internal":true`)

	active, err := GetAllActiveUserSubscriptions(80101)
	require.NoError(t, err)
	require.Len(t, active, 3)
	activeByPlan := make(map[int]SubscriptionSummary, len(active))
	for _, summary := range active {
		activeByPlan[summary.Subscription.PlanId] = summary
	}
	assert.Equal(t, "Retired VIP", activeByPlan[publicPlan.Id].PlanTitle)
	assert.False(t, activeByPlan[publicPlan.Id].PlanInternal)
	assert.Equal(t, "Internal benefit", activeByPlan[internalPlan.Id].PlanTitle)
	assert.True(t, activeByPlan[internalPlan.Id].PlanInternal)
	assert.Equal(t, "Prepaid card quota", activeByPlan[prepaidPlan.Id].PlanTitle)
	assert.True(t, activeByPlan[prepaidPlan.Id].PlanInternal)
}

func TestSubscriptionSummariesWorkBeforeFurtherMetadataTablesExist(t *testing.T) {
	previousDB := DB
	legacyDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = legacyDB
	sqlDB, err := legacyDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		DB = previousDB
		_ = sqlDB.Close()
	})
	require.NoError(t, legacyDB.AutoMigrate(&SubscriptionPlan{}, &UserSubscription{}))
	assert.False(t, legacyDB.Migrator().HasTable(&FurtherSubscriptionPolicy{}))
	assert.False(t, legacyDB.Migrator().HasTable(&FurtherPrepaidCard{}))
	plan := SubscriptionPlan{Title: "Legacy VIP"}
	require.NoError(t, legacyDB.Create(&plan).Error)
	now := time.Now().Unix()
	require.NoError(t, legacyDB.Create(&UserSubscription{
		UserId: 101, PlanId: plan.Id, Status: "active", EndTime: now + 3600,
	}).Error)
	for _, list := range []func(int) ([]SubscriptionSummary, error){GetAllUserSubscriptions, GetAllActiveUserSubscriptions} {
		summaries, err := list(101)
		require.NoError(t, err)
		require.Len(t, summaries, 1)
		assert.Equal(t, "Legacy VIP", summaries[0].PlanTitle)
		assert.False(t, summaries[0].PlanInternal)
	}
}
