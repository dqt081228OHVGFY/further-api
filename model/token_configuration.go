package model

import (
	"context"
	"errors"
	"math/big"

	"gorm.io/gorm"
)

// UpdateTokenConfiguration treats the submitted allowance as an edit relative
// to the controller's baseline. Usage committed while the request is being
// validated is preserved, as are current status and immutable key ownership.
func UpdateTokenConfiguration(ctx context.Context, next *Token, baselineQuota int) error {
	return persistTokenConfiguration(ctx, next, &baselineQuota, false)
}

func persistTokenConfiguration(ctx context.Context, next *Token, baselineQuota *int, includeStatus bool) error {
	if next == nil || next.Id <= 0 || next.UserId <= 0 {
		return gorm.ErrRecordNotFound
	}
	var stored Token
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", next.Id, next.UserId).First(&stored).Error; err != nil {
			return err
		}
		quota := next.RemainQuota
		if baselineQuota != nil {
			value := new(big.Int).SetInt64(int64(stored.RemainQuota))
			value.Add(value, big.NewInt(int64(next.RemainQuota)))
			value.Sub(value, big.NewInt(int64(*baselineQuota)))
			if !value.IsInt64() {
				return errors.New("令牌额度超出可用范围")
			}
			quota = int(value.Int64())
		}
		if err := invalidateTokenCacheForMutation(stored.Key); err != nil {
			return err
		}
		updates := map[string]any{
			"name": next.Name, "expired_time": next.ExpiredTime, "remain_quota": quota,
			"unlimited_quota": next.UnlimitedQuota, "model_limits_enabled": next.ModelLimitsEnabled,
			"model_limits": next.ModelLimits, "allow_ips": next.AllowIps, "group": next.Group,
			"cross_group_retry": next.CrossGroupRetry, "auto_groups": next.AutoGroups,
		}
		if includeStatus {
			updates["status"] = next.Status
		}
		result := tx.Model(&Token{}).Where("id = ? AND user_id = ?", stored.Id, stored.UserId).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		return tx.First(&stored, stored.Id).Error
	})
	if err == nil {
		*next = stored
	}
	return err
}

func (token *Token) Update() error {
	return persistTokenConfiguration(context.Background(), token, nil, true)
}

func (token *Token) SelectUpdate() error {
	if token == nil || token.Id <= 0 || token.UserId <= 0 {
		return gorm.ErrRecordNotFound
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var current Token
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", token.Id, token.UserId).First(&current).Error; err != nil {
			return err
		}
		if err := invalidateTokenCacheForMutation(current.Key); err != nil {
			return err
		}
		return tx.Model(&current).Updates(map[string]any{"accessed_time": token.AccessedTime, "status": token.Status}).Error
	})
}

func (token *Token) Delete() error {
	if token == nil || token.Id <= 0 || token.UserId <= 0 {
		return gorm.ErrRecordNotFound
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var current Token
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", token.Id, token.UserId).First(&current).Error; err != nil {
			return err
		}
		if err := invalidateTokenCacheForMutation(current.Key); err != nil {
			return err
		}
		return tx.Delete(&current).Error
	})
}

func BatchDeleteTokens(ids []int, userID int) (int, error) {
	if userID <= 0 || len(ids) == 0 {
		return 0, errors.New("请选择要删除的令牌")
	}
	unique := make([]int, 0, len(ids))
	seen := make(map[int]bool, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return 0, errors.New("令牌编号无效")
		}
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	deleted := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		var tokens []Token
		if err := lockForUpdate(tx).Where("user_id = ? AND id IN ?", userID, unique).Order("id").Find(&tokens).Error; err != nil {
			return err
		}
		if len(tokens) == 0 {
			return nil
		}
		if err := invalidateTokensCache(tokens); err != nil {
			return err
		}
		result := tx.Where("user_id = ? AND id IN ?", userID, unique).Delete(&Token{})
		deleted = int(result.RowsAffected)
		return result.Error
	})
	return deleted, err
}
