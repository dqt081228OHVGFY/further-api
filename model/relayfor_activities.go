package model

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var RelayForActivityLocation = time.FixedZone("Asia/Shanghai", 8*60*60)
var relayForCheckinSettingMutex sync.RWMutex

func RelayForActivityPageAlias(page string) string {
	switch page {
	case "benefits", "checkin", "activity":
		return "activities"
	}
	return page
}

func RelayForCheckinSettingsSnapshot() operation_setting.CheckinSetting {
	relayForCheckinSettingMutex.RLock()
	defer relayForCheckinSettingMutex.RUnlock()
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return *operation_setting.GetCheckinSetting()
}

type RelayForHolidayCampaign struct {
	Key                 string `json:"key" gorm:"primaryKey;size:32"`
	Title               string `json:"title" gorm:"size:240"`
	Description         string `json:"description" gorm:"size:1600"`
	Details             string `json:"details" gorm:"type:text"`
	Rules               string `json:"rules" gorm:"type:text"`
	Enabled             bool   `json:"enabled"`
	SubscriptionEnabled bool   `json:"subscription_enabled"`
	SubscriptionPlanIDs []int  `json:"subscription_plan_ids" gorm:"serializer:json;type:text"`
	DaysBefore          int    `json:"days_before"`
	DaysAfter           int    `json:"days_after"`
	RewardQuota         int    `json:"reward_quota"`
	DailyClaimLimit     int    `json:"daily_claim_limit"`
	DailyBudgetQuota    int64  `json:"daily_budget_quota"`
	TargetGroup         string `json:"target_group" gorm:"size:64"`
	UpdatedBy           int    `json:"-"`
	UpdatedAt           int64  `json:"updated_at"`
}

type RelayForHolidayClaim struct {
	ID          int    `json:"id" gorm:"primaryKey"`
	CampaignKey string `json:"campaign_key" gorm:"size:32;uniqueIndex:idx_holiday_claim,priority:1;index"`
	UserID      int    `json:"-" gorm:"uniqueIndex:idx_holiday_claim,priority:2;index"`
	ClaimDate   string `json:"claim_date" gorm:"size:10;uniqueIndex:idx_holiday_claim,priority:3;index"`
	RewardQuota int    `json:"reward_quota"`
	CreatedAt   int64  `json:"created_at"`
}

// A holiday subscription is a free, persistent calendar preference. It is
// deliberately separate from paid subscriptions and never changes user quota.
type RelayForHolidaySubscription struct {
	ID          int    `json:"-" gorm:"primaryKey"`
	CampaignKey string `json:"key" gorm:"size:32;not null;uniqueIndex:idx_holiday_subscription,priority:1"`
	UserID      int    `json:"-" gorm:"not null;uniqueIndex:idx_holiday_subscription,priority:2;index"`
	CreatedAt   int64  `json:"created_at"`
}

func RelayForDefaultHolidayCampaigns() []RelayForHolidayCampaign {
	campaigns := []RelayForHolidayCampaign{
		{Key: "duanwu", Title: "端午 · 粽享好时光", Description: "五月初五，以一份心意为新的创造蓄力。", Enabled: true},
		{Key: "qixi", Title: "七夕 · 与灵感相遇", Description: "七月初七，让每一个心动的想法有回应。", Enabled: true},
		{Key: "mid_autumn", Title: "中秋 · 月满好礼", Description: "八月十五，月饼与金色月光陪你团圆。", Enabled: true},
		{Key: "chongyang", Title: "重阳 · 登高望远", Description: "九月初九，留一份从容，看更远的风景。", Enabled: true},
		{Key: "new_year_eve", Title: "除夕 · 辞旧迎新", Description: "农历岁末的最后一天，收藏这一年的收获。", Enabled: true},
		{Key: "national_day", Title: "国庆 · 山河同庆", Description: "十月一日到七日，携一份热爱，奔赴更辽阔的天地。", Enabled: true, DaysAfter: 6},
	}
	for index := range campaigns {
		campaigns[index].SubscriptionEnabled = true
		campaigns[index].SubscriptionPlanIDs = []int{}
	}
	return campaigns
}

