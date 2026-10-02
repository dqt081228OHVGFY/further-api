package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RelayForSubscriptionModelRule struct {
	ModelID     int     `json:"model_id"`
	Model       string  `json:"model"`
	DisplayName string  `json:"display_name"`
	NameRule    int     `json:"name_rule,omitempty"`
	Multiplier  float64 `json:"multiplier"`
}

type RelayForSubscriptionPolicy struct {
	PlanID               int    `gorm:"primaryKey"`
	Code                 string `gorm:"size:64;uniqueIndex"`
	Collapsed            bool
	Internal             bool `gorm:"index"`
	DiscountMultiplier   float64
	DiscountSavingsLimit int64
	RequestsPerMinute    int
	ConcurrentRequests   int
	UpgradeFromPlanID    int    `gorm:"index"`
	ModelRulesJSON       string `gorm:"type:text"`
}

type RelayForUserSubscriptionPolicy struct {
	SubscriptionID          int `gorm:"primaryKey"`
	PlanID                  int `gorm:"index"`
	DiscountMultiplier      float64
	DiscountSavingsLimit    int64
	DiscountSavingsUsed     int64
	DiscountSavingsReserved int64
	RequestsPerMinute       int
	ConcurrentRequests      int
	UpgradeFromPlanID       int
	ModelRulesJSON          string `gorm:"type:text"`
}

func (p RelayForUserSubscriptionPolicy) Rules() []RelayForSubscriptionModelRule {
	rules := []RelayForSubscriptionModelRule{}
	_ = json.Unmarshal([]byte(p.ModelRulesJSON), &rules)
	return rules
}

func (p RelayForUserSubscriptionPolicy) ModelMultiplier(modelName string) float64 {
	multiplier := p.DiscountMultiplier
	if multiplier <= 0 || multiplier > 1 {
		multiplier = 1
	}
	for _, rule := range p.Rules() {
		matched := rule.Model == modelName
		switch rule.NameRule {
		case NameRulePrefix:
			matched = strings.HasPrefix(modelName, rule.Model)
		case NameRuleSuffix:
			matched = strings.HasSuffix(modelName, rule.Model)
		case NameRuleContains:
			matched = strings.Contains(modelName, rule.Model)
		}
		if matched && rule.Multiplier > 0 && rule.Multiplier <= 1 {
			return rule.Multiplier
		}
	}
	return multiplier
}

type RelayForSubscriptionPreference struct {
	UserID     int `gorm:"primaryKey"`
	TimedFirst bool
}
type RelayForSubscriptionHidden struct {
	SubscriptionID int `gorm:"primaryKey"`
	UserID         int `gorm:"index"`
}
type RelayForEntitlement struct {
	Subscription UserSubscription
	Policy       RelayForUserSubscriptionPolicy
}

var relayForTimedSources = []string{"relayfor_benefit_timed", "relayfor_converted", "relayfor_prepaid"}

func relayForPolicySnapshot(policy RelayForSubscriptionPolicy, subscriptionID int) RelayForUserSubscriptionPolicy {
	return RelayForUserSubscriptionPolicy{SubscriptionID: subscriptionID, PlanID: policy.PlanID, DiscountMultiplier: policy.DiscountMultiplier, DiscountSavingsLimit: policy.DiscountSavingsLimit, RequestsPerMinute: policy.RequestsPerMinute, ConcurrentRequests: policy.ConcurrentRequests, UpgradeFromPlanID: policy.UpgradeFromPlanID, ModelRulesJSON: policy.ModelRulesJSON}
}

func AttachRelayForSubscriptionPolicyTx(tx *gorm.DB, sub *UserSubscription, plan *SubscriptionPlan) error {
	if !tx.Migrator().HasTable(&RelayForSubscriptionPolicy{}) {
		return nil
	}
	var policy RelayForSubscriptionPolicy
	err := tx.First(&policy, "plan_id = ?", plan.Id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if policy.UpgradeFromPlanID > 0 {
		valid, err := relayForActiveUpgradeChainTx(tx, sub.UserId, policy.UpgradeFromPlanID, map[int]bool{sub.PlanId: true})
		if err != nil {
			return err
		}
		if !valid {
			return errors.New("请先订阅有效的前置套餐")
		}
	}
	snapshot := relayForPolicySnapshot(policy, sub.Id)
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&snapshot).Error
}

