package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
)

func PreConsumeUserSubscriptionContext(ctx context.Context, requestID string, userID int, modelName string, quotaType int, amount int64) (*SubscriptionPreConsumeResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if userID <= 0 || strings.TrimSpace(requestID) == "" || len(requestID) > 64 || amount <= 0 {
		return nil, errors.New("订阅额度预扣参数无效")
	}
	var result *SubscriptionPreConsumeResult
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSubscriptionOwner(tx, userID); err != nil {
			return err
		}
		var existing SubscriptionPreConsumeRecord
		err := tx.Where("request_id = ?", requestID).First(&existing).Error
		if err == nil {
			result, err = replaySubscriptionReservation(tx, &existing, userID)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		order, err := furtherSubscriptionOrderTx(tx, userID)
		if err != nil {
			return err
		}
		now := furtherDBTimestampTx(tx)
		var subscriptions []UserSubscription
		if err := lockForUpdate(tx).Where("user_id = ? AND status = ? AND start_time <= ? AND end_time > ?", userID, "active", now, now).Order(order).Find(&subscriptions).Error; err != nil {
			return err
		}
		if len(subscriptions) == 0 {
			return ErrNoActiveSubscription
		}
		for index := range subscriptions {
			subscription := &subscriptions[index]
			plan, err := getSubscriptionPlanByIdTx(tx, subscription.PlanId)
			if err != nil {
				return err
			}
			if err := maybeResetUserSubscriptionWithPlanTx(tx, subscription, plan, now); err != nil {
				return err
			}
			policy, eligible, err := furtherDiscountPolicyTx(tx, subscription, plan)
			if err != nil {
				return err
			}
			if !eligible {
				continue
			}
			charged := amount
			if policy != nil {
				charged, _ = furtherDiscountAmounts(amount, policy.ModelMultiplier(modelName), policy, 0)
			}
			if subscription.AmountTotal > 0 && subscription.AmountTotal-subscription.AmountUsed < charged {
				continue
			}
			after, err := checkedSubscriptionUsage(subscription.AmountUsed, charged)
			if err != nil {
				return err
			}
			record := SubscriptionPreConsumeRecord{RequestId: requestID, UserId: userID, UserSubscriptionId: subscription.Id, PreConsumed: charged, Status: "consumed"}
			inserted := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "request_id"}}, DoNothing: true}).Create(&record)
			if inserted.Error != nil {
				return inserted.Error
			}
			if inserted.RowsAffected == 0 {
				if err := tx.Where("request_id = ?", requestID).First(&existing).Error; err != nil {
					return err
				}
				result, err = replaySubscriptionReservation(tx, &existing, userID)
				return err
			}
			if err := reserveFurtherDiscountTx(tx, requestID, subscription, policy, modelName, amount, charged); err != nil {
				return err
			}
			if err := tx.Model(&UserSubscription{}).Where("id = ?", subscription.Id).Updates(map[string]any{"amount_used": after, "updated_at": now}).Error; err != nil {
				return err
			}
			result = &SubscriptionPreConsumeResult{UserSubscriptionId: subscription.Id, PreConsumed: charged, AmountTotal: subscription.AmountTotal, AmountUsedBefore: subscription.AmountUsed, AmountUsedAfter: after}
			return nil
		}
		return fmt.Errorf("%w, need=%d", ErrSubscriptionQuota, amount)
	})
	return result, err
}

func RefundSubscriptionPreConsumeContext(ctx context.Context, requestID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if strings.TrimSpace(requestID) == "" {
		return errors.New("订阅请求编号无效")
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record SubscriptionPreConsumeRecord
		if err := tx.Where("request_id = ?", requestID).First(&record).Error; err != nil {
			return err
		}
		if err := lockSubscriptionOwner(tx, record.UserId); err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&record).Error; err != nil {
			return err
		}
		if record.Status == "refunded" || record.Status == "settled" {
			return nil
		}
		if err := refundFurtherDiscountTx(tx, requestID); err != nil {
			return err
		}
		if record.PreConsumed > 0 {
			var subscription UserSubscription
			if err := lockForUpdate(tx).First(&subscription, record.UserSubscriptionId).Error; err != nil {
				return err
			}
			after, err := checkedSubscriptionUsage(subscription.AmountUsed, -record.PreConsumed)
			if err != nil {
				return err
			}
			if err := tx.Model(&subscription).Updates(map[string]any{"amount_used": after, "updated_at": common.GetTimestamp()}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&record).Updates(map[string]any{"status": "refunded", "updated_at": common.GetTimestamp()}).Error
	})
}

