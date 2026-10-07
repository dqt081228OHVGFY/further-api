package model

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

func resetUserSubscriptionTx(tx *gorm.DB, subscription *UserSubscription, plan *SubscriptionPlan, now int64, advance bool) error {
	if tx == nil || subscription == nil || plan == nil {
		return errors.New("订阅额度重置参数无效")
	}
	updates := map[string]any{"amount_used": int64(0), "updated_at": now}
	subscription.AmountUsed = 0
	if advance {
		subscription.NextResetTime = calcNextResetTime(time.Unix(now, 0), plan, subscription.EndTime)
		subscription.LastResetTime = 0
		if subscription.NextResetTime > 0 {
			subscription.LastResetTime = now
		}
		updates["next_reset_time"], updates["last_reset_time"] = subscription.NextResetTime, subscription.LastResetTime
	}
	return tx.Model(&UserSubscription{}).Where("id = ?", subscription.Id).Updates(updates).Error
}

func adminResetSubscriptionsTx(tx *gorm.DB, userID int, plan *SubscriptionPlan, now int64, advance bool) (*SubscriptionResetResult, error) {
	if tx == nil || plan == nil {
		return nil, errors.New("订阅额度重置参数无效")
	}
	query := lockForUpdate(tx).Where("plan_id = ? AND status = ? AND start_time <= ? AND end_time > ?", plan.Id, "active", now, now)
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	var subscriptions []UserSubscription
	if err := query.Order("user_id, end_time, id").Find(&subscriptions).Error; err != nil {
		return nil, err
	}
	if userID > 0 && len(subscriptions) == 0 {
		return nil, errors.New("该用户没有有效的此套餐订阅")
	}
	result := &SubscriptionResetResult{PlanId: plan.Id, PlanTitle: plan.Title, MatchedCount: len(subscriptions), AdvanceResetTime: advance, AffectedUserIds: make([]int, 0)}
	seen := make(map[int]bool)
	for index := range subscriptions {
		if err := resetUserSubscriptionTx(tx, &subscriptions[index], plan, now, advance); err != nil {
			return nil, err
		}
		result.ResetCount++
		id := subscriptions[index].UserId
		if !seen[id] {
			seen[id] = true
			result.AffectedUserIds = append(result.AffectedUserIds, id)
		}
	}
	result.UserCount = len(result.AffectedUserIds)
	return result, nil
}

func adminResetUserSubscriptionsByPlanTx(tx *gorm.DB, userID int, plan *SubscriptionPlan, now int64, advance bool) (*SubscriptionResetResult, error) {
	return adminResetSubscriptionsTx(tx, userID, plan, now, advance)
}

func adminResetPlanSubscriptionsTx(tx *gorm.DB, plan *SubscriptionPlan, now int64, advance bool) (*SubscriptionResetResult, error) {
	return adminResetSubscriptionsTx(tx, 0, plan, now, advance)
}

func runAdminSubscriptionReset(ctx context.Context, userID, planID int, advance bool) (*SubscriptionResetResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	if userID < 0 || planID <= 0 {
		return nil, errors.New("订阅额度重置编号无效")
	}
	var result *SubscriptionResetResult
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		plan, err := getSubscriptionPlanByIdTx(tx, planID)
		if err != nil {
			return err
		}
		result, err = adminResetSubscriptionsTx(tx, userID, plan, furtherDBTimestampTx(tx), advance)
		return err
	})
	return result, err
}

func AdminResetUserSubscriptionsByPlan(userID, planID int, advance bool) (*SubscriptionResetResult, error) {
	return AdminResetUserSubscriptionsByPlanContext(context.Background(), userID, planID, advance)
}

func AdminResetUserSubscriptionsByPlanContext(ctx context.Context, userID, planID int, advance bool) (*SubscriptionResetResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if userID <= 0 {
		return nil, errors.New("用户编号无效")
	}
	return runAdminSubscriptionReset(ctx, userID, planID, advance)
}

func AdminResetPlanSubscriptions(planID int, advance bool) (*SubscriptionResetResult, error) {
	return AdminResetPlanSubscriptionsContext(context.Background(), planID, advance)
}

func AdminResetPlanSubscriptionsContext(ctx context.Context, planID int, advance bool) (*SubscriptionResetResult, error) {
	return runAdminSubscriptionReset(ctx, 0, planID, advance)
}

// Calculate the last elapsed boundary directly. A one-second custom period
// never loops over every missed second after a server has been offline.
func maybeResetUserSubscriptionWithPlanTx(tx *gorm.DB, subscription *UserSubscription, plan *SubscriptionPlan, now int64) error {
	if tx == nil || subscription == nil || plan == nil {
		return errors.New("订阅额度重置参数无效")
	}
	if subscription.NextResetTime > now || NormalizeResetPeriod(plan.QuotaResetPeriod) == SubscriptionResetNever {
		return nil
	}
	anchor := subscription.LastResetTime
	if anchor <= 0 {
		anchor = subscription.StartTime
	}
	next := calcNextResetTime(time.Unix(anchor, 0), plan, subscription.EndTime)
	if next == 0 {
		return nil
	}
	last := anchor
	due := next <= now
	if due {
		clock := time.Unix(now, 0)
		midnight := time.Date(clock.Year(), clock.Month(), clock.Day(), 0, 0, 0, 0, clock.Location())
		switch NormalizeResetPeriod(plan.QuotaResetPeriod) {
		case SubscriptionResetDaily:
			last = midnight.Unix()
		case SubscriptionResetWeekly:
			last = midnight.AddDate(0, 0, -(int(clock.Weekday())+6)%7).Unix()
		case SubscriptionResetMonthly:
			last = time.Date(clock.Year(), clock.Month(), 1, 0, 0, 0, 0, clock.Location()).Unix()
		case SubscriptionResetCustom:
			last = anchor + ((now-anchor)/plan.QuotaResetCustomSeconds)*plan.QuotaResetCustomSeconds
		}
		next = calcNextResetTime(time.Unix(last, 0), plan, subscription.EndTime)
	}
	if !due && subscription.NextResetTime != 0 {
		return nil
	}
	updates := map[string]any{"last_reset_time": last, "next_reset_time": next, "updated_at": now}
	if due {
		updates["amount_used"] = int64(0)
		subscription.AmountUsed = 0
	}
	subscription.LastResetTime, subscription.NextResetTime = last, next
	return tx.Model(&UserSubscription{}).Where("id = ?", subscription.Id).Updates(updates).Error
}

