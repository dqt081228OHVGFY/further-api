package model

import (
	"context"
	"errors"
	"hash/fnv"
	"sync"

	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
)

var ErrTaskAccountingConflict = errors.New("任务计费状态已被其他处理器更新。")
var taskAccountingGuards [128]sync.Mutex

type TaskAccountingResult struct {
	Before  int
	After   int
	Delta   int
	Applied bool
}

// The task quota is the persisted compare-and-swap marker. Funding, allowance,
// accumulated usage and that marker commit together, so a retry cannot refund
// funds again after a quota write failed. Pre-insert immediate tasks are
// serialized locally and retain their caller-owned quota marker.
func ApplyTaskAccountingContext(ctx context.Context, task *Task, target int) (TaskAccountingResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return TaskAccountingResult{}, err
	}
	if task == nil || task.UserId <= 0 || target < 0 || target > common.MaxQuota {
		return TaskAccountingResult{}, errors.New("任务计费参数无效。")
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(task.TaskID))
	guard := &taskAccountingGuards[hash.Sum32()%uint32(len(taskAccountingGuards))]
	guard.Lock()
	defer guard.Unlock()
	result := TaskAccountingResult{Before: task.Quota, After: target, Delta: target - task.Quota}
	if result.Delta == 0 {
		return result, nil
	}
	var tokenKey string
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current := *task
		if task.ID > 0 {
			if err := lockForUpdate(tx).First(&current, task.ID).Error; err != nil {
				return err
			}
			if current.UserId != task.UserId || current.TaskID != task.TaskID {
				return ErrTaskAccountingConflict
			}
			if current.Quota == target {
				result.Before, result.Delta = target, 0
				return nil
			}
			if current.Quota != task.Quota {
				return ErrTaskAccountingConflict
			}
		}
		delta := int64(result.Delta)
		if current.PrivateData.BillingSource == "subscription" && current.PrivateData.SubscriptionId > 0 {
			var subscription UserSubscription
			if err := lockForUpdate(tx).First(&subscription, current.PrivateData.SubscriptionId).Error; err != nil {
				return err
			}
			if subscription.UserId != current.UserId {
				return errors.New("任务订阅归属无效。")
			}
			after, err := checkedSubscriptionUsage(subscription.AmountUsed, delta)
			if err != nil {
				return err
			}
			if subscription.AmountTotal > 0 && after > subscription.AmountTotal {
				return ErrSubscriptionQuota
			}
			if err := tx.Model(&subscription).Updates(map[string]any{"amount_used": after, "updated_at": common.GetTimestamp()}).Error; err != nil {
				return err
			}
		} else {
			updated := tx.Model(&User{}).Where("id = ?", current.UserId).Update("quota", gorm.Expr("quota - ?", delta))
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
		}
		if current.PrivateData.TokenId > 0 {
			var token Token
			lookup := lockForUpdate(tx).Unscoped().First(&token, current.PrivateData.TokenId)
			if lookup.Error != nil && !errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
				return lookup.Error
			}
			if lookup.Error == nil && !token.DeletedAt.Valid {
				if token.UserId != current.UserId {
					return errors.New("任务令牌归属无效。")
				}
				tokenKey = token.Key
				if err := tx.Unscoped().Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]any{
					"remain_quota": gorm.Expr("remain_quota - ?", delta), "used_quota": gorm.Expr("used_quota + ?", delta),
				}).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Model(&User{}).Where("id = ?", current.UserId).Update("used_quota", gorm.Expr("used_quota + ?", delta)).Error; err != nil {
			return err
		}
		if current.ChannelId > 0 {
			if err := tx.Model(&Channel{}).Where("id = ?", current.ChannelId).Update("used_quota", gorm.Expr("used_quota + ?", delta)).Error; err != nil {
				return err
			}
		}
		if current.ID > 0 {
			updated := tx.Model(&Task{}).Where("id = ? AND quota = ?", current.ID, result.Before).Update("quota", target)
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return ErrTaskAccountingConflict
			}
		}
		result.Applied = true
		return nil
	})
	if err != nil {
		return TaskAccountingResult{}, err
	}
	task.Quota = result.After
	if result.Applied {
		// Invalidate committed projections rather than publishing a snapshot
		// that can overwrite concurrent request reservations.
		if err := invalidateUserCache(task.UserId); err != nil {
			common.SysError("任务计费账户缓存失效失败：" + err.Error())
		}
		if tokenKey != "" {
			if err := invalidateTokenCacheForMutation(tokenKey); err != nil {
				common.SysError("任务计费令牌缓存失效失败：" + err.Error())
			}
		}
	}
	return result, nil
}
