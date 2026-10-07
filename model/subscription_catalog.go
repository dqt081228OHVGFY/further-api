package model

import (
	"context"
	"errors"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/setting/ratio_setting"
	"gorm.io/gorm"
)

// SubscriptionPlanInput is the editable catalog contract. Persistence IDs,
// timestamps and entitlement snapshots can never be supplied by an editor.
type SubscriptionPlanInput struct {
	Title                   string  `json:"title"`
	Subtitle                string  `json:"subtitle"`
	PriceAmount             float64 `json:"price_amount"`
	Currency                string  `json:"currency"`
	DurationUnit            string  `json:"duration_unit"`
	DurationValue           int     `json:"duration_value"`
	CustomSeconds           int64   `json:"custom_seconds"`
	Enabled                 *bool   `json:"enabled"`
	SortOrder               int     `json:"sort_order"`
	AllowBalancePay         *bool   `json:"allow_balance_pay"`
	AllowWalletOverflow     *bool   `json:"allow_wallet_overflow"`
	StripePriceId           string  `json:"stripe_price_id"`
	CreemProductId          string  `json:"creem_product_id"`
	WaffoPancakeProductId   string  `json:"waffo_pancake_product_id"`
	MaxPurchasePerUser      int     `json:"max_purchase_per_user"`
	UpgradeGroup            string  `json:"upgrade_group"`
	DowngradeGroup          string  `json:"downgrade_group"`
	TotalAmount             int64   `json:"total_amount"`
	QuotaResetPeriod        string  `json:"quota_reset_period"`
	QuotaResetCustomSeconds int64   `json:"quota_reset_custom_seconds"`
}

var ErrSubscriptionPlanProtected = errors.New("套餐不存在或不可编辑")

func (input SubscriptionPlanInput) normalized() (SubscriptionPlanInput, error) {
	input.Title, input.Subtitle = strings.TrimSpace(input.Title), strings.TrimSpace(input.Subtitle)
	if input.Title == "" || utf8.RuneCountInString(input.Title) > 128 {
		return input, errors.New("套餐标题需为 1 至 128 个字符")
	}
	if utf8.RuneCountInString(input.Subtitle) > 255 {
		return input, errors.New("套餐说明不能超过 255 个字符")
	}
	if math.IsNaN(input.PriceAmount) || math.IsInf(input.PriceAmount, 0) || input.PriceAmount < 0 || input.PriceAmount > 9999 {
		return input, errors.New("套餐价格需在 0 至 9999 之间")
	}
	input.Currency = "USD"
	input.DurationUnit = strings.TrimSpace(input.DurationUnit)
	if input.DurationUnit == "" {
		input.DurationUnit = SubscriptionDurationMonth
	}
	switch input.DurationUnit {
	case SubscriptionDurationYear, SubscriptionDurationMonth, SubscriptionDurationDay, SubscriptionDurationHour:
		if input.DurationValue <= 0 {
			input.DurationValue = 1
		}
		if input.DurationValue > 10000 {
			return input, errors.New("套餐有效期过长")
		}
	case SubscriptionDurationCustom:
		if input.CustomSeconds <= 0 || input.CustomSeconds > 3153600000 {
			return input, errors.New("自定义有效期需为 1 秒至 100 年")
		}
	default:
		return input, errors.New("套餐有效期单位无效")
	}
	if input.MaxPurchasePerUser < 0 || input.TotalAmount < 0 {
		return input, errors.New("购买上限与总额度不能为负数")
	}
	input.UpgradeGroup, input.DowngradeGroup = strings.TrimSpace(input.UpgradeGroup), strings.TrimSpace(input.DowngradeGroup)
	groups := ratio_setting.GetGroupRatioCopy()
	for _, group := range []string{input.UpgradeGroup, input.DowngradeGroup} {
		if group != "" {
			if _, exists := groups[group]; !exists {
				return input, errors.New("套餐指定的用户分组不存在")
			}
		}
	}
	input.QuotaResetPeriod = strings.TrimSpace(input.QuotaResetPeriod)
	if input.QuotaResetPeriod == "" {
		input.QuotaResetPeriod = SubscriptionResetNever
	}
	switch input.QuotaResetPeriod {
	case SubscriptionResetNever, SubscriptionResetDaily, SubscriptionResetWeekly, SubscriptionResetMonthly:
	case SubscriptionResetCustom:
		if input.QuotaResetCustomSeconds <= 0 || input.QuotaResetCustomSeconds > 3153600000 {
			return input, errors.New("自定义额度重置周期需为 1 秒至 100 年")
		}
	default:
		return input, errors.New("额度重置周期无效")
	}
	for _, identifier := range []string{input.StripePriceId, input.CreemProductId, input.WaffoPancakeProductId} {
		if len(identifier) > 128 {
			return input, errors.New("支付产品编号不能超过 128 个字符")
		}
	}
	return input, nil
}

