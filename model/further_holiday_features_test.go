package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
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

func assertNoHolidayFollowFields(t *testing.T, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	for _, field := range []string{"subscription_enabled", "subscribed", "can_subscribe", "subscription_count"} {
		require.NotContains(t, string(encoded), `"`+field+`"`, "removed free-follow state must not be exposed")
	}
}

func TestFurtherHolidayViewsAndRewardsWithoutFollowStorage(t *testing.T) {
	activityTestDB(t)
	require.False(t, DB.Migrator().HasTable("further_holiday_subscriptions"))
	require.False(t, DB.Migrator().HasColumn(&FurtherHolidayCampaign{}, "subscription_enabled"))
	user := activityTestUser(t, "holiday-no-follow-storage", 0)
	campaign := activityCampaign(t)
	plan := SubscriptionPlan{Title: "节日付费权益", PriceAmount: 12}
	require.NoError(t, DB.Create(&plan).Error)
	campaign.SubscriptionPlanIDs = []int{plan.Id}
	require.NoError(t, SaveFurtherHolidayCampaign(campaign))
	assertNoHolidayFollowFields(t, campaign)
	assertNoHolidayFollowFields(t, FurtherDefaultHolidayCampaigns())
	now := activityTime("2026-09-25T04:00:00Z")
	for _, request := range []struct {
		userID int
		admin  bool
	}{{0, false}, {user.Id, false}, {0, true}} {
		view, err := FurtherActivitiesView(request.userID, now, request.admin)
		require.NoError(t, err)
		assertNoHolidayFollowFields(t, view)
		item := holidayViewCampaign(t, view, campaign.Key)
		require.Equal(t, []int{plan.Id}, item["subscription_plan_ids"])
		require.Equal(t, "2026-09-25T00:00:00+08:00", item["starts_at"])
		require.Equal(t, "2026-09-26T00:00:00+08:00", item["ends_at"])
	}
	_, err := ClaimFurtherHoliday(user.Id, campaign.Key, now)
	require.NoError(t, err)
	view, err := FurtherActivitiesView(user.Id, now, false)
	require.NoError(t, err)
	require.Len(t, view["history"], 1)
	assertNoHolidayFollowFields(t, view)
	require.NoError(t, DB.First(&user, user.Id).Error)
	require.Equal(t, 100, user.Quota)
}

func TestFurtherHolidayAndDailyCheckinAreIndependent(t *testing.T) {
	activityTestDB(t)
	previous := FurtherCheckinSettingsSnapshot()
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
			_, err := ClaimFurtherHoliday(user.Id, campaign.Key, now)
			require.NoError(t, err)
			_, err = userCheckinAt(user.Id, now)
			require.NoError(t, err)
		} else {
			_, err := userCheckinAt(user.Id, now)
			require.NoError(t, err)
			_, err = ClaimFurtherHoliday(user.Id, campaign.Key, now)
			require.NoError(t, err)
		}
		_, err := userCheckinAt(user.Id, now)
		require.Error(t, err)
		_, err = ClaimFurtherHoliday(user.Id, campaign.Key, now)
		require.Error(t, err)
		require.NoError(t, DB.First(&user, user.Id).Error)
		require.Equal(t, 175, user.Quota)
		var daily, holiday int64
		require.NoError(t, DB.Model(&Checkin{}).Where("user_id = ?", user.Id).Count(&daily).Error)
		require.NoError(t, DB.Model(&FurtherHolidayClaim{}).Where("user_id = ?", user.Id).Count(&holiday).Error)
		require.EqualValues(t, 1, daily)
		require.EqualValues(t, 1, holiday)
	}
}

func TestFurtherHolidayWindowsAndBudgetAcrossUsers(t *testing.T) {
	activityTestDB(t)
	campaign := activityCampaign(t)
	campaign.DaysBefore, campaign.DaysAfter = 2, 1
	campaign.DailyBudgetQuota = 150
	require.NoError(t, SaveFurtherHolidayCampaign(campaign))
	start := activityTime("2026-09-22T16:00:00Z")
	end := activityTime("2026-09-26T16:00:00Z")
	for _, test := range []struct {
		now    time.Time
		active bool
	}{{start.Add(-time.Nanosecond), false}, {start, true}, {end.Add(-time.Nanosecond), true}, {end, false}} {
		_, _, active := FurtherHolidayWindow(campaign, test.now)
		require.Equal(t, test.active, active)
	}
	first := activityTestUser(t, "budget-first", 0)
	second := activityTestUser(t, "budget-second", 0)
	_, err := ClaimFurtherHoliday(first.Id, campaign.Key, start)
	require.NoError(t, err)
	_, err = ClaimFurtherHoliday(second.Id, campaign.Key, start)
	require.ErrorContains(t, err, "预算")
	// A new Shanghai day has a new budget and independent per-user claim.
	_, err = ClaimFurtherHoliday(second.Id, campaign.Key, start.Add(24*time.Hour))
	require.NoError(t, err)
	view, err := FurtherActivitiesView(first.Id, end, false)
	require.NoError(t, err)
	item := holidayViewCampaign(t, view, campaign.Key)
	require.Equal(t, "ended", item["status"])
	require.Equal(t, "2027-09-15", item["next_date"])
	require.Equal(t, false, item["can_claim"])
}

