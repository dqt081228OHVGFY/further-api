package model

import (
	"errors"
	"fmt"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"gorm.io/gorm"
)

// Referral conversion transfers two balance columns together and publishes
// only the committed wallet delta, preserving concurrent relay reservations.
func (user *User) TransferAffQuotaToQuota(amount int) error {
	if user == nil || user.Id <= 0 {
		return gorm.ErrRecordNotFound
	}
	if amount <= 0 || float64(amount) < common.QuotaPerUnit {
		return fmt.Errorf("转移额度最小为 %s", logger.LogQuota(common.QuotaFromFloat(common.QuotaPerUnit)))
	}
	if amount > common.MaxWalletQuota {
		return ErrWalletQuotaLimitExceeded
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var current User
		if err := lockForUpdate(tx).First(&current, user.Id).Error; err != nil {
			return err
		}
		if current.AffQuota < amount {
			return errors.New("邀请额度不足")
		}
		if current.Quota > common.MaxWalletQuota-amount {
			return ErrWalletQuotaLimitExceeded
		}
		if err := tx.Model(&current).Updates(map[string]any{"aff_quota": gorm.Expr("aff_quota - ?", amount), "quota": gorm.Expr("quota + ?", amount)}).Error; err != nil {
			return err
		}
		return tx.First(user, current.Id).Error
	})
	if err != nil {
		return err
	}
	if err := cacheIncrUserQuota(user.Id, int64(amount)); err != nil {
		common.SysError("failed to publish referral conversion allowance: " + err.Error())
	}
	return nil
}