func subscriptionCatalogQuery(db *gorm.DB) *gorm.DB {
	query := db.Model(&SubscriptionPlan{})
	if db.Migrator().HasTable(&FurtherSubscriptionPolicy{}) {
		query = query.Where("id NOT IN (?)", db.Model(&FurtherSubscriptionPolicy{}).Select("plan_id").Where("internal = ?", true))
	}
	if db.Migrator().HasTable(&FurtherPrepaidCard{}) {
		query = query.Where("id NOT IN (?)", db.Unscoped().Model(&FurtherPrepaidCard{}).Select("plan_id").Where("plan_id > 0"))
	}
	return query
}

func ListSubscriptionCatalog(ctx context.Context, enabledOnly bool) ([]SubscriptionPlan, error) {
	plans := make([]SubscriptionPlan, 0)
	query := subscriptionCatalogQuery(DB.WithContext(ctx))
	if enabledOnly {
		query = query.Where("enabled = ?", true)
	}
	if err := query.Order("sort_order DESC, id DESC").Find(&plans).Error; err != nil {
		return nil, err
	}
	for index := range plans {
		plans[index].NormalizeDefaults()
	}
	return plans, nil
}

func (input SubscriptionPlanInput) applyTo(plan *SubscriptionPlan) {
	plan.Title, plan.Subtitle = input.Title, input.Subtitle
	plan.PriceAmount, plan.Currency = input.PriceAmount, input.Currency
	plan.DurationUnit, plan.DurationValue, plan.CustomSeconds = input.DurationUnit, input.DurationValue, input.CustomSeconds
	if input.Enabled != nil {
		plan.Enabled = *input.Enabled
	}
	plan.SortOrder = input.SortOrder
	if input.AllowBalancePay != nil {
		plan.AllowBalancePay = common.GetPointer(*input.AllowBalancePay)
	}
	if input.AllowWalletOverflow != nil {
		plan.AllowWalletOverflow = common.GetPointer(*input.AllowWalletOverflow)
	}
	plan.StripePriceId, plan.CreemProductId, plan.WaffoPancakeProductId = input.StripePriceId, input.CreemProductId, input.WaffoPancakeProductId
	plan.MaxPurchasePerUser, plan.TotalAmount = input.MaxPurchasePerUser, input.TotalAmount
	plan.UpgradeGroup, plan.DowngradeGroup = input.UpgradeGroup, input.DowngradeGroup
	plan.QuotaResetPeriod, plan.QuotaResetCustomSeconds = input.QuotaResetPeriod, input.QuotaResetCustomSeconds
	plan.NormalizeDefaults()
}