func TestFurtherHolidayConfigurationContentAndPlanLinks(t *testing.T) {
	activityTestDB(t)
	campaign := activityCampaign(t)
	plan := SubscriptionPlan{Title: "中秋订阅", PriceAmount: 9.9}
	require.NoError(t, DB.Create(&plan).Error)
	campaign.SubscriptionPlanIDs = []int{plan.Id}
	campaign.Details = "  参加活动，收藏这个节日。\n活动额度每日独立领取。  "
	campaign.Rules = "  名额与预算以当日为准。  "
	require.NoError(t, SaveFurtherHolidayCampaign(campaign))
	var stored FurtherHolidayCampaign
	require.NoError(t, DB.First(&stored, "key = ?", campaign.Key).Error)
	require.Equal(t, strings.TrimSpace(campaign.Details), stored.Details)
	require.Equal(t, strings.TrimSpace(campaign.Rules), stored.Rules)
	require.Equal(t, []int{plan.Id}, stored.SubscriptionPlanIDs)
	campaign.SubscriptionPlanIDs = []int{plan.Id, plan.Id}
	require.Error(t, SaveFurtherHolidayCampaign(campaign))
	campaign.SubscriptionPlanIDs = []int{plan.Id + 500}
	require.ErrorContains(t, SaveFurtherHolidayCampaign(campaign), "不存在")
	campaign.SubscriptionPlanIDs = []int{-1}
	require.Error(t, SaveFurtherHolidayCampaign(campaign))
	campaign.SubscriptionPlanIDs = nil
	campaign.Details = strings.Repeat("节", 4001)
	require.Error(t, SaveFurtherHolidayCampaign(campaign))
	campaign.Details, campaign.Rules = "", strings.Repeat("节", 2001)
	require.Error(t, SaveFurtherHolidayCampaign(campaign))
}

func TestFurtherHolidaySchemaUpgradePreservesCampaignAndClaim(t *testing.T) {
	setupFurtherBenefitsDB(t)
	// Simulate the pre-feature schema, then apply the same AutoMigrate models
	// registered by migrateDB. Existing rewards and claims must be preserved.
	require.NoError(t, DB.Exec("CREATE TABLE further_holiday_campaigns (key text PRIMARY KEY, title text, description text, enabled numeric, subscription_enabled numeric, days_before integer, days_after integer, reward_quota integer, daily_claim_limit integer, daily_budget_quota integer, target_group text, updated_by integer, updated_at integer)").Error)
	require.NoError(t, DB.Exec("INSERT INTO further_holiday_campaigns (key,title,enabled,reward_quota) VALUES ('mid_autumn','中秋',1,100)").Error)
	require.NoError(t, DB.Exec("CREATE TABLE further_holiday_subscriptions (id integer PRIMARY KEY, campaign_key text, user_id integer, created_at integer)").Error)
	require.NoError(t, DB.Exec("INSERT INTO further_holiday_subscriptions (id,campaign_key,user_id,created_at) VALUES (17,'mid_autumn',42,1800000000)").Error)
	require.NoError(t, DB.AutoMigrate(&FurtherHolidayClaim{}))
	record := FurtherHolidayClaim{CampaignKey: "mid_autumn", UserID: 42, ClaimDate: "2026-09-25", RewardQuota: 100}
	require.NoError(t, DB.Create(&record).Error)
	require.NoError(t, DB.AutoMigrate(&FurtherHolidayCampaign{}, &FurtherHolidayClaim{}))
	var campaign FurtherHolidayCampaign
	require.NoError(t, DB.First(&campaign, "key = ?", "mid_autumn").Error)
	require.Equal(t, 100, campaign.RewardQuota)
	require.True(t, campaign.Enabled)
	var count int64
	require.NoError(t, DB.Model(&FurtherHolidayClaim{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	// Historic rows remain untouched; current views and rewards do not depend
	// on this retired table or the obsolete campaign column.
	var rows []map[string]any
	require.NoError(t, DB.Table("further_holiday_subscriptions").Find(&rows).Error)
	require.Len(t, rows, 1)
	before, err := json.Marshal(rows)
	require.NoError(t, err)
	user := activityTestUser(t, "holiday-retired-storage", 0)
	require.NoError(t, SaveFurtherHolidayCampaign(campaign))
	view, err := FurtherActivitiesView(user.Id, activityTime("2026-09-25T04:00:00Z"), false)
	require.NoError(t, err)
	assertNoHolidayFollowFields(t, view)
	_, err = ClaimFurtherHoliday(user.Id, campaign.Key, activityTime("2026-09-25T04:00:00Z"))
	require.NoError(t, err)
	rows = nil
	require.NoError(t, DB.Table("further_holiday_subscriptions").Find(&rows).Error)
	after, err := json.Marshal(rows)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}

func TestFurtherNationalDayCalendarWindowAndIndependentReward(t *testing.T) {
	activityTestDB(t)
	user := activityTestUser(t, "national-day-user", 0)
	now := activityTime("2026-10-02T04:00:00Z")
	view, err := FurtherActivitiesView(user.Id, now, false)
	require.NoError(t, err)
	require.Equal(t, "national_day", view["theme"])
	item := holidayViewCampaign(t, view, "national_day")
	require.Equal(t, "2026-10-01", item["date"])
	require.Equal(t, true, item["active"])
	require.Equal(t, false, item["can_claim"], "default calendar never grants unconfigured rewards")
	require.Equal(t, "2026-10-08T00:00:00+08:00", item["ends_at"])
	var campaign FurtherHolidayCampaign
	for _, candidate := range FurtherDefaultHolidayCampaigns() {
		if candidate.Key == "national_day" {
			campaign = candidate
		}
	}
	campaign.RewardQuota = 100
	require.NoError(t, SaveFurtherHolidayCampaign(campaign))
	_, err = ClaimFurtherHoliday(user.Id, campaign.Key, now)
	require.NoError(t, err)
	_, err = ClaimFurtherHoliday(user.Id, campaign.Key, now)
	require.Error(t, err)
	_, _, active := FurtherHolidayWindow(campaign, activityTime("2026-10-07T16:00:00Z"))
	require.False(t, active)
}