func GetRelayForActiveEntitlements(userID int, modelName string) ([]RelayForEntitlement, error) {
	var subs []UserSubscription
	now := common.GetTimestamp()
	if err := DB.Where("user_id = ? AND status = ? AND start_time <= ? AND end_time > ?", userID, "active", now, now).Order("end_time ASC, id ASC").Find(&subs).Error; err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(subs))
	plans := make([]int, 0, len(subs))
	activePlans := map[int]bool{}
	for _, sub := range subs {
		ids = append(ids, sub.Id)
		plans = append(plans, sub.PlanId)
		activePlans[sub.PlanId] = true
	}
	var snapshots []RelayForUserSubscriptionPolicy
	var policies []RelayForSubscriptionPolicy
	if len(ids) > 0 {
		if err := DB.Where("subscription_id IN ?", ids).Find(&snapshots).Error; err != nil {
			return nil, err
		}
		if err := DB.Where("plan_id IN ?", plans).Find(&policies).Error; err != nil {
			return nil, err
		}
	}
	bySub := map[int]RelayForUserSubscriptionPolicy{}
	byPlan := map[int]RelayForSubscriptionPolicy{}
	for _, item := range snapshots {
		bySub[item.SubscriptionID] = item
	}
	for _, item := range policies {
		byPlan[item.PlanID] = item
	}
	result := []RelayForEntitlement{}
	for _, sub := range subs {
		policy, ok := bySub[sub.Id]
		if !ok {
			if template, exists := byPlan[sub.PlanId]; exists {
				policy = relayForPolicySnapshot(template, sub.Id)
			} else {
				policy = RelayForUserSubscriptionPolicy{SubscriptionID: sub.Id, PlanID: sub.PlanId, DiscountMultiplier: 1}
			}
		}
		if policy.UpgradeFromPlanID > 0 && !activePlans[policy.UpgradeFromPlanID] {
			continue
		}
		result = append(result, RelayForEntitlement{Subscription: sub, Policy: policy})
	}
	// Remove transitively invalid upgrade chains as well as direct orphans.
	for changed := true; changed; {
		changed = false
		validPlans := map[int]bool{}
		for _, item := range result {
			validPlans[item.Subscription.PlanId] = true
		}
		kept := result[:0]
		for _, item := range result {
			if item.Policy.UpgradeFromPlanID > 0 && !validPlans[item.Policy.UpgradeFromPlanID] {
				changed = true
				continue
			}
			kept = append(kept, item)
		}
		result = kept
	}
	return result, nil
}

type RelayForPlanInput struct {
	Code                    string                          `json:"code"`
	Name                    string                          `json:"name"`
	Description             string                          `json:"description"`
	DurationDays            int                             `json:"duration_days"`
	ResetIntervalDays       int                             `json:"reset_interval_days"`
	Quota                   int64                           `json:"quota"`
	QuotaGroup              string                          `json:"quota_group"`
	UnlimitedQuota          bool                            `json:"unlimited_quota"`
	DiscountMultiplier      float64                         `json:"discount_multiplier"`
	DiscountSavingsLimit    int64                           `json:"discount_savings_limit"`
	DisplayOrder            int                             `json:"display_order"`
	UpgradeFromPlanID       int                             `json:"upgrade_from_plan_id"`
	RequestsPerMinute       int                             `json:"requests_per_minute"`
	ConcurrentRequests      int                             `json:"concurrent_requests"`
	Collapsed               bool                            `json:"collapsed"`
	ModelRules              []RelayForSubscriptionModelRule `json:"model_rules"`
	RefreshExistingBenefits bool                            `json:"refresh_existing_benefits"`
	Status                  int                             `json:"status"`
}

