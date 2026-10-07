package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
)

// One database projection serves both account lists and administrator lists.
// Retired plans and deleted prepaid cards keep their historical labels.
func listUserSubscriptionSummaries(db *gorm.DB, userID int, activeOnly bool) ([]SubscriptionSummary, error) {
	if userID <= 0 {
		return nil, errors.New("用户编号无效")
	}
	internal := []string{"user_subscriptions.source = 'further_prepaid'"}
	if db.Migrator().HasTable(&FurtherSubscriptionPolicy{}) {
		internal = append(internal, "EXISTS (SELECT 1 FROM further_subscription_policies policy WHERE policy.plan_id = user_subscriptions.plan_id AND policy.internal = ?)")
	}
	if db.Migrator().HasTable(&FurtherPrepaidCard{}) {
		internal = append(internal, "EXISTS (SELECT 1 FROM further_prepaid_cards card WHERE card.plan_id = user_subscriptions.plan_id)")
	}
	projection := "user_subscriptions.*, COALESCE(subscription_plans.title, '') AS catalog_title, CASE WHEN (" + strings.Join(internal, " OR ") + ") THEN 1 ELSE 0 END AS catalog_internal"
	query := db.Model(&UserSubscription{}).Joins("LEFT JOIN subscription_plans ON subscription_plans.id = user_subscriptions.plan_id").Where("user_subscriptions.user_id = ?", userID)
	if activeOnly {
		now := common.GetTimestamp()
		query = query.Where("user_subscriptions.status = ? AND user_subscriptions.start_time <= ? AND user_subscriptions.end_time > ?", "active", now, now)
	}
	var rows []struct {
		UserSubscription `gorm:"embedded"`
		CatalogTitle     string
		CatalogInternal  bool
	}
	selectArgs := make([]any, 0, 1)
	if db.Migrator().HasTable(&FurtherSubscriptionPolicy{}) {
		selectArgs = append(selectArgs, true)
	}
	if err := query.Select(projection, selectArgs...).Order("user_subscriptions.end_time DESC, user_subscriptions.id DESC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]SubscriptionSummary, len(rows))
	for index := range rows {
		result[index] = SubscriptionSummary{Subscription: &rows[index].UserSubscription, PlanTitle: rows[index].CatalogTitle, PlanInternal: rows[index].CatalogInternal}
	}
	return result, nil
}

func GetAllUserSubscriptions(userID int) ([]SubscriptionSummary, error) {
	return GetAllUserSubscriptionsContext(context.Background(), userID)
}

func GetAllUserSubscriptionsContext(ctx context.Context, userID int) ([]SubscriptionSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	return listUserSubscriptionSummaries(DB.WithContext(ctx), userID, false)
}

func GetAllActiveUserSubscriptions(userID int) ([]SubscriptionSummary, error) {
	return GetAllActiveUserSubscriptionsContext(context.Background(), userID)
}

func GetAllActiveUserSubscriptionsContext(ctx context.Context, userID int) ([]SubscriptionSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	return listUserSubscriptionSummaries(DB.WithContext(ctx), userID, true)
}

func HasActiveUserSubscription(userID int) (bool, error) {
	if userID <= 0 {
		return false, errors.New("用户编号无效")
	}
	var subscriptionID int
	now := common.GetTimestamp()
	err := DB.Model(&UserSubscription{}).Select("id").Where("user_id = ? AND status = ? AND start_time <= ? AND end_time > ?", userID, "active", now, now).Limit(1).Scan(&subscriptionID).Error
	return subscriptionID > 0, err
}

func UserActiveSubscriptionsAllowWalletOverflow(userID int) (bool, error) {
	if userID <= 0 {
		return false, errors.New("用户编号无效")
	}
	var blockedID int
	now := common.GetTimestamp()
	err := DB.Model(&UserSubscription{}).Select("id").Where("user_id = ? AND status = ? AND start_time <= ? AND end_time > ? AND allow_wallet_overflow = ?", userID, "active", now, now, false).Limit(1).Scan(&blockedID).Error
	return blockedID == 0, err
}

func AdminBindSubscription(userID, planID int, sourceNote string) (string, error) {
	return AdminBindSubscriptionContext(context.Background(), userID, planID, sourceNote)
}

func AdminBindSubscriptionContext(ctx context.Context, userID, planID int, sourceNote string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if DB == nil {
		return "", gorm.ErrInvalidDB
	}
	if userID <= 0 || planID <= 0 {
		return "", errors.New("请选择有效的用户与套餐")
	}
	var subscription *UserSubscription
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id", "status").First(&user, userID).Error; err != nil {
			return err
		}
		plan, err := getSubscriptionPlanByIdTx(tx, planID)
		if err != nil {
			return err
		}
		subscription, err = CreateUserSubscriptionFromPlanTx(tx, userID, plan, "admin")
		return err
	})
	if err != nil {
		return "", err
	}
	if subscription.PrevUserGroup == "" {
		return "", nil
	}
	refreshSubscriptionUserGroupCache(userID, "catalog grant")
	return fmt.Sprintf("用户分组将升级到 %s", subscription.UpgradeGroup), nil
}

func mutateSubscriptionLifecycle(ctx context.Context, id int, erase bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if DB == nil {
		return "", gorm.ErrInvalidDB
	}
	if id <= 0 {
		return "", errors.New("订阅编号无效")
	}
	var userID int
	var targetGroup string
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The user lock orders mutations with new grants and group changes.
		var lookup UserSubscription
		if err := tx.Select("id", "user_id").First(&lookup, id).Error; err != nil {
			return err
		}
		userID = lookup.UserId
		var user User
		if err := lockForUpdate(tx).First(&user, userID).Error; err != nil {
			return err
		}
		var subscription UserSubscription
		if err := lockForUpdate(tx).First(&subscription, id).Error; err != nil {
			return err
		}
		now := furtherDBTimestampTx(tx)
		if !erase && subscription.Status == "cancelled" {
			return nil
		}
		if !erase {
			if err := tx.Model(&subscription).Updates(map[string]any{"status": "cancelled", "end_time": now, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		var err error
		targetGroup, err = downgradeUserGroupForSubscriptionTx(tx, &subscription, now)
		if err != nil || !erase {
			return err
		}
		for _, dependent := range []struct {
			row any
			key string
		}{
			{&FurtherUserSubscriptionPolicy{}, "subscription_id"},
			{&FurtherSubscriptionHidden{}, "subscription_id"},
		} {
			if tx.Migrator().HasTable(dependent.row) {
				if err := tx.Where(dependent.key+" = ?", id).Delete(dependent.row).Error; err != nil {
					return err
				}
			}
		}
		return tx.Delete(&subscription).Error
	})
	if err != nil {
		return "", err
	}
	if targetGroup == "" {
		return "", nil
	}
	refreshSubscriptionUserGroupCache(userID, "subscription lifecycle")
	return fmt.Sprintf("用户分组将回退到 %s", targetGroup), nil
}

func AdminInvalidateUserSubscription(id int) (string, error) {
	return AdminInvalidateUserSubscriptionContext(context.Background(), id)
}

func AdminInvalidateUserSubscriptionContext(ctx context.Context, id int) (string, error) {
	return mutateSubscriptionLifecycle(ctx, id, false)
}

func AdminDeleteUserSubscription(id int) (string, error) {
	return AdminDeleteUserSubscriptionContext(context.Background(), id)
}

func AdminDeleteUserSubscriptionContext(ctx context.Context, id int) (string, error) {
	return mutateSubscriptionLifecycle(ctx, id, true)
}
