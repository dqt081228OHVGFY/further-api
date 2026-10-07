package model

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
)

type subscriptionCacheIdentity struct {
	database *gorm.DB
	planID   int
}

type subscriptionCachedPlan struct {
	plan     SubscriptionPlan
	deadline time.Time
}

var subscriptionCatalogMemory = struct {
	sync.RWMutex
	epoch uint64
	plans map[subscriptionCacheIdentity]subscriptionCachedPlan
}{plans: make(map[subscriptionCacheIdentity]subscriptionCachedPlan)}

func cloneSubscriptionPlan(plan SubscriptionPlan) SubscriptionPlan {
	if plan.AllowBalancePay != nil {
		plan.AllowBalancePay = common.GetPointer(*plan.AllowBalancePay)
	}
	if plan.AllowWalletOverflow != nil {
		plan.AllowWalletOverflow = common.GetPointer(*plan.AllowWalletOverflow)
	}
	return plan
}

func InvalidateSubscriptionPlanCache(planID int) {
	if planID <= 0 {
		return
	}
	subscriptionCatalogMemory.Lock()
	subscriptionCatalogMemory.epoch++
	for identity := range subscriptionCatalogMemory.plans {
		if identity.planID == planID {
			delete(subscriptionCatalogMemory.plans, identity)
		}
	}
	subscriptionCatalogMemory.Unlock()
}

func GetSubscriptionPlanById(id int) (*SubscriptionPlan, error) {
	return getSubscriptionPlanByIdTx(nil, id)
}

func getSubscriptionPlanByIdTx(tx *gorm.DB, id int) (*SubscriptionPlan, error) {
	if id <= 0 {
		return nil, errors.New("套餐编号无效")
	}
	if tx != nil {
		var plan SubscriptionPlan
		if err := tx.First(&plan, id).Error; err != nil {
			return nil, err
		}
		plan.NormalizeDefaults()
		return &plan, nil
	}
	identity, now := subscriptionCacheIdentity{database: DB, planID: id}, time.Now()
	subscriptionCatalogMemory.RLock()
	entry, exists := subscriptionCatalogMemory.plans[identity]
	epoch := subscriptionCatalogMemory.epoch
	subscriptionCatalogMemory.RUnlock()
	if exists && now.Before(entry.deadline) {
		plan := cloneSubscriptionPlan(entry.plan)
		return &plan, nil
	}
	var plan SubscriptionPlan
	if err := DB.First(&plan, id).Error; err != nil {
		return nil, err
	}
	plan.NormalizeDefaults()
	ttl := max(1, common.GetEnvOrDefault("SUBSCRIPTION_PLAN_CACHE_TTL", 300))
	capacity := max(1, common.GetEnvOrDefault("SUBSCRIPTION_PLAN_CACHE_CAP", 5000))
	subscriptionCatalogMemory.Lock()
	if epoch == subscriptionCatalogMemory.epoch {
		if len(subscriptionCatalogMemory.plans) >= capacity {
			var oldestIdentity subscriptionCacheIdentity
			var oldest time.Time
			for key, cached := range subscriptionCatalogMemory.plans {
				if oldest.IsZero() || cached.deadline.Before(oldest) {
					oldestIdentity, oldest = key, cached.deadline
				}
			}
			delete(subscriptionCatalogMemory.plans, oldestIdentity)
		}
		subscriptionCatalogMemory.plans[identity] = subscriptionCachedPlan{plan: cloneSubscriptionPlan(plan), deadline: now.Add(time.Duration(ttl) * time.Second)}
	}
	subscriptionCatalogMemory.Unlock()
	return &plan, nil
}

func CountUserSubscriptionsByPlan(userID, planID int) (int64, error) {
	if userID <= 0 || planID <= 0 {
		return 0, errors.New("订阅用户与套餐编号无效")
	}
	var count int64
	err := DB.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ?", userID, planID).Count(&count).Error
	return count, err
}

func GetSubscriptionPlanInfoByUserSubscriptionId(id int) (*SubscriptionPlanInfo, error) {
	if id <= 0 {
		return nil, errors.New("订阅编号无效")
	}
	var info SubscriptionPlanInfo
	result := DB.Table("user_subscriptions AS subscription").Select("subscription.plan_id AS plan_id, plan.title AS plan_title").Joins("JOIN subscription_plans AS plan ON plan.id = subscription.plan_id").Where("subscription.id = ?", id).Limit(1).Scan(&info)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &info, nil
}

func refreshSubscriptionUserGroupCache(userID int, operation string) {
	if err := RefreshUserGroupCache(userID); err != nil {
		common.SysError(fmt.Sprintf("failed to publish subscription group for user %d after %s: %v", userID, operation, err))
	}
}