func SaveRelayForSubscriptionPlan(id int, input RelayForPlanInput) (int, error) {
	input.Code, input.Name, input.Description, input.QuotaGroup = strings.TrimSpace(input.Code), strings.TrimSpace(input.Name), strings.TrimSpace(input.Description), strings.TrimSpace(input.QuotaGroup)
	if input.DiscountMultiplier == 0 {
		input.DiscountMultiplier = 1
	}
	if input.Code == "" || len(input.Code) > 64 || input.Name == "" || len([]rune(input.Name)) > 128 || len([]rune(input.Description)) > 255 || input.DurationDays < 1 || input.DurationDays > 3650 || input.ResetIntervalDays < 0 || input.ResetIntervalDays > 3650 || input.Quota < 0 || input.Quota > int64(common.MaxWalletQuota) || (!input.UnlimitedQuota && input.Quota == 0) || input.DiscountSavingsLimit < 0 || input.DiscountSavingsLimit > int64(common.MaxWalletQuota) || input.DiscountMultiplier <= 0 || input.DiscountMultiplier > 1 || math.IsNaN(input.DiscountMultiplier) || input.RequestsPerMinute < 0 || input.RequestsPerMinute > 1000000 || input.ConcurrentRequests < 0 || input.ConcurrentRequests > 100000 || len(input.ModelRules) > 1000 || (input.Status != 1 && input.Status != 2) || input.UpgradeFromPlanID < 0 {
		return 0, errors.New("套餐参数无效")
	}
	var savedID int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var plan SubscriptionPlan
		if id > 0 {
			if err := lockForUpdate(tx).First(&plan, id).Error; err != nil {
				return err
			}
		}
		if input.UpgradeFromPlanID > 0 {
			seen := map[int]bool{id: true}
			next := input.UpgradeFromPlanID
			for depth := 0; next > 0; depth++ {
				if seen[next] || depth >= 32 {
					return errors.New("前置套餐不能形成循环，最多32层")
				}
				seen[next] = true
				var parent SubscriptionPlan
				if err := lockForUpdate(tx).First(&parent, next).Error; err != nil {
					return errors.New("前置套餐不存在")
				}
				var p RelayForSubscriptionPolicy
				err := tx.First(&p, "plan_id = ?", next).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					break
				}
				if err != nil {
					return err
				}
				next = p.UpgradeFromPlanID
			}
		}
		seenModels := map[int]bool{}
		for index := range input.ModelRules {
			rule := &input.ModelRules[index]
			if rule.ModelID <= 0 || seenModels[rule.ModelID] || rule.Multiplier < 0 || rule.Multiplier > 1 || math.IsNaN(rule.Multiplier) {
				return errors.New("专属模型配置无效")
			}
			seenModels[rule.ModelID] = true
			var record Model
			if err := tx.First(&record, rule.ModelID).Error; err != nil {
				return errors.New("专属模型不存在")
			}
			rule.Model, rule.DisplayName, rule.NameRule = record.ModelName, record.ModelName, record.NameRule
		}
		rules, err := json.Marshal(input.ModelRules)
		if err != nil {
			return err
		}
		plan.Title, plan.Subtitle, plan.DurationUnit, plan.DurationValue = input.Name, input.Description, SubscriptionDurationDay, input.DurationDays
		plan.TotalAmount = input.Quota
		if input.UnlimitedQuota {
			plan.TotalAmount = 0
		}
		plan.SortOrder = -input.DisplayOrder
		plan.UpgradeGroup = input.QuotaGroup
		plan.Enabled = input.Status == 1
		plan.QuotaResetPeriod = SubscriptionResetNever
		plan.QuotaResetCustomSeconds = 0
		if input.ResetIntervalDays > 0 {
			plan.QuotaResetPeriod = SubscriptionResetCustom
			plan.QuotaResetCustomSeconds = int64(input.ResetIntervalDays) * 86400
		}
		if id == 0 {
			if err := tx.Create(&plan).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Save(&plan).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&plan).Update("enabled", input.Status == 1).Error; err != nil {
			return err
		}
		policy := RelayForSubscriptionPolicy{PlanID: plan.Id, Code: input.Code, Collapsed: input.Collapsed, DiscountMultiplier: input.DiscountMultiplier, DiscountSavingsLimit: input.DiscountSavingsLimit, RequestsPerMinute: input.RequestsPerMinute, ConcurrentRequests: input.ConcurrentRequests, UpgradeFromPlanID: input.UpgradeFromPlanID, ModelRulesJSON: string(rules)}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "plan_id"}}, UpdateAll: true}).Create(&policy).Error; err != nil {
			return err
		}
		if input.RefreshExistingBenefits {
			var subs []UserSubscription
			if err := lockForUpdate(tx).Where("plan_id = ? AND status = ? AND end_time > ?", plan.Id, "active", common.GetTimestamp()).Find(&subs).Error; err != nil {
				return err
			}
			for _, sub := range subs {
				snapshot := relayForPolicySnapshot(policy, sub.Id)
				if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "subscription_id"}}, DoUpdates: clause.AssignmentColumns([]string{"discount_multiplier", "discount_savings_limit", "requests_per_minute", "concurrent_requests", "upgrade_from_plan_id", "model_rules_json"})}).Create(&snapshot).Error; err != nil {
					return err
				}
				if err := tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Update("amount_total", plan.TotalAmount).Error; err != nil {
					return err
				}
			}
		}
		savedID = plan.Id
		return nil
	})
	if err == nil {
		InvalidateSubscriptionPlanCache(savedID)
	}
	return savedID, err
}

func relayForPlanView(plan SubscriptionPlan, policy RelayForSubscriptionPolicy) map[string]any {
	code := policy.Code
	if code == "" {
		code = fmt.Sprintf("newapi-%d", plan.Id)
	}
	snapshot := relayForPolicySnapshot(policy, 0)
	multiplier := snapshot.ModelMultiplier("")
	days := plan.DurationValue
	switch plan.DurationUnit {
	case SubscriptionDurationMonth:
		days *= 30
	case SubscriptionDurationYear:
		days *= 365
	case SubscriptionDurationCustom:
		days = int(plan.CustomSeconds / 86400)
	case SubscriptionDurationHour:
		days = (days + 23) / 24
	}
	reset := 0
	switch NormalizeResetPeriod(plan.QuotaResetPeriod) {
	case SubscriptionResetDaily:
		reset = 1
	case SubscriptionResetWeekly:
		reset = 7
	case SubscriptionResetMonthly:
		reset = 30
	case SubscriptionResetCustom:
		reset = int(plan.QuotaResetCustomSeconds / 86400)
	}
	status := 2
	if plan.Enabled {
		status = 1
	}
	return map[string]any{"id": plan.Id, "code": code, "name": plan.Title, "description": plan.Subtitle, "duration_days": days, "duration_unit": plan.DurationUnit, "duration_value": plan.DurationValue, "reset_interval_days": reset, "quota": plan.TotalAmount, "quota_group": plan.UpgradeGroup, "unlimited_quota": plan.TotalAmount <= 0, "discount_multiplier": multiplier, "discount_savings_limit": policy.DiscountSavingsLimit, "display_order": -plan.SortOrder, "requests_per_minute": policy.RequestsPerMinute, "concurrent_requests": policy.ConcurrentRequests, "upgrade_from_plan_id": policy.UpgradeFromPlanID, "collapsed": policy.Collapsed, "model_rules": snapshot.Rules(), "status": status, "price_amount": plan.PriceAmount, "currency": plan.Currency}
}