func subscriptionWorkerLimit(limit int) int {
	if limit <= 0 {
		return 200
	}
	return limit
}

func ExpireDueSubscriptions(limit int) (int, error) {
	return ExpireDueSubscriptionsContext(context.Background(), limit)
}

func ExpireDueSubscriptionsContext(ctx context.Context, limit int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	db := DB.WithContext(ctx)
	now := GetDBTimestampContext(ctx)
	var candidates []UserSubscription
	if err := db.Where("status = ? AND end_time > 0 AND end_time <= ?", "active", now).Order("end_time, id").Limit(subscriptionWorkerLimit(limit)).Find(&candidates).Error; err != nil {
		return 0, err
	}
	byOwner := make(map[int][]int)
	owners := make([]int, 0)
	for _, candidate := range candidates {
		if _, exists := byOwner[candidate.UserId]; !exists {
			owners = append(owners, candidate.UserId)
		}
		byOwner[candidate.UserId] = append(byOwner[candidate.UserId], candidate.Id)
	}
	total := 0
	for _, owner := range owners {
		changed, count := false, 0
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := lockSubscriptionOwner(tx, owner); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			var expired []UserSubscription
			if err := lockForUpdate(tx).Where("id IN ? AND status = ? AND end_time <= ?", byOwner[owner], "active", now).Order("end_time DESC, id DESC").Find(&expired).Error; err != nil {
				return err
			}
			if len(expired) == 0 {
				return nil
			}
			ids := make([]int, len(expired))
			for index := range expired {
				ids[index] = expired[index].Id
			}
			result := tx.Model(&UserSubscription{}).Where("id IN ? AND status = ?", ids, "active").Updates(map[string]any{"status": "expired", "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			count = int(result.RowsAffected)
			var transition UserSubscription
			result = tx.Where("user_id = ? AND end_time <= ? AND status IN ? AND (downgrade_group <> '' OR upgrade_group <> '')", owner, now, []string{"active", "expired"}).Order("end_time DESC, id DESC").Limit(1).Find(&transition)
			if result.Error != nil || result.RowsAffected == 0 {
				return result.Error
			}
			target, err := downgradeUserGroupForSubscriptionTx(tx, &transition, now)
			changed = target != ""
			return err
		})
		if err != nil {
			return total, err
		}
		total += count
		if changed {
			refreshSubscriptionUserGroupCache(owner, "scheduled expiration")
		}
	}
	return total, nil
}

func ResetDueSubscriptions(limit int) (int, error) {
	return ResetDueSubscriptionsContext(context.Background(), limit)
}

func ResetDueSubscriptionsContext(ctx context.Context, limit int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	db := DB.WithContext(ctx)
	now := GetDBTimestampContext(ctx)
	var candidates []int
	if err := db.Model(&UserSubscription{}).Where("status = ? AND start_time <= ? AND end_time > ? AND next_reset_time > 0 AND next_reset_time <= ?", "active", now, now, now).Order("next_reset_time, id").Limit(subscriptionWorkerLimit(limit)).Pluck("id", &candidates).Error; err != nil {
		return 0, err
	}
	count := 0
	for _, id := range candidates {
		reset := false
		err := db.Transaction(func(tx *gorm.DB) error {
			var subscription UserSubscription
			if err := lockForUpdate(tx).Where("id = ? AND status = ? AND next_reset_time > 0 AND next_reset_time <= ?", id, "active", now).First(&subscription).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}
			plan, err := getSubscriptionPlanByIdTx(tx, subscription.PlanId)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			previous := subscription.LastResetTime
			if err := maybeResetUserSubscriptionWithPlanTx(tx, &subscription, plan, now); err != nil {
				return err
			}
			reset = subscription.LastResetTime != previous
			return nil
		})
		if err != nil {
			return count, err
		}
		if reset {
			count++
		}
	}
	return count, nil
}

func CleanupSubscriptionPreConsumeRecords(age int64) (int64, error) {
	return CleanupSubscriptionPreConsumeRecordsContext(context.Background(), age)
}

func CleanupSubscriptionPreConsumeRecordsContext(ctx context.Context, age int64) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if age <= 0 {
		age = 7 * 86400
	}
	cutoff := GetDBTimestampContext(ctx) - age
	db := DB.WithContext(ctx)
	query := db.Where("updated_at < ?", cutoff)
	// Older ordinary-plan writers kept the consumed state after settlement.
	// Apply their configured retention too, while an active discount budget
	// reservation still retains its matching idempotency record.
	if db.Migrator().HasTable(&FurtherDiscountReservation{}) {
		live := db.Model(&FurtherDiscountReservation{}).Select("request_id").Where("status = ?", "reserved")
		query = query.Where("request_id NOT IN (?)", live)
	}
	result := query.Delete(&SubscriptionPreConsumeRecord{})
	return result.RowsAffected, result.Error
}