func PostConsumeUserSubscriptionDeltaContext(ctx context.Context, id int, delta int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if id <= 0 {
		return errors.New("订阅编号无效")
	}
	if delta == 0 {
		return nil
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var subscription UserSubscription
		if err := lockForUpdate(tx).First(&subscription, id).Error; err != nil {
			return err
		}
		after, err := checkedSubscriptionUsage(subscription.AmountUsed, delta)
		if err != nil {
			return err
		}
		if subscription.AmountTotal > 0 && after > subscription.AmountTotal {
			return fmt.Errorf("subscription used exceeds total, used=%d total=%d", after, subscription.AmountTotal)
		}
		return tx.Model(&subscription).Updates(map[string]any{"amount_used": after, "updated_at": common.GetTimestamp()}).Error
	})
}

func GetSubscriptionPlanInfoByUserSubscriptionIdContext(ctx context.Context, id int) (*SubscriptionPlanInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if id <= 0 {
		return nil, errors.New("订阅编号无效")
	}
	var info SubscriptionPlanInfo
	result := DB.WithContext(ctx).Table("user_subscriptions AS subscription").Select("subscription.plan_id AS plan_id, plan.title AS plan_title").Joins("JOIN subscription_plans AS plan ON plan.id = subscription.plan_id").Where("subscription.id = ?", id).Limit(1).Scan(&info)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &info, nil
}

func HasActiveUserSubscriptionContext(ctx context.Context, userID int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if userID <= 0 {
		return false, errors.New("用户编号无效")
	}
	var subscriptionID int
	now := common.GetTimestamp()
	err := DB.WithContext(ctx).Model(&UserSubscription{}).Select("id").Where("user_id = ? AND status = ? AND start_time <= ? AND end_time > ?", userID, "active", now, now).Limit(1).Scan(&subscriptionID).Error
	return subscriptionID > 0, err
}

func UserActiveSubscriptionsAllowWalletOverflowContext(ctx context.Context, userID int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if userID <= 0 {
		return false, errors.New("用户编号无效")
	}
	var blockedID int
	now := common.GetTimestamp()
	err := DB.WithContext(ctx).Model(&UserSubscription{}).Select("id").Where("user_id = ? AND status = ? AND start_time <= ? AND end_time > ? AND allow_wallet_overflow = ?", userID, "active", now, now, false).Limit(1).Scan(&blockedID).Error
	return blockedID == 0, err
}

func GetFurtherDiscountReservationContext(ctx context.Context, requestID string) (*FurtherDiscountReservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if !DB.WithContext(ctx).Migrator().HasTable(&FurtherDiscountReservation{}) {
		return nil, nil
	}
	var record FurtherDiscountReservation
	err := DB.WithContext(ctx).First(&record, "request_id = ?", requestID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &record, err
}

func GetFurtherActiveEntitlementsContext(ctx context.Context, userID int, modelName string) ([]FurtherEntitlement, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var subs []UserSubscription
	now := common.GetTimestamp()
	if err := DB.WithContext(ctx).Where("user_id = ? AND status = ? AND start_time <= ? AND end_time > ?", userID, "active", now, now).Order("end_time ASC, id ASC").Find(&subs).Error; err != nil {
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
	var snapshots []FurtherUserSubscriptionPolicy
	var policies []FurtherSubscriptionPolicy
	if len(ids) > 0 {
		if err := DB.WithContext(ctx).Where("subscription_id IN ?", ids).Find(&snapshots).Error; err != nil {
			return nil, err
		}
		if err := DB.WithContext(ctx).Where("plan_id IN ?", plans).Find(&policies).Error; err != nil {
			return nil, err
		}
	}
	bySub := map[int]FurtherUserSubscriptionPolicy{}
	byPlan := map[int]FurtherSubscriptionPolicy{}
	for _, item := range snapshots {
		bySub[item.SubscriptionID] = item
	}
	for _, item := range policies {
		byPlan[item.PlanID] = item
	}
	result := []FurtherEntitlement{}
	for _, sub := range subs {
		policy, ok := bySub[sub.Id]
		if !ok {
			if template, exists := byPlan[sub.PlanId]; exists {
				policy = furtherPolicySnapshot(template, sub.Id)
			} else {
				policy = FurtherUserSubscriptionPolicy{SubscriptionID: sub.Id, PlanID: sub.PlanId, DiscountMultiplier: 1}
			}
		}
		if policy.UpgradeFromPlanID > 0 && !activePlans[policy.UpgradeFromPlanID] {
			continue
		}
		result = append(result, FurtherEntitlement{Subscription: sub, Policy: policy})
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