func ListRelayForSubscriptionPlans(userID int, admin bool) ([]map[string]any, error) {
	var plans []SubscriptionPlan
	query := DB.Where("id NOT IN (?)", DB.Model(&RelayForSubscriptionPolicy{}).Select("plan_id").Where("internal = ?", true))
	if DB.Migrator().HasTable(&RelayForPrepaidCard{}) {
		query = query.Where("id NOT IN (?)", DB.Model(&RelayForPrepaidCard{}).Unscoped().Select("plan_id"))
	}
	if !admin {
		query = query.Where("enabled = ?", true)
	}
	if err := query.Order("sort_order DESC, id ASC").Find(&plans).Error; err != nil {
		return nil, err
	}
	var policies []RelayForSubscriptionPolicy
	if err := DB.Find(&policies).Error; err != nil {
		return nil, err
	}
	byID := map[int]RelayForSubscriptionPolicy{}
	for _, policy := range policies {
		byID[policy.PlanID] = policy
	}
	active := map[int]bool{}
	if !admin {
		entitlements, err := GetRelayForActiveEntitlements(userID, "")
		if err != nil {
			return nil, err
		}
		for _, item := range entitlements {
			active[item.Subscription.PlanId] = true
		}
	}
	result := []map[string]any{}
	for _, plan := range plans {
		policy := byID[plan.Id]
		view := relayForPlanView(plan, policy)
		view["active"] = active[plan.Id]
		view["upgrade_eligible"] = policy.UpgradeFromPlanID == 0 || active[policy.UpgradeFromPlanID]
		if policy.UpgradeFromPlanID > 0 {
			var base SubscriptionPlan
			if err := DB.First(&base, policy.UpgradeFromPlanID).Error; err == nil {
				view["upgrade_from_plan_name"] = base.Title
				view["upgrade_from_plan_code"] = relayForPlanView(base, byID[base.Id])["code"]
			}
		}
		result = append(result, view)
	}
	return result, nil
}

func relayForSubscriptionView(sub UserSubscription, plan SubscriptionPlan, policy RelayForUserSubscriptionPolicy, template RelayForSubscriptionPolicy) map[string]any {
	view := relayForPlanView(plan, template)
	view["id"], view["plan_id"], view["plan_name"], view["plan_code"] = sub.Id, plan.Id, plan.Title, view["code"]
	view["quota_total"], view["quota_remaining"], view["unlimited_quota"] = sub.AmountTotal, max(int64(0), sub.AmountTotal-sub.AmountUsed), sub.AmountTotal <= 0
	view["starts_at"], view["expires_at"], view["next_reset_at"] = prepaidTime(sub.StartTime), prepaidTime(sub.EndTime), prepaidTime(sub.NextResetTime)
	status := 1
	if sub.Status != "active" {
		status = 3
	} else if sub.EndTime <= common.GetTimestamp() {
		status = 2
	}
	view["status"] = status
	view["discount_multiplier"], view["discount_savings_limit"], view["discount_savings_used"] = policy.ModelMultiplier(""), policy.DiscountSavingsLimit, policy.DiscountSavingsUsed
	view["requests_per_minute"], view["concurrent_requests"], view["upgrade_from_plan_id"] = policy.RequestsPerMinute, policy.ConcurrentRequests, policy.UpgradeFromPlanID
	rules := map[string]any{}
	for _, rule := range policy.Rules() {
		rules[rule.Model] = rule
	}
	view["model_rules"] = rules
	return view
}

func ListRelayForUserSubscriptions(userID int, timed bool) ([]map[string]any, error) {
	var subs []UserSubscription
	query := DB.Where("user_id = ? AND id NOT IN (?)", userID, DB.Model(&RelayForSubscriptionHidden{}).Select("subscription_id").Where("user_id = ?", userID))
	if timed {
		query = query.Where("source IN ?", relayForTimedSources)
	} else {
		query = query.Where("source NOT IN ?", relayForTimedSources)
	}
	if err := query.Order("end_time DESC, id DESC").Find(&subs).Error; err != nil {
		return nil, err
	}
	result := []map[string]any{}
	for _, sub := range subs {
		var plan SubscriptionPlan
		if err := DB.First(&plan, sub.PlanId).Error; err != nil {
			return nil, err
		}
		var policy RelayForSubscriptionPolicy
		err := DB.First(&policy, "plan_id = ?", sub.PlanId).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		snapshot := relayForPolicySnapshot(policy, sub.Id)
		err = DB.First(&snapshot, "subscription_id = ?", sub.Id).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		view := relayForSubscriptionView(sub, plan, snapshot, policy)
		if timed {
			status := 1
			if sub.Status != "active" || sub.EndTime <= common.GetTimestamp() {
				status = 3
			} else if sub.AmountTotal-sub.AmountUsed <= 0 {
				status = 2
			}
			view["status"], view["name"] = status, plan.Title
		}
		result = append(result, view)
	}
	return result, nil
}