func ValidateRelayForHolidayCampaign(c RelayForHolidayCampaign) error {
	valid := false
	for _, item := range RelayForDefaultHolidayCampaigns() {
		if item.Key == c.Key {
			valid = true
		}
	}
	if !valid || strings.TrimSpace(c.Title) == "" || len([]rune(c.Title)) > 60 || len([]rune(c.Description)) > 400 || len(c.TargetGroup) > 64 || c.DaysBefore < 0 || c.DaysBefore > 7 || c.DaysAfter < 0 || c.DaysAfter > 7 || c.RewardQuota < 0 || c.RewardQuota > common.MaxWalletQuota || c.DailyClaimLimit < 0 || c.DailyClaimLimit > 100000 || c.DailyBudgetQuota < 0 || c.DailyBudgetQuota > int64(common.MaxWalletQuota) {
		return errors.New("活动参数无效：窗口最多前后各7天，奖励、名额和预算必须在允许范围内")
	}
	if len([]rune(c.Details)) > 4000 || len([]rune(c.Rules)) > 2000 {
		return errors.New("活动详情最多4000字，活动规则最多2000字")
	}
	if len(c.SubscriptionPlanIDs) > 12 {
		return errors.New("每个节日最多关联12个订阅套餐")
	}
	seen := map[int]bool{}
	for _, id := range c.SubscriptionPlanIDs {
		if id <= 0 || seen[id] {
			return errors.New("订阅套餐必须是有效且不重复的套餐编号")
		}
		seen[id] = true
	}
	return nil
}

func SaveRelayForHolidayCampaign(c RelayForHolidayCampaign) error {
	c.Title, c.Description, c.TargetGroup = strings.TrimSpace(c.Title), strings.TrimSpace(c.Description), strings.TrimSpace(c.TargetGroup)
	c.Details, c.Rules = strings.TrimSpace(c.Details), strings.TrimSpace(c.Rules)
	if err := ValidateRelayForHolidayCampaign(c); err != nil {
		return err
	}
	if c.SubscriptionPlanIDs == nil {
		c.SubscriptionPlanIDs = []int{}
	}
	if len(c.SubscriptionPlanIDs) > 0 {
		var count int64
		if err := DB.Model(&SubscriptionPlan{}).Where("id IN ?", c.SubscriptionPlanIDs).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(c.SubscriptionPlanIDs)) {
			return errors.New("关联的订阅套餐不存在")
		}
	}
	c.UpdatedAt = time.Now().Unix()
	return DB.Save(&c).Error
}

func ListRelayForHolidayCampaigns() ([]RelayForHolidayCampaign, error) {
	var saved []RelayForHolidayCampaign
	if err := DB.Find(&saved).Error; err != nil {
		return nil, err
	}
	lookup := map[string]RelayForHolidayCampaign{}
	for _, c := range saved {
		lookup[c.Key] = c
	}
	result := RelayForDefaultHolidayCampaigns()
	for index, item := range result {
		if c, ok := lookup[item.Key]; ok {
			if c.SubscriptionPlanIDs == nil {
				c.SubscriptionPlanIDs = []int{}
			}
			result[index] = c
		}
	}
	return result, nil
}

func RelayForHolidayWindow(c RelayForHolidayCampaign, now time.Time) (start, end time.Time, active bool) {
	local := now.In(RelayForActivityLocation)
	for _, year := range []int{local.Year() - 1, local.Year(), local.Year() + 1} {
		date, ok := RelayForHolidayDate(c.Key, year)
		if !ok {
			continue
		}
		festival, err := time.ParseInLocation("2006-01-02", date, RelayForActivityLocation)
		if err != nil {
			continue
		}
		from, to := festival.AddDate(0, 0, -c.DaysBefore), festival.AddDate(0, 0, c.DaysAfter+1)
		if !local.Before(from) && local.Before(to) {
			return from, to, c.Enabled
		}
	}
	return time.Time{}, time.Time{}, false
}