// SaveSubscriptionCatalog serializes edits against a current row and selects
// only catalog columns. User entitlement snapshots are deliberately separate.
func SaveSubscriptionCatalog(ctx context.Context, id int, input SubscriptionPlanInput) (*SubscriptionPlan, error) {
	if id < 0 {
		return nil, ErrSubscriptionPlanProtected
	}
	input, err := input.normalized()
	if err != nil {
		return nil, err
	}
	var plan SubscriptionPlan
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if id > 0 {
			if err := lockForUpdate(subscriptionCatalogQuery(tx)).First(&plan, id).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrSubscriptionPlanProtected
				}
				return err
			}
		} else {
			plan.Enabled = true
		}
		input.applyTo(&plan)
		if id == 0 {
			enabled := plan.Enabled
			if err := tx.Create(&plan).Error; err != nil {
				return err
			}
			// GORM applies a true insert default even when false was supplied.
			if !enabled {
				if err := tx.Model(&plan).Update("enabled", false).Error; err != nil {
					return err
				}
				plan.Enabled = false
			}
			return nil
		}
		return tx.Model(&plan).Select("title", "subtitle", "price_amount", "currency", "duration_unit", "duration_value", "custom_seconds", "enabled", "sort_order", "allow_balance_pay", "allow_wallet_overflow", "stripe_price_id", "creem_product_id", "waffo_pancake_product_id", "max_purchase_per_user", "total_amount", "upgrade_group", "downgrade_group", "quota_reset_period", "quota_reset_custom_seconds", "updated_at").Updates(&plan).Error
	})
	if err != nil {
		return nil, err
	}
	InvalidateSubscriptionPlanCache(plan.Id)
	return &plan, nil
}

func SetSubscriptionCatalogEnabled(ctx context.Context, id int, enabled bool) error {
	if id <= 0 {
		return ErrSubscriptionPlanProtected
	}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var plan SubscriptionPlan
		if err := lockForUpdate(subscriptionCatalogQuery(tx)).First(&plan, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSubscriptionPlanProtected
			}
			return err
		}
		if plan.Enabled == enabled {
			return nil
		}
		return tx.Model(&plan).Updates(map[string]any{"enabled": enabled, "updated_at": common.GetTimestamp()}).Error
	})
	if err == nil {
		InvalidateSubscriptionPlanCache(id)
	}
	return err
}

type SubscriptionWorkspace struct {
	BillingPreference string                `json:"billing_preference"`
	Subscriptions     []SubscriptionSummary `json:"subscriptions"`
	AllSubscriptions  []SubscriptionSummary `json:"all_subscriptions"`
}

func ReadSubscriptionWorkspace(ctx context.Context, userID int) (*SubscriptionWorkspace, error) {
	if userID <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var user User
	if err := DB.WithContext(ctx).Select("id", "setting").First(&user, userID).Error; err != nil {
		return nil, err
	}
	all, err := listUserSubscriptionSummaries(DB.WithContext(ctx), userID, false)
	if err != nil {
		return nil, err
	}
	workspace := &SubscriptionWorkspace{BillingPreference: common.NormalizeBillingPreference(user.GetSetting().BillingPreference), AllSubscriptions: all, Subscriptions: make([]SubscriptionSummary, 0)}
	now := common.GetTimestamp()
	for _, summary := range all {
		if sub := summary.Subscription; sub.Status == "active" && sub.StartTime <= now && sub.EndTime > now {
			workspace.Subscriptions = append(workspace.Subscriptions, summary)
		}
	}
	return workspace, nil
}

func SetSubscriptionBillingPreference(ctx context.Context, userID int, preference string) (string, error) {
	preference = strings.TrimSpace(preference)
	switch preference {
	case "subscription_first", "wallet_first", "subscription_only", "wallet_only":
	default:
		return "", errors.New("请选择有效的扣费方式")
	}
	if userID <= 0 {
		return "", gorm.ErrRecordNotFound
	}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).First(&user, userID).Error; err != nil {
			return err
		}
		settings := user.GetSetting()
		settings.BillingPreference = preference
		user.SetSetting(settings)
		return tx.Model(&user).Update("setting", user.Setting).Error
	})
	if err != nil {
		return "", err
	}
	if err := PublishUserAuthCache(userID); err != nil {
		return "", err
	}
	return preference, nil
}