func createRelayForTimedSubscriptionTx(tx *gorm.DB, userID int, name string, quota int64, expiresAt int64, source string) (*UserSubscription, error) {
	now := common.GetTimestamp()
	if quota <= 0 || expiresAt <= now {
		return nil, errors.New("限时额度或到期时间无效")
	}
	allowPay, overflow := false, true
	plan := SubscriptionPlan{Title: name, Currency: "USD", DurationUnit: SubscriptionDurationCustom, CustomSeconds: expiresAt - now, TotalAmount: quota, QuotaResetPeriod: SubscriptionResetNever, AllowBalancePay: &allowPay, AllowWalletOverflow: &overflow}
	if err := tx.Create(&plan).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&plan).Update("enabled", false).Error; err != nil {
		return nil, err
	}
	policy := RelayForSubscriptionPolicy{PlanID: plan.Id, Code: fmt.Sprintf("internal-%d", plan.Id), Internal: true, DiscountMultiplier: 1, ModelRulesJSON: "[]"}
	if err := tx.Create(&policy).Error; err != nil {
		return nil, err
	}
	sub := &UserSubscription{UserId: userID, PlanId: plan.Id, AmountTotal: quota, StartTime: now, EndTime: expiresAt, Status: "active", Source: source, AllowWalletOverflow: true}
	if err := tx.Create(sub).Error; err != nil {
		return nil, err
	}
	return sub, nil
}

type RelayForBenefitCampaign struct {
	ID                     int            `json:"id" gorm:"primaryKey"`
	PlanID                 int            `json:"plan_id" gorm:"index"`
	Name                   string         `json:"name" gorm:"size:320"`
	Description            string         `json:"description" gorm:"size:1200"`
	TargetUserGroup        string         `json:"target_user_group" gorm:"size:64"`
	QuotaGroup             string         `json:"quota_group" gorm:"size:64"`
	PermanentQuota         int            `json:"permanent_quota"`
	TimedQuota             int64          `json:"timed_quota"`
	TimedQuotaExpiresAt    int64          `json:"-"`
	RestoreQuotaPercent    float64        `json:"restore_quota_percent"`
	ExtendSubscriptionDays int            `json:"extend_subscription_days"`
	ResetSubscriptionQuota bool           `json:"reset_subscription_quota"`
	CreatedAt              int64          `json:"-"`
	CreatedBy              int            `json:"-"`
	DeletedAt              gorm.DeletedAt `json:"-" gorm:"index"`
}

type RelayForBenefitGrant struct {
	ID                  int `gorm:"primaryKey"`
	CampaignID          int `gorm:"uniqueIndex:idx_relayfor_grant_owner,priority:1;index"`
	UserID              int `gorm:"uniqueIndex:idx_relayfor_grant_owner,priority:2;index"`
	Status              int `gorm:"index"`
	ClaimExpiresAt      int64
	ClaimedAt           int64
	Dismissed           bool
	TimedSubscriptionID int
}

func CreateRelayForBenefitCampaign(campaign RelayForBenefitCampaign) (map[string]any, error) {
	now := common.GetTimestamp()
	campaign.Name = strings.TrimSpace(campaign.Name)
	if campaign.PlanID <= 0 || campaign.Name == "" || len([]rune(campaign.Name)) > 80 || len([]rune(campaign.Description)) > 300 || campaign.PermanentQuota < 0 || campaign.PermanentQuota > common.MaxWalletQuota || campaign.TimedQuota < 0 || campaign.TimedQuota > int64(common.MaxWalletQuota) || campaign.RestoreQuotaPercent < 0 || campaign.RestoreQuotaPercent > 100 || math.IsNaN(campaign.RestoreQuotaPercent) || campaign.ExtendSubscriptionDays < 0 || campaign.ExtendSubscriptionDays > 3650 || (campaign.ResetSubscriptionQuota && campaign.RestoreQuotaPercent > 0) || len(campaign.TargetUserGroup) > 64 || len(campaign.QuotaGroup) > 64 {
		return nil, errors.New("福利参数无效")
	}
	if campaign.PermanentQuota == 0 && campaign.TimedQuota == 0 && campaign.RestoreQuotaPercent == 0 && campaign.ExtendSubscriptionDays == 0 && !campaign.ResetSubscriptionQuota {
		return nil, errors.New("至少配置一种真实福利")
	}
	if campaign.TimedQuota > 0 && campaign.TimedQuotaExpiresAt > 0 && campaign.TimedQuotaExpiresAt <= now {
		return nil, errors.New("限时额度必须配置未来的到期时间")
	}
	if campaign.QuotaGroup != "" && campaign.TargetUserGroup != "" && campaign.QuotaGroup != campaign.TargetUserGroup {
		return nil, errors.New("额度用户组与目标用户组不一致")
	}
	var count int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var plan SubscriptionPlan
		if err := tx.First(&plan, campaign.PlanID).Error; err != nil {
			return errors.New("套餐不存在")
		}
		group := campaign.TargetUserGroup
		if group == "" {
			group = campaign.QuotaGroup
		}
		var recipients []struct {
			UserID         int
			ClaimExpiresAt int64
		}
		query := tx.Table("user_subscriptions AS s").Joins("JOIN users AS u ON u.id = s.user_id AND u.deleted_at IS NULL").Where("s.plan_id = ? AND s.status = ? AND s.start_time <= ? AND s.end_time > ? AND u.status = ?", campaign.PlanID, "active", now, now, common.UserStatusEnabled)
		if group != "" {
			query = query.Where(clause.Eq{Column: clause.Column{Table: "u", Name: "group"}, Value: group})
		}
		if err := query.Select("s.user_id, MAX(s.end_time) AS claim_expires_at").Group("s.user_id").Limit(100001).Scan(&recipients).Error; err != nil {
			return err
		}
		if len(recipients) > 100000 {
			return errors.New("单次福利发放最多10万名用户，请按用户组分批")
		}
		if len(recipients) == 0 {
			return errors.New("当前没有符合条件的有效套餐用户")
		}
		campaign.CreatedAt = now
		if err := tx.Create(&campaign).Error; err != nil {
			return err
		}
		grants := make([]RelayForBenefitGrant, 0, len(recipients))
		for _, recipient := range recipients {
			grants = append(grants, RelayForBenefitGrant{CampaignID: campaign.ID, UserID: recipient.UserID, Status: 1, ClaimExpiresAt: recipient.ClaimExpiresAt})
		}
		if err := tx.CreateInBatches(&grants, 200).Error; err != nil {
			return err
		}
		count = len(grants)
		return nil
	})
	if err != nil {
		return nil, err
	}
	view, err := relayForCampaignView(campaign)
	if err != nil {
		return nil, err
	}
	view["recipient_count"] = count
	return map[string]any{"campaign": view}, nil
}

