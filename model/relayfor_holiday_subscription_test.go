package model

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func holidayViewCampaign(t *testing.T, view map[string]any, key string) map[string]any {
	t.Helper()
	for _, campaign := range view["campaigns"].([]map[string]any) {
		if campaign["key"] == key {
			return campaign
		}
	}
	t.Fatalf("campaign %s missing from view", key)
	return nil
}

func TestRelayForHolidaySubscriptionPersistsAndIsIdempotent(t *testing.T) {
	activityTestDB(t)
	user := activityTestUser(t, "holiday-subscriber", 123)
	now := activityTime("2026-10-02T00:00:00Z")
	// A default calendar entry can be followed before a reward is configured.
	require.NoError(t, SetRelayForHolidaySubscription(user.Id, "chongyang", true, now))
	require.NoError(t, SetRelayForHolidaySubscription(user.Id, "chongyang", true, now.Add(time.Hour)))
	var records []RelayForHolidaySubscription
	require.NoError(t, DB.Find(&records).Error)
	require.Len(t, records, 1)
	require.Equal(t, now.Unix(), records[0].CreatedAt)
	view, err := RelayForActivitiesView(user.Id, now, false)
	require.NoError(t, err)
	campaign := holidayViewCampaign(t, view, "chongyang")
	require.Equal(t, true, campaign["subscribed"])
	require.Equal(t, true, campaign["can_subscribe"])
	require.Equal(t, false, campaign["can_claim"])
	other := activityTestUser(t, "holiday-observer", 0)
	view, err = RelayForActivitiesView(other.Id, now, false)
	require.NoError(t, err)
	require.Equal(t, false, holidayViewCampaign(t, view, "chongyang")["subscribed"])
	admin, err := RelayForActivitiesView(0, now, true)
	require.NoError(t, err)
	require.EqualValues(t, 1, holidayViewCampaign(t, admin, "chongyang")["subscription_count"])
	require.NoError(t, DB.First(&user, user.Id).Error)
	require.Equal(t, 123, user.Quota)
	require.NoError(t, SetRelayForHolidaySubscription(user.Id, "chongyang", false, now))
	require.NoError(t, SetRelayForHolidaySubscription(user.Id, "chongyang", false, now))
	require.NoError(t, DB.Find(&records).Error)
	require.Empty(t, records)
}

func TestRelayForHolidaySubscriptionDisabledAndGroupPolicy(t *testing.T) {
	activityTestDB(t)
	user := activityTestUser(t, "holiday-controls", 0)
	campaign := activityCampaign(t)
	campaign.TargetGroup = "vip"
	require.NoError(t, SaveRelayForHolidayCampaign(campaign))
	now := activityTime("2026-09-25T04:00:00Z")
	// Following is independent from the reward's eligibility group.
	require.NoError(t, SetRelayForHolidaySubscription(user.Id, campaign.Key, true, now))
	_, err := ClaimRelayForHoliday(user.Id, campaign.Key, now)
	require.ErrorContains(t, err, "条件")
	campaign.SubscriptionEnabled = false
	require.NoError(t, SaveRelayForHolidayCampaign(campaign))
	require.ErrorContains(t, SetRelayForHolidaySubscription(user.Id, campaign.Key, true, now), "未开放")
	view, err := RelayForActivitiesView(user.Id, now, false)
	require.NoError(t, err)
	item := holidayViewCampaign(t, view, campaign.Key)
	require.Equal(t, true, item["subscribed"])
	require.Equal(t, false, item["can_subscribe"])
	require.NoError(t, SetRelayForHolidaySubscription(user.Id, campaign.Key, false, now))
	campaign.SubscriptionEnabled, campaign.Enabled = true, false
	require.NoError(t, SaveRelayForHolidayCampaign(campaign))
	require.ErrorContains(t, SetRelayForHolidaySubscription(user.Id, campaign.Key, true, now), "未开放")
	require.NoError(t, SetRelayForHolidaySubscription(user.Id, campaign.Key, false, now))
	view, err = RelayForActivitiesView(user.Id, now, false)
	require.NoError(t, err)
	require.Len(t, view["campaigns"], 5)
	_, err = ClaimRelayForHoliday(user.Id, campaign.Key, now)
	require.Error(t, err)
	require.Error(t, SetRelayForHolidaySubscription(0, "chongyang", true, now))
	require.Error(t, SetRelayForHolidaySubscription(user.Id, "unknown", true, now))
	require.NoError(t, DB.Model(&user).Update("status", common.UserStatusDisabled).Error)
	require.Error(t, SetRelayForHolidaySubscription(user.Id, "chongyang", true, now))
}