// RelayForHolidayNextWindow returns the current or next verified occurrence,
// including its full claim window, even while the campaign is only a preview.
func RelayForHolidayNextWindow(c RelayForHolidayCampaign, now time.Time) (date string, start, end time.Time) {
	local := now.In(RelayForActivityLocation)
	for _, year := range []int{local.Year() - 1, local.Year(), local.Year() + 1} {
		date, ok := RelayForHolidayDate(c.Key, year)
		if !ok {
			continue
		}
		festival, err := time.ParseInLocation(time.DateOnly, date, RelayForActivityLocation)
		if err != nil {
			continue
		}
		from, to := festival.AddDate(0, 0, -c.DaysBefore), festival.AddDate(0, 0, c.DaysAfter+1)
		if local.Before(to) {
			return date, from, to
		}
	}
	return "", time.Time{}, time.Time{}
}

func SetRelayForHolidaySubscription(userID int, key string, subscribed bool, now time.Time) error {
	if userID <= 0 {
		return errors.New("请先登录")
	}
	var campaign *RelayForHolidayCampaign
	for _, item := range RelayForDefaultHolidayCampaigns() {
		if item.Key == key {
			candidate := item
			campaign = &candidate
			break
		}
	}
	if campaign == nil {
		return errors.New("活动不存在")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if subscribed {
			var saved RelayForHolidayCampaign
			err := lockForUpdate(tx).Where(&RelayForHolidayCampaign{Key: key}).First(&saved).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil {
				campaign = &saved
			}
			if !campaign.Enabled || !campaign.SubscriptionEnabled {
				return errors.New("本活动暂未开放订阅")
			}
		}
		var user User
		if err := lockForUpdate(tx).Select("id", "status").First(&user, userID).Error; err != nil {
			return err
		}
		if user.Status != common.UserStatusEnabled {
			return errors.New("账户不可用")
		}
		if !subscribed {
			// Withdrawal remains available after a campaign is disabled.
			return tx.Where("campaign_key = ? AND user_id = ?", key, userID).Delete(&RelayForHolidaySubscription{}).Error
		}
		record := RelayForHolidaySubscription{CampaignKey: key, UserID: userID, CreatedAt: now.Unix()}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record).Error
	})
}

