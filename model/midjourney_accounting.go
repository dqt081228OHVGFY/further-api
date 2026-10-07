package model

import (
	"context"
	"errors"

	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Reversing a legacy task uses its persisted stage markers. A quota CAS and
// every corresponding balance/usage change commit in the same transaction.
func RefundMidjourneyAccountingContext(ctx context.Context, task *Midjourney) (int, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if task == nil || task.Id <= 0 {
		return 0, false, errors.New("Midjourney 退款任务未持久化。")
	}
	quota, applied, tokenKey := 0, false, ""
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current Midjourney
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, task.Id).Error; err != nil {
			return err
		}
		if current.Quota == 0 {
			return nil
		}
		if current.Quota < 0 || current.Quota > common.MaxQuota {
			return errors.New("Midjourney 持久化退款金额无效。")
		}
		quota = current.Quota
		marker := tx.Model(&Midjourney{}).Where("id = ? AND quota = ?", current.Id, quota).Update("quota", 0)
		if marker.Error != nil {
			return marker.Error
		}
		if marker.RowsAffected != 1 {
			return ErrTaskAccountingConflict
		}
		owner := tx.Model(&User{}).Where("id = ? AND quota <= ?", current.UserId, common.MaxWalletQuota-quota).
			Updates(map[string]any{"quota": gorm.Expr("quota + ?", quota), "used_quota": gorm.Expr("used_quota - ?", quota)})
		if owner.Error != nil {
			return owner.Error
		}
		if owner.RowsAffected != 1 {
			return errors.New("Midjourney 退款账户不可用或额度已达上限。")
		}
		if current.TokenId > 0 {
			var token Token
			read := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&token, current.TokenId).Error
			if read != nil && !errors.Is(read, gorm.ErrRecordNotFound) {
				return read
			}
			if read == nil {
				tokenKey = token.Key
				if err := tx.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]any{
					"remain_quota": gorm.Expr("remain_quota + ?", quota), "used_quota": gorm.Expr("used_quota - ?", quota),
				}).Error; err != nil {
					return err
				}
			}
		}
		if channelID := current.GetBillingChannelId(); channelID > 0 {
			if err := tx.Model(&Channel{}).Where("id = ?", channelID).Update("used_quota", gorm.Expr("used_quota - ?", quota)).Error; err != nil {
				return err
			}
		}
		applied = true
		return nil
	})
	if err != nil {
		return 0, false, err
	}
	task.Quota = 0
	if applied {
		if err := invalidateUserCache(task.UserId); err != nil {
			common.SysError("Midjourney 退款账户缓存失效失败：" + err.Error())
		}
		if tokenKey != "" {
			if err := invalidateTokenCacheForMutation(tokenKey); err != nil {
				common.SysError("Midjourney 退款令牌缓存失效失败：" + err.Error())
			}
		}
	}
	return quota, applied, nil
}