func relayForCampaignView(campaign RelayForBenefitCampaign) (map[string]any, error) {
	encoded, _ := json.Marshal(campaign)
	view := map[string]any{}
	_ = json.Unmarshal(encoded, &view)
	view["created_at"] = prepaidTime(campaign.CreatedAt)
	view["timed_quota_expires_at"] = prepaidTime(campaign.TimedQuotaExpiresAt)
	var plan SubscriptionPlan
	if err := DB.First(&plan, campaign.PlanID).Error; err != nil {
		return nil, err
	}
	view["plan_name"] = plan.Title
	return view, nil
}

func ListRelayForBenefitCampaigns() ([]map[string]any, error) {
	var campaigns []RelayForBenefitCampaign
	if err := DB.Order("id DESC").Find(&campaigns).Error; err != nil {
		return nil, err
	}
	result := []map[string]any{}
	now := common.GetTimestamp()
	for _, campaign := range campaigns {
		view, err := relayForCampaignView(campaign)
		if err != nil {
			return nil, err
		}
		var counts struct {
			RecipientCount int64
			PendingCount   int64
			ClaimedCount   int64
			ExpiredCount   int64
			RevokedCount   int64
		}
		if err := DB.Model(&RelayForBenefitGrant{}).Where("campaign_id = ?", campaign.ID).Select("COUNT(*) AS recipient_count, SUM(CASE WHEN status=1 AND claim_expires_at > ? THEN 1 ELSE 0 END) AS pending_count, SUM(CASE WHEN status=2 THEN 1 ELSE 0 END) AS claimed_count, SUM(CASE WHEN status=1 AND claim_expires_at <= ? THEN 1 ELSE 0 END) AS expired_count, SUM(CASE WHEN status=4 THEN 1 ELSE 0 END) AS revoked_count", now, now).Scan(&counts).Error; err != nil {
			return nil, err
		}
		view["recipient_count"], view["pending_count"], view["claimed_count"], view["expired_count"], view["revoked_count"] = counts.RecipientCount, counts.PendingCount, counts.ClaimedCount, counts.ExpiredCount, counts.RevokedCount
		result = append(result, view)
	}
	return result, nil
}

func RevokeRelayForBenefitCampaign(id int, remove bool) (int64, error) {
	var affected int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var campaign RelayForBenefitCampaign
		if err := lockForUpdate(tx).First(&campaign, id).Error; err != nil {
			return err
		}
		if remove {
			var pending int64
			if err := tx.Model(&RelayForBenefitGrant{}).Where("campaign_id = ? AND status = 1 AND claim_expires_at > ?", id, common.GetTimestamp()).Count(&pending).Error; err != nil {
				return err
			}
			if pending > 0 {
				return errors.New("请先撤回待领取福利")
			}
			return tx.Delete(&campaign).Error
		}
		result := tx.Model(&RelayForBenefitGrant{}).Where("campaign_id = ? AND status = 1 AND claim_expires_at > ?", id, common.GetTimestamp()).Update("status", 4)
		affected = result.RowsAffected
		return result.Error
	})
	return affected, err
}