func TestRelayForHolidaySubscriptionConcurrentUniquePreference(t *testing.T) {
	activityTestDB(t)
	user := activityTestUser(t, "holiday-follow-concurrent", 0)
	// SQLite serializes transactions on a single writer connection. PostgreSQL
	// and MySQL additionally lock the user row; the unique index backs all three.
	sqlDB, err := DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	var wait sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results <- SetRelayForHolidaySubscription(user.Id, "chongyang", true, activityTime("2026-10-02T00:00:00Z"))
		}()
	}
	wait.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	var count int64
	require.NoError(t, DB.Model(&RelayForHolidaySubscription{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestRelayForHolidayAndDailyCheckinAreIndependent(t *testing.T) {
	activityTestDB(t)
	previous := RelayForCheckinSettingsSnapshot()
	*operation_setting.GetCheckinSetting() = operation_setting.CheckinSetting{Enabled: true, MinQuota: 75, MaxQuota: 75}
	t.Cleanup(func() { *operation_setting.GetCheckinSetting() = previous })
	campaign := activityCampaign(t)
	now := activityTime("2026-09-24T16:00:00Z")
	for _, holidayFirst := range []bool{false, true} {
		name := "daily-before-holiday"
		if holidayFirst {
			name = "holiday-before-daily"
		}
		user := activityTestUser(t, name, 0)
		if holidayFirst {
			_, err := ClaimRelayForHoliday(user.Id, campaign.Key, now)
			require.NoError(t, err)
			_, err = userCheckinAt(user.Id, now)
			require.NoError(t, err)
		} else {
			_, err := userCheckinAt(user.Id, now)
			require.NoError(t, err)
			_, err = ClaimRelayForHoliday(user.Id, campaign.Key, now)
			require.NoError(t, err)
		}
		_, err := userCheckinAt(user.Id, now)
		require.Error(t, err)
		_, err = ClaimRelayForHoliday(user.Id, campaign.Key, now)
		require.Error(t, err)
		require.NoError(t, DB.First(&user, user.Id).Error)
		require.Equal(t, 175, user.Quota)
		var daily, holiday int64
		require.NoError(t, DB.Model(&Checkin{}).Where("user_id = ?", user.Id).Count(&daily).Error)
		require.NoError(t, DB.Model(&RelayForHolidayClaim{}).Where("user_id = ?", user.Id).Count(&holiday).Error)
		require.EqualValues(t, 1, daily)
		require.EqualValues(t, 1, holiday)
	}
}

func TestRelayForHolidayWindowsAndBudgetAcrossUsers(t *testing.T) {
	activityTestDB(t)
	campaign := activityCampaign(t)
	campaign.DaysBefore, campaign.DaysAfter = 2, 1
	campaign.DailyBudgetQuota = 150
	require.NoError(t, SaveRelayForHolidayCampaign(campaign))
	start := activityTime("2026-09-22T16:00:00Z")
	end := activityTime("2026-09-26T16:00:00Z")
	for _, test := range []struct {
		now    time.Time
		active bool
	}{{start.Add(-time.Nanosecond), false}, {start, true}, {end.Add(-time.Nanosecond), true}, {end, false}} {
		_, _, active := RelayForHolidayWindow(campaign, test.now)
		require.Equal(t, test.active, active)
	}
	first := activityTestUser(t, "budget-first", 0)
	second := activityTestUser(t, "budget-second", 0)
	_, err := ClaimRelayForHoliday(first.Id, campaign.Key, start)
	require.NoError(t, err)
	_, err = ClaimRelayForHoliday(second.Id, campaign.Key, start)
	require.ErrorContains(t, err, "预算")
	// A new Shanghai day has a new budget and independent per-user claim.
	_, err = ClaimRelayForHoliday(second.Id, campaign.Key, start.Add(24*time.Hour))
	require.NoError(t, err)
	view, err := RelayForActivitiesView(first.Id, end, false)
	require.NoError(t, err)
	item := holidayViewCampaign(t, view, campaign.Key)
	require.Equal(t, "ended", item["status"])
	require.Equal(t, "2027-09-15", item["next_date"])
	require.Equal(t, false, item["can_claim"])
}

func TestRelayForHolidayConfigurationContentAndPlanLinks(t *testing.T) {
	activityTestDB(t)
	campaign := activityCampaign(t)
	plan := SubscriptionPlan{Title: "中秋订阅", PriceAmount: 9.9}
	require.NoError(t, DB.Create(&plan).Error)
	campaign.SubscriptionPlanIDs = []int{plan.Id}
	campaign.Details = "  参加活动，收藏这个节日。\n活动额度每日独立领取。  "
	campaign.Rules = "  名额与预算以当日为准。  "
	require.NoError(t, SaveRelayForHolidayCampaign(campaign))
	var stored RelayForHolidayCampaign
	require.NoError(t, DB.First(&stored, "key = ?", campaign.Key).Error)
	require.Equal(t, strings.TrimSpace(campaign.Details), stored.Details)
	require.Equal(t, strings.TrimSpace(campaign.Rules), stored.Rules)
	require.Equal(t, []int{plan.Id}, stored.SubscriptionPlanIDs)
	campaign.SubscriptionPlanIDs = []int{plan.Id, plan.Id}
	require.Error(t, SaveRelayForHolidayCampaign(campaign))
	campaign.SubscriptionPlanIDs = []int{plan.Id + 500}
	require.ErrorContains(t, SaveRelayForHolidayCampaign(campaign), "不存在")
	campaign.SubscriptionPlanIDs = []int{-1}
	require.Error(t, SaveRelayForHolidayCampaign(campaign))
	campaign.SubscriptionPlanIDs = nil
	campaign.Details = strings.Repeat("节", 4001)
	require.Error(t, SaveRelayForHolidayCampaign(campaign))
	campaign.Details, campaign.Rules = "", strings.Repeat("节", 2001)
	require.Error(t, SaveRelayForHolidayCampaign(campaign))
}

func TestRelayForHolidaySchemaUpgradePreservesCampaignAndClaim(t *testing.T) {
	setupRelayForBenefitsDB(t)
	// Simulate the pre-feature schema, then apply the same AutoMigrate models
	// registered by migrateDB. Existing rewards and claims must be preserved.
	require.NoError(t, DB.Exec("CREATE TABLE relay_for_holiday_campaigns (key text PRIMARY KEY, title text, description text, enabled numeric, days_before integer, days_after integer, reward_quota integer, daily_claim_limit integer, daily_budget_quota integer, target_group text, updated_by integer, updated_at integer)").Error)
	require.NoError(t, DB.Exec("INSERT INTO relay_for_holiday_campaigns (key,title,enabled,reward_quota) VALUES ('mid_autumn','中秋',1,100)").Error)
	require.NoError(t, DB.AutoMigrate(&RelayForHolidayClaim{}))
	record := RelayForHolidayClaim{CampaignKey: "mid_autumn", UserID: 42, ClaimDate: "2026-09-25", RewardQuota: 100}
	require.NoError(t, DB.Create(&record).Error)
	require.NoError(t, DB.AutoMigrate(&RelayForHolidayCampaign{}, &RelayForHolidayClaim{}, &RelayForHolidaySubscription{}))
	var campaign RelayForHolidayCampaign
	require.NoError(t, DB.First(&campaign, "key = ?", "mid_autumn").Error)
	require.Equal(t, 100, campaign.RewardQuota)
	require.True(t, campaign.Enabled)
	require.False(t, campaign.SubscriptionEnabled)
	var count int64
	require.NoError(t, DB.Model(&RelayForHolidayClaim{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.True(t, DB.Migrator().HasIndex(&RelayForHolidaySubscription{}, "idx_holiday_subscription"))
	subscription := RelayForHolidaySubscription{CampaignKey: "mid_autumn", UserID: 42}
	require.NoError(t, DB.Create(&subscription).Error)
	subscription.ID = 0
	require.Error(t, DB.Create(&subscription).Error)
}

func TestRelayForNationalDayCalendarWindowAndIndependentReward(t *testing.T) {
	activityTestDB(t)
	user := activityTestUser(t, "national-day-user", 0)
	now := activityTime("2026-10-02T04:00:00Z")
	view, err := RelayForActivitiesView(user.Id, now, false)
	require.NoError(t, err)
	require.Equal(t, "national_day", view["theme"])
	item := holidayViewCampaign(t, view, "national_day")
	require.Equal(t, "2026-10-01", item["date"])
	require.Equal(t, true, item["active"])
	require.Equal(t, true, item["subscription_enabled"])
	require.Equal(t, false, item["can_claim"], "default calendar never grants unconfigured rewards")
	require.Equal(t, "2026-10-08T00:00:00+08:00", item["ends_at"])
	require.NoError(t, SetRelayForHolidaySubscription(user.Id, "national_day", true, now))
	var campaign RelayForHolidayCampaign
	for _, candidate := range RelayForDefaultHolidayCampaigns() {
		if candidate.Key == "national_day" {
			campaign = candidate
		}
	}
	campaign.RewardQuota = 100
	require.NoError(t, SaveRelayForHolidayCampaign(campaign))
	_, err = ClaimRelayForHoliday(user.Id, campaign.Key, now)
	require.NoError(t, err)
	_, err = ClaimRelayForHoliday(user.Id, campaign.Key, now)
	require.Error(t, err)
	_, _, active := RelayForHolidayWindow(campaign, activityTime("2026-10-07T16:00:00Z"))
	require.False(t, active)
}