func RelayForActivitiesView(userID int, now time.Time, admin bool) (map[string]any, error) {
	campaigns, err := ListRelayForHolidayCampaigns()
	if err != nil {
		return nil, err
	}
	today := now.In(RelayForActivityLocation).Format("2006-01-02")
	var user User
	subscribedKeys := map[string]bool{}
	if userID > 0 {
		if err := DB.First(&user, userID).Error; err != nil {
			return nil, err
		}
		var subscriptions []RelayForHolidaySubscription
		if err := DB.Where("user_id = ?", userID).Find(&subscriptions).Error; err != nil {
			return nil, err
		}
		for _, subscription := range subscriptions {
			subscribedKeys[subscription.CampaignKey] = true
		}
	}
	views := []map[string]any{}
	theme := ""
	for _, c := range campaigns {
		_, _, active := RelayForHolidayWindow(c, now)
		if !admin && !c.Enabled {
			continue
		}
		date, _ := RelayForHolidayDate(c.Key, now.In(RelayForActivityLocation).Year())
		nextDate, start, end := RelayForHolidayNextWindow(c, now)
		status := "upcoming"
		if active {
			status = "active"
		} else if festival, err := time.ParseInLocation(time.DateOnly, date, RelayForActivityLocation); err == nil && !now.Before(festival.AddDate(0, 0, c.DaysAfter+1)) {
			status = "ended"
		}
		claimed := false
		if userID > 0 {
			var count int64
			if err := DB.Model(&RelayForHolidayClaim{}).Where("campaign_key = ? AND user_id = ? AND claim_date = ?", c.Key, userID, today).Count(&count).Error; err != nil {
				return nil, err
			}
			claimed = count > 0
		}
		var usage struct {
			Count int64
			Quota int64
		}
		if err := DB.Model(&RelayForHolidayClaim{}).Where("campaign_key = ? AND claim_date = ?", c.Key, today).Select("COUNT(*) AS count, COALESCE(SUM(reward_quota), 0) AS quota").Scan(&usage).Error; err != nil {
			return nil, err
		}
		if usage.Quota < 0 || usage.Count < 0 {
			return nil, errors.New("活动领取统计无效，请联系管理员")
		}
		eligible := userID > 0 && user.Status == common.UserStatusEnabled && (c.TargetGroup == "" || user.Group == c.TargetGroup)
		budgetOK := c.DailyBudgetQuota == 0 || usage.Quota <= c.DailyBudgetQuota-int64(c.RewardQuota)
		limitOK := c.DailyClaimLimit == 0 || usage.Count < int64(c.DailyClaimLimit)
		v := map[string]any{"key": c.Key, "title": c.Title, "description": c.Description, "details": c.Details, "rules": c.Rules, "date": date, "next_date": nextDate, "status": status, "active": active, "reward_quota": c.RewardQuota, "claimed_today": claimed, "can_claim": active && eligible && !claimed && c.RewardQuota > 0 && budgetOK && limitOK, "target_group": c.TargetGroup, "subscription_enabled": c.SubscriptionEnabled, "subscribed": subscribedKeys[c.Key], "can_subscribe": userID > 0 && user.Status == common.UserStatusEnabled && c.Enabled && c.SubscriptionEnabled, "days_before": c.DaysBefore, "days_after": c.DaysAfter}
		v["subscription_plan_ids"] = c.SubscriptionPlanIDs
		if !start.IsZero() {
			v["starts_at"] = start.Format(time.RFC3339)
			v["ends_at"] = end.Format(time.RFC3339)
		}
		if active && theme == "" {
			theme = c.Key
		}
		if admin {
			v["enabled"] = c.Enabled
			v["days_before"] = c.DaysBefore
			v["days_after"] = c.DaysAfter
			v["daily_claim_limit"] = c.DailyClaimLimit
			v["daily_budget_quota"] = c.DailyBudgetQuota
			v["today_claims"] = usage.Count
			v["today_awarded_quota"] = usage.Quota
			var subscriptionCount int64
			if err := DB.Model(&RelayForHolidaySubscription{}).Where("campaign_key = ?", c.Key).Count(&subscriptionCount).Error; err != nil {
				return nil, err
			}
			v["subscription_count"] = subscriptionCount
		}
		views = append(views, v)
	}
	checkin := RelayForCheckinSettingsSnapshot()
	result := map[string]any{"server_date": today, "server_time": now.In(RelayForActivityLocation).Format(time.RFC3339), "timezone": "Asia/Shanghai", "calendar_min_year": RelayForHolidayMinYear, "calendar_max_year": RelayForHolidayMaxYear, "theme": theme, "campaigns": views, "checkin": map[string]any{"enabled": checkin.Enabled, "min_quota": checkin.MinQuota, "max_quota": checkin.MaxQuota, "turnstile_required": common.TurnstileCheckEnabled, "turnstile_site_key": common.TurnstileSiteKey}}
	if userID > 0 && !admin {
		var history []RelayForHolidayClaim
		if err := DB.Where("user_id = ?", userID).Order("id DESC").Limit(30).Find(&history).Error; err != nil {
			return nil, err
		}
		result["history"] = history
	}
	return result, nil
}

