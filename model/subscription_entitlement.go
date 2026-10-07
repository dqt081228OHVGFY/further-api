package model

import (
	"errors"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
)

func calcPlanEndTime(start time.Time, plan *SubscriptionPlan) (int64, error) {
	if plan == nil {
		return 0, errors.New("套餐不存在")
	}
	var end int64
	count := plan.DurationValue
	switch plan.DurationUnit {
	case SubscriptionDurationYear, SubscriptionDurationMonth:
		if count <= 0 || count > 10000 {
			return 0, errors.New("套餐日历有效期无效")
		}
		if plan.DurationUnit == SubscriptionDurationYear {
			end = start.AddDate(count, 0, 0).Unix()
		} else {
			end = start.AddDate(0, count, 0).Unix()
		}
	case SubscriptionDurationDay, SubscriptionDurationHour, SubscriptionDurationCustom:
		seconds := plan.CustomSeconds
		if plan.DurationUnit != SubscriptionDurationCustom {
			unit := int64(3600)
			if plan.DurationUnit == SubscriptionDurationDay {
				unit *= 24
			}
			if count <= 0 || int64(count) > math.MaxInt64/unit {
				return 0, errors.New("套餐有效期无效")
			}
			seconds = int64(count) * unit
		}
		if seconds <= 0 || start.Unix() > math.MaxInt64-seconds {
			return 0, errors.New("套餐有效期超出可用范围")
		}
		end = start.Unix() + seconds
	default:
		return 0, errors.New("套餐有效期单位无效")
	}
	if end <= start.Unix() {
		return 0, errors.New("套餐到期时间无效")
	}
	return end, nil
}

func NormalizeResetPeriod(period string) string {
	period = strings.TrimSpace(period)
	switch period {
	case SubscriptionResetDaily, SubscriptionResetWeekly, SubscriptionResetMonthly, SubscriptionResetCustom:
		return period
	default:
		return SubscriptionResetNever
	}
}

func calcNextResetTime(base time.Time, plan *SubscriptionPlan, endUnix int64) int64 {
	if plan == nil {
		return 0
	}
	midnight := time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, base.Location())
	var next int64
	switch NormalizeResetPeriod(plan.QuotaResetPeriod) {
	case SubscriptionResetDaily:
		next = midnight.AddDate(0, 0, 1).Unix()
	case SubscriptionResetWeekly:
		days := (8 - int(base.Weekday())) % 7
		if days == 0 {
			days = 7
		}
		next = midnight.AddDate(0, 0, days).Unix()
	case SubscriptionResetMonthly:
		next = time.Date(base.Year(), base.Month()+1, 1, 0, 0, 0, 0, base.Location()).Unix()
	case SubscriptionResetCustom:
		seconds := plan.QuotaResetCustomSeconds
		if seconds <= 0 || base.Unix() > math.MaxInt64-seconds {
			return 0
		}
		next = base.Unix() + seconds
	default:
		return 0
	}
	if next <= base.Unix() || (endUnix > 0 && next > endUnix) {
		return 0
	}
	return next
}

// A grant snapshots the plan after acquiring the account lock, so every
// purchase path shares the same purchase-limit and group transition rules.
func CreateUserSubscriptionFromPlanTx(tx *gorm.DB, userID int, plan *SubscriptionPlan, source string) (*UserSubscription, error) {
	if tx == nil || userID <= 0 || plan == nil || plan.Id <= 0 {
		return nil, errors.New("订阅发放参数无效")
	}
	var account User
	if err := lockForUpdate(tx).Select("id", "group").First(&account, userID).Error; err != nil {
		return nil, err
	}
	if plan.MaxPurchasePerUser > 0 {
		var issued int64
		if err := tx.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ?", userID, plan.Id).Count(&issued).Error; err != nil {
			return nil, err
		}
		if issued >= int64(plan.MaxPurchasePerUser) {
			return nil, errors.New("已达到该套餐购买上限")
		}
	}
	now := furtherDBTimestampTx(tx)
	end, err := calcPlanEndTime(time.Unix(now, 0), plan)
	if err != nil {
		return nil, err
	}
	upgrade := strings.TrimSpace(plan.UpgradeGroup)
	previous := ""
	if upgrade != "" && upgrade != account.Group {
		previous = account.Group
		if err := tx.Model(&account).Update("group", upgrade).Error; err != nil {
			return nil, err
		}
	}
	next := calcNextResetTime(time.Unix(now, 0), plan, end)
	subscription := &UserSubscription{
		UserId: userID, PlanId: plan.Id, AmountTotal: plan.TotalAmount,
		StartTime: now, EndTime: end, Status: "active", Source: source,
		NextResetTime: next, UpgradeGroup: upgrade, PrevUserGroup: previous,
		DowngradeGroup:      strings.TrimSpace(plan.DowngradeGroup),
		AllowWalletOverflow: plan.AllowWalletOverflow == nil || *plan.AllowWalletOverflow,
		CreatedAt:           now, UpdatedAt: now,
	}
	if next > 0 {
		subscription.LastResetTime = now
	}
	if err := tx.Create(subscription).Error; err != nil {
		return nil, err
	}
	if err := AttachFurtherSubscriptionPolicyTx(tx, subscription, plan); err != nil {
		return nil, err
	}
	return subscription, nil
}

func getUserGroupByIdTx(tx *gorm.DB, userID int) (string, error) {
	if userID <= 0 {
		return "", errors.New("用户编号无效")
	}
	if tx == nil {
		tx = DB
	}
	var account User
	err := lockForUpdate(tx).Select("id", "group").First(&account, userID).Error
	return account.Group, err
}

func downgradeUserGroupForSubscriptionTx(tx *gorm.DB, subscription *UserSubscription, now int64) (string, error) {
	if tx == nil || subscription == nil {
		return "", errors.New("订阅分组变更参数无效")
	}
	upgrade, fallback := strings.TrimSpace(subscription.UpgradeGroup), strings.TrimSpace(subscription.DowngradeGroup)
	if upgrade == "" && fallback == "" {
		return "", nil
	}
	current, err := getUserGroupByIdTx(tx, subscription.UserId)
	if err != nil {
		return "", err
	}
	var surviving UserSubscription
	result := tx.Select("id", "upgrade_group").Where("user_id = ? AND id <> ? AND status = ? AND start_time <= ? AND end_time > ? AND upgrade_group <> ''", subscription.UserId, subscription.Id, "active", now, now).Order("start_time DESC, id DESC").Limit(1).Find(&surviving)
	if result.Error != nil {
		return "", result.Error
	}
	target := fallback
	if result.RowsAffected > 0 {
		target = strings.TrimSpace(surviving.UpgradeGroup)
	} else if target == "" && current == upgrade {
		target = strings.TrimSpace(subscription.PrevUserGroup)
	}
	if target == "" || target == current {
		return "", nil
	}
	if err := tx.Model(&User{}).Where("id = ?", subscription.UserId).Update("group", target).Error; err != nil {
		return "", err
	}
	return target, nil
}
