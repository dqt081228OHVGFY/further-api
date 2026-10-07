package model

import (
	"sync"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func activityTestDB(t *testing.T) {
	db := setupFurtherBenefitsDB(t)
	require.NoError(t, db.AutoMigrate(&FurtherHolidayCampaign{}, &FurtherHolidayClaim{}, &SubscriptionPlan{}, &Checkin{}))
}
func activityTestUser(t *testing.T, name string, quota int) User {
	t.Helper()
	user := User{Username: name, AffCode: name, Password: "unused", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Quota: quota}
	require.NoError(t, DB.Create(&user).Error)
	return user
}
func activityTime(value string) time.Time { now, _ := time.Parse(time.RFC3339, value); return now }
func activityCampaign(t *testing.T) FurtherHolidayCampaign {
	c := FurtherDefaultHolidayCampaigns()[2]
	c.RewardQuota = 100
	require.NoError(t, SaveFurtherHolidayCampaign(c))
	return c
}

func TestFurtherHolidayClaimServerDayAndOnce(t *testing.T) {
	activityTestDB(t)
	user := activityTestUser(t, "holiday-user", 0)
	activityCampaign(t)
	// UTC is still Sep 24, but the campaign day has started in Shanghai.
	now := activityTime("2026-09-24T16:01:00Z")
	record, err := ClaimFurtherHoliday(user.Id, "mid_autumn", now)
	require.NoError(t, err)
	require.Equal(t, "2026-09-25", record.ClaimDate)
	_, err = ClaimFurtherHoliday(user.Id, "mid_autumn", now.Add(time.Hour))
	require.Error(t, err)
	require.NoError(t, DB.First(&user, user.Id).Error)
	require.Equal(t, 100, user.Quota)
	_, err = ClaimFurtherHoliday(user.Id, "mid_autumn", activityTime("2026-09-25T16:00:00Z"))
	require.Error(t, err)
}

func TestFurtherHolidayClaimBudgetGroupAndRollback(t *testing.T) {
	activityTestDB(t)
	first := activityTestUser(t, "holiday-first", 0)
	second := activityTestUser(t, "holiday-second", 0)
	c := activityCampaign(t)
	c.DailyBudgetQuota = 100
	require.NoError(t, SaveFurtherHolidayCampaign(c))
	now := activityTime("2026-09-25T04:00:00Z")
	_, err := ClaimFurtherHoliday(first.Id, c.Key, now)
	require.NoError(t, err)
	_, err = ClaimFurtherHoliday(second.Id, c.Key, now)
	require.ErrorContains(t, err, "预算")
	var count int64
	require.NoError(t, DB.Model(&FurtherHolidayClaim{}).Where("user_id = ?", second.Id).Count(&count).Error)
	require.Zero(t, count)
	c.DailyBudgetQuota = 0
	c.TargetGroup = "paid"
	require.NoError(t, SaveFurtherHolidayCampaign(c))
	_, err = ClaimFurtherHoliday(second.Id, c.Key, now)
	require.ErrorContains(t, err, "条件")
	c.TargetGroup = ""
	require.NoError(t, SaveFurtherHolidayCampaign(c))
	require.NoError(t, DB.Model(&User{}).Where("id = ?", second.Id).Update("quota", common.MaxWalletQuota-50).Error)
	_, err = ClaimFurtherHoliday(second.Id, c.Key, now)
	require.Error(t, err)
	require.NoError(t, DB.Model(&FurtherHolidayClaim{}).Where("user_id = ?", second.Id).Count(&count).Error)
	require.Zero(t, count)
}

func TestFurtherHolidayClaimConcurrencyAndLimits(t *testing.T) {
	activityTestDB(t)
	user := activityTestUser(t, "holiday-concurrent", 0)
	c := activityCampaign(t)
	c.DailyClaimLimit = 1
	require.NoError(t, SaveFurtherHolidayCampaign(c))
	now := activityTime("2026-09-25T04:00:00Z")
	var wait sync.WaitGroup
	results := make(chan error, 8)
	for index := 0; index < 8; index++ {
		wait.Add(1)
		go func() { defer wait.Done(); _, err := ClaimFurtherHoliday(user.Id, c.Key, now); results <- err }()
	}
	wait.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	require.Equal(t, 1, successes)
	require.NoError(t, DB.First(&user, user.Id).Error)
	require.Equal(t, 100, user.Quota)
	other := activityTestUser(t, "holiday-other", 0)
	_, err := ClaimFurtherHoliday(other.Id, c.Key, now)
	require.ErrorContains(t, err, "名额")
}

func TestFurtherHolidayYearRoundPreviewAndConfiguration(t *testing.T) {
	activityTestDB(t)
	view, err := FurtherActivitiesView(0, activityTime("2026-10-10T00:00:00Z"), false)
	require.NoError(t, err)
	require.Len(t, view["campaigns"], 6)
	for _, campaign := range view["campaigns"].([]map[string]any) {
		require.Equal(t, false, campaign["active"])
		require.Equal(t, false, campaign["can_claim"])
		require.NotEmpty(t, campaign["next_date"])
		require.NotEmpty(t, campaign["starts_at"])
	}
	require.Empty(t, view["theme"])
	c := activityCampaign(t)
	c.DaysAfter = 7
	require.NoError(t, SaveFurtherHolidayCampaign(c))
	view, err = FurtherActivitiesView(0, activityTime("2026-10-01T00:00:00Z"), false)
	require.NoError(t, err)
	require.Equal(t, "mid_autumn", view["theme"])
	c.DaysAfter = 8
	require.Error(t, SaveFurtherHolidayCampaign(c))
	c.DaysAfter = 0
	c.RewardQuota = -1
	require.Error(t, SaveFurtherHolidayCampaign(c))
	_, err = ClaimFurtherHoliday(1, "", activityTime("2026-09-25T04:00:00Z"))
	require.ErrorContains(t, err, "不存在")
}

func TestFurtherHolidayRewardAndBudgetSafeBoundaries(t *testing.T) {
	c := FurtherDefaultHolidayCampaigns()[2]
	c.RewardQuota = common.MaxWalletQuota
	c.DailyBudgetQuota = int64(common.MaxWalletQuota)
	require.NoError(t, ValidateFurtherHolidayCampaign(c))
	c.DailyBudgetQuota++
	require.Error(t, ValidateFurtherHolidayCampaign(c))
	c.DailyBudgetQuota = 0
	c.RewardQuota++
	require.Error(t, ValidateFurtherHolidayCampaign(c))
}

func TestFurtherDailyCheckinAtomicShanghaiReward(t *testing.T) {
	activityTestDB(t)
	previous := *operation_setting.GetCheckinSetting()
	t.Cleanup(func() { *operation_setting.GetCheckinSetting() = previous })
	*operation_setting.GetCheckinSetting() = operation_setting.CheckinSetting{Enabled: true, MinQuota: 100, MaxQuota: 100}
	user := activityTestUser(t, "daily-checkin", 0)
	now := activityTime("2026-09-24T16:00:00Z")
	record, err := userCheckinAt(user.Id, now)
	require.NoError(t, err)
	require.Equal(t, "2026-09-25", record.CheckinDate)
	_, err = userCheckinAt(user.Id, now.Add(time.Hour))
	require.Error(t, err)
	_, err = userCheckinAt(user.Id, now.Add(24*time.Hour))
	require.NoError(t, err)
	require.NoError(t, DB.First(&user, user.Id).Error)
	require.Equal(t, 200, user.Quota)
	overflow := activityTestUser(t, "overflow-checkin", common.MaxWalletQuota-50)
	_, err = userCheckinAt(overflow.Id, now)
	require.Error(t, err)
	var count int64
	require.NoError(t, DB.Model(&Checkin{}).Where("user_id = ?", overflow.Id).Count(&count).Error)
	require.Zero(t, count)
	require.Error(t, SaveFurtherActivityCheckinSettings(operation_setting.CheckinSetting{Enabled: true, MinQuota: 200, MaxQuota: 100}))
}

func TestFurtherActivityCheckinSettingsPersistAndRollback(t *testing.T) {
	activityTestDB(t)
	previous := FurtherCheckinSettingsSnapshot()
	t.Cleanup(func() { *operation_setting.GetCheckinSetting() = previous })
	settings := operation_setting.CheckinSetting{Enabled: true, MinQuota: 150, MaxQuota: 300}
	require.NoError(t, SaveFurtherActivityCheckinSettings(settings))
	require.Equal(t, settings, FurtherCheckinSettingsSnapshot())
	var saved Option
	require.NoError(t, DB.First(&saved, "key = ?", "checkin_setting.min_quota").Error)
	require.Equal(t, "150", saved.Value)
	// The test database is SQLite: reject an option update to verify the shared
	// bulk transaction leaves both persisted and live settings untouched.
	require.NoError(t, DB.Exec("CREATE TRIGGER activity_checkin_fail BEFORE UPDATE ON options WHEN NEW.key = 'checkin_setting.max_quota' BEGIN SELECT RAISE(ABORT, 'test settings failure'); END").Error)
	require.Error(t, SaveFurtherActivityCheckinSettings(operation_setting.CheckinSetting{Enabled: false, MinQuota: 200, MaxQuota: 400}))
	require.Equal(t, settings, FurtherCheckinSettingsSnapshot())
	require.NoError(t, DB.First(&saved, "key = ?", "checkin_setting.min_quota").Error)
	require.Equal(t, "150", saved.Value)
}