func ListRelayForUserBenefits(userID int) ([]map[string]any, error) {
	var grants []RelayForBenefitGrant
	if err := DB.Where("user_id = ? AND dismissed = ?", userID, false).Order("id DESC").Find(&grants).Error; err != nil {
		return nil, err
	}
	result := []map[string]any{}
	now := common.GetTimestamp()
	for _, grant := range grants {
		var campaign RelayForBenefitCampaign
		err := DB.First(&campaign, grant.CampaignID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		view, err := relayForCampaignView(campaign)
		if err != nil {
			return nil, err
		}
		status := grant.Status
		if status == 1 && grant.ClaimExpiresAt <= now {
			status = 3
		}
		view["id"], view["campaign_id"], view["status"], view["can_claim"] = grant.ID, grant.CampaignID, status, status == 1
		view["claim_expires_at"], view["claimed_at"] = prepaidTime(grant.ClaimExpiresAt), prepaidTime(grant.ClaimedAt)
		result = append(result, view)
	}
	return result, nil
}

func ClaimRelayForBenefit(userID, grantID int) (map[string]any, error) {
	var grant RelayForBenefitGrant
	credited := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", grantID, userID).First(&grant).Error; err != nil {
			return err
		}
		if grant.Status == 2 {
			return nil
		}
		now := common.GetTimestamp()
		if grant.Status != 1 || grant.ClaimExpiresAt <= now {
			return errors.New("福利已失效或已撤回")
		}
		var campaign RelayForBenefitCampaign
		if err := tx.First(&campaign, grant.CampaignID).Error; err != nil {
			return errors.New("福利活动不存在")
		}
		var user User
		if err := lockForUpdate(tx).First(&user, userID).Error; err != nil {
			return err
		}
		if user.Status != common.UserStatusEnabled {
			return errors.New("账户不可用")
		}
		if campaign.QuotaGroup != "" && user.Group != campaign.QuotaGroup {
			return errors.New("当前用户组不符合福利要求")
		}
		var subs []UserSubscription
		if err := lockForUpdate(tx).Where("user_id = ? AND plan_id = ? AND status = ? AND end_time > ?", userID, campaign.PlanID, "active", now).Find(&subs).Error; err != nil {
			return err
		}
		if len(subs) == 0 {
			return errors.New("关联套餐已失效，无法领取福利")
		}
		claimed := tx.Model(&RelayForBenefitGrant{}).Where("id = ? AND user_id = ? AND status = 1 AND claim_expires_at > ?", grantID, userID, now).Updates(map[string]any{"status": 2, "claimed_at": now})
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected != 1 {
			return errors.New("福利状态已变更，请刷新")
		}
		if campaign.PermanentQuota > 0 {
			if err := creditTopUpQuota(tx, userID, campaign.PermanentQuota, nil); err != nil {
				return err
			}
			credited = campaign.PermanentQuota
		}
		if campaign.TimedQuota > 0 {
			expiry := campaign.TimedQuotaExpiresAt
			if expiry == 0 {
				expiry = grant.ClaimExpiresAt + int64(campaign.ExtendSubscriptionDays)*86400
			}
			timed, err := createRelayForTimedSubscriptionTx(tx, userID, campaign.Name, campaign.TimedQuota, expiry, "relayfor_benefit_timed")
			if err != nil {
				return err
			}
			grant.TimedSubscriptionID = timed.Id
			if err := tx.Model(&RelayForBenefitGrant{}).Where("id = ?", grantID).Update("timed_subscription_id", timed.Id).Error; err != nil {
				return err
			}
		}
		for _, sub := range subs {
			updates := map[string]any{}
			if campaign.ResetSubscriptionQuota {
				updates["amount_used"] = 0
			} else if campaign.RestoreQuotaPercent > 0 && sub.AmountTotal > 0 {
				restore := int64(math.Ceil(float64(sub.AmountTotal) * campaign.RestoreQuotaPercent / 100))
				updates["amount_used"] = max(int64(0), sub.AmountUsed-restore)
			}
			if campaign.ExtendSubscriptionDays > 0 {
				updates["end_time"] = sub.EndTime + int64(campaign.ExtendSubscriptionDays)*86400
			}
			if len(updates) > 0 {
				if err := tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Updates(updates).Error; err != nil {
					return err
				}
			}
		}
		grant.Status, grant.ClaimedAt = 2, now
		return nil
	})
	if err != nil {
		return nil, err
	}
	if credited > 0 {
		syncCreditUserQuotaCache(userID, credited, "subscription benefit")
	}
	return map[string]any{"id": grant.ID, "status": grant.Status, "claimed_at": prepaidTime(grant.ClaimedAt), "timed_subscription_id": grant.TimedSubscriptionID}, nil
}

func DismissRelayForBenefit(userID, grantID int) error {
	result := DB.Model(&RelayForBenefitGrant{}).Where("id = ? AND user_id = ? AND status = 2", grantID, userID).Update("dismissed", true)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("仅可忽略自己已领取的福利")
	}
	return nil
}

func relayForDBTimestampTx(tx *gorm.DB) int64 {
	query := "SELECT UNIX_TIMESTAMP()"
	switch common.MainDatabaseType() {
	case common.DatabaseTypeSQLite:
		query = "SELECT strftime('%s','now')"
	case common.DatabaseTypePostgreSQL:
		query = "SELECT EXTRACT(EPOCH FROM NOW())::bigint"
	}
	var now int64
	if err := tx.Raw(query).Scan(&now).Error; err != nil || now <= 0 {
		return common.GetTimestamp()
	}
	return now
}

func HideRelayForUserSubscription(userID, subscriptionID int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var sub UserSubscription
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", subscriptionID, userID).First(&sub).Error; err != nil {
			return err
		}
		if sub.Status == "active" && sub.EndTime > common.GetTimestamp() {
			return errors.New("有效套餐需要先放弃")
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&RelayForSubscriptionHidden{UserID: userID, SubscriptionID: subscriptionID}).Error
	})
}