func ClaimRelayForHoliday(userID int, key string, now time.Time) (*RelayForHolidayClaim, error) {
	if userID <= 0 {
		return nil, errors.New("请先登录")
	}
	validKey := false
	for _, campaign := range RelayForDefaultHolidayCampaigns() {
		if campaign.Key == key {
			validKey = true
		}
	}
	if !validKey {
		return nil, errors.New("活动不存在")
	}
	day := now.In(RelayForActivityLocation).Format("2006-01-02")
	var record RelayForHolidayClaim
	err := DB.Transaction(func(tx *gorm.DB) error {
		var campaign RelayForHolidayCampaign
		if err := lockForUpdate(tx).Where(&RelayForHolidayCampaign{Key: key}).First(&campaign).Error; err != nil {
			return errors.New("活动奖励尚未配置")
		}
		if err := ValidateRelayForHolidayCampaign(campaign); err != nil {
			return err
		}
		_, _, active := RelayForHolidayWindow(campaign, now)
		if !active {
			return errors.New("当前不在活动开放时间内")
		}
		if campaign.RewardQuota <= 0 {
			return errors.New("本活动没有可领取的额度奖励")
		}
		var user User
		if err := lockForUpdate(tx).First(&user, userID).Error; err != nil {
			return err
		}
		if user.Status != common.UserStatusEnabled || (campaign.TargetGroup != "" && user.Group != campaign.TargetGroup) {
			return errors.New("当前账户不符合活动领取条件")
		}
		var duplicate int64
		if err := tx.Model(&RelayForHolidayClaim{}).Where("campaign_key = ? AND user_id = ? AND claim_date = ?", key, userID, day).Count(&duplicate).Error; err != nil {
			return err
		}
		if duplicate > 0 {
			return errors.New("今日已经领取本活动奖励")
		}
		var usage struct {
			Count int64
			Quota int64
		}
		if err := tx.Model(&RelayForHolidayClaim{}).Where("campaign_key = ? AND claim_date = ?", key, day).Select("COUNT(*) AS count, COALESCE(SUM(reward_quota), 0) AS quota").Scan(&usage).Error; err != nil {
			return err
		}
		if usage.Quota < 0 || usage.Count < 0 {
			return errors.New("活动领取统计无效，请联系管理员")
		}
		if campaign.DailyClaimLimit > 0 && usage.Count >= int64(campaign.DailyClaimLimit) {
			return errors.New("今日活动名额已经用完")
		}
		if campaign.DailyBudgetQuota > 0 && usage.Quota > campaign.DailyBudgetQuota-int64(campaign.RewardQuota) {
			return errors.New("今日活动预算已经用完")
		}
		record = RelayForHolidayClaim{CampaignKey: key, UserID: userID, ClaimDate: day, RewardQuota: campaign.RewardQuota, CreatedAt: now.Unix()}
		if err := tx.Create(&record).Error; err != nil {
			return errors.New("领取未完成，请刷新确认后重试")
		}
		return creditTopUpQuota(tx, userID, campaign.RewardQuota, nil)
	})
	if err != nil {
		return nil, err
	}
	syncCreditUserQuotaCache(userID, record.RewardQuota, "holiday activity")
	return &record, nil
}

func SaveRelayForActivityCheckinSettings(settings operation_setting.CheckinSetting) error {
	if settings.MinQuota < 0 || settings.MaxQuota < settings.MinQuota || settings.MaxQuota > common.MaxWalletQuota {
		return errors.New("签到最小与最大额度无效")
	}
	relayForCheckinSettingMutex.Lock()
	defer relayForCheckinSettingMutex.Unlock()
	return UpdateOptionsBulk(map[string]string{"checkin_setting.enabled": strconv.FormatBool(settings.Enabled), "checkin_setting.min_quota": strconv.Itoa(settings.MinQuota), "checkin_setting.max_quota": strconv.Itoa(settings.MaxQuota)})
}