func AbandonRelayForSubscription(userID, subscriptionID int, mode string) (map[string]any, error) {
	if mode != "convert" && mode != "end" {
		return nil, errors.New("无效的放弃方式")
	}
	converted := int64(0)
	affected := []int{}
	groupChanged := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		now := common.GetTimestamp()
		var subs []UserSubscription
		if err := lockForUpdate(tx).Where("user_id = ? AND status = ? AND end_time > ?", userID, "active", now).Find(&subs).Error; err != nil {
			return err
		}
		byID := map[int]UserSubscription{}
		for _, sub := range subs {
			byID[sub.Id] = sub
		}
		target, exists := byID[subscriptionID]
		if !exists {
			return errors.New("套餐不存在或已结束")
		}
		if mode == "convert" && target.AmountTotal <= 0 {
			return errors.New("不限额套餐不能转换为固定额度")
		}
		var snapshots []RelayForUserSubscriptionPolicy
		if err := tx.Where("subscription_id IN (?)", tx.Model(&UserSubscription{}).Select("id").Where("user_id = ?", userID)).Find(&snapshots).Error; err != nil {
			return err
		}
		dependencies := map[int]int{}
		for _, snapshot := range snapshots {
			dependencies[snapshot.SubscriptionID] = snapshot.UpgradeFromPlanID
		}
		selected := map[int]bool{subscriptionID: true}
		for changed := true; changed; {
			changed = false
			remaining := map[int]bool{}
			removed := map[int]bool{}
			for _, sub := range subs {
				if selected[sub.Id] {
					removed[sub.PlanId] = true
				} else {
					remaining[sub.PlanId] = true
				}
			}
			for _, sub := range subs {
				base := dependencies[sub.Id]
				if !selected[sub.Id] && base > 0 && removed[base] && !remaining[base] {
					selected[sub.Id] = true
					changed = true
				}
			}
		}
		for _, sub := range subs {
			if !selected[sub.Id] {
				continue
			}
			if mode == "convert" && sub.AmountTotal > 0 {
				remaining := max(int64(0), sub.AmountTotal-sub.AmountUsed)
				if remaining > 0 {
					var plan SubscriptionPlan
					if err := tx.First(&plan, sub.PlanId).Error; err != nil {
						return err
					}
					if _, err := createRelayForTimedSubscriptionTx(tx, userID, plan.Title+" · 转换额度", remaining, sub.EndTime, "relayfor_converted"); err != nil {
						return err
					}
					converted += remaining
				}
			}
			if err := tx.Model(&UserSubscription{}).Where("id = ? AND status = ?", sub.Id, "active").Updates(map[string]any{"status": "cancelled", "end_time": now}).Error; err != nil {
				return err
			}
			affected = append(affected, sub.Id)
		}
		for _, id := range affected {
			sub := byID[id]
			group, err := downgradeUserGroupForSubscriptionTx(tx, &sub, now)
			if err != nil {
				return err
			}
			groupChanged = groupChanged || group != ""
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if groupChanged {
		refreshSubscriptionUserGroupCache(userID, "user subscription abandonment")
	}
	return map[string]any{"result": map[string]any{"converted_quota": converted, "affected_subscriptions": affected}}, nil
}

func SetRelayForBillingPriority(userID int, timedFirst bool) error {
	return DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"timed_first"})}).Create(&RelayForSubscriptionPreference{UserID: userID, TimedFirst: timedFirst}).Error
}

func GetRelayForBillingPriority(userID int) (bool, error) {
	var pref RelayForSubscriptionPreference
	err := DB.First(&pref, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return pref.TimedFirst, err
}

func relayForSubscriptionOrderTx(tx *gorm.DB, userID int) (string, error) {
	if !tx.Migrator().HasTable(&RelayForSubscriptionPreference{}) {
		return "end_time asc, id asc", nil
	}
	var pref RelayForSubscriptionPreference
	err := tx.First(&pref, "user_id = ?", userID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	first, last := 0, 1
	if !pref.TimedFirst {
		first, last = 1, 0
	}
	return fmt.Sprintf("CASE WHEN source IN ('relayfor_benefit_timed','relayfor_converted','relayfor_prepaid') THEN %d ELSE %d END, end_time asc, id asc", first, last), nil
}

// GrantRelayForSubscriptionTx is shared by paid orders, redemption codes and
// administrator grants. Call RefreshUserGroupCache after the transaction
// commits when the returned subscription changes the user's group.
func GrantRelayForSubscriptionTx(tx *gorm.DB, userID, planID int, source string) (*UserSubscription, error) {
	var user User
	if err := lockForUpdate(tx).First(&user, userID).Error; err != nil {
		return nil, err
	}
	if user.Status != common.UserStatusEnabled {
		return nil, errors.New("账户不可用")
	}
	var plan SubscriptionPlan
	if err := tx.First(&plan, planID).Error; err != nil {
		return nil, err
	}
	if !plan.Enabled {
		return nil, errors.New("套餐已停用")
	}
	return CreateUserSubscriptionFromPlanTx(tx, userID, &plan, source)
}

func ExtendRelayForSubscriptionTx(tx *gorm.DB, userID, planID, days int) (int, error) {
	if days < 1 || days > 3650 {
		return 0, errors.New("展期天数必须为1到3650天")
	}
	var subs []UserSubscription
	if err := lockForUpdate(tx).Where("user_id = ? AND plan_id = ? AND status = ? AND end_time > ?", userID, planID, "active", common.GetTimestamp()).Find(&subs).Error; err != nil {
		return 0, err
	}
	if len(subs) == 0 {
		return 0, errors.New("没有可展期的有效套餐")
	}
	for _, sub := range subs {
		if err := tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Update("end_time", sub.EndTime+int64(days)*86400).Error; err != nil {
			return 0, err
		}
	}
	return len(subs), nil
}
