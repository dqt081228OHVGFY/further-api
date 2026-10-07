package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func RedeemContext(ctx context.Context, key string, userID int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if key == "" || userID <= 0 {
		return 0, ErrRedeemFailed
	}
	var redemption Redemption
	err := DB.WithContext(ctx).Session(&gorm.Session{Logger: DB.Logger.LogMode(gormlogger.Silent)}).Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where(commonKeyCol+" = ?", key).First(&redemption).Error; err != nil {
			return err
		}
		if redemption.Status != common.RedemptionCodeStatusEnabled || redemption.ExpiredTime != 0 && redemption.ExpiredTime <= common.GetTimestamp() {
			return ErrRedeemFailed
		}
		result := tx.Model(&Redemption{}).Where("id = ? AND status = ?", redemption.Id, common.RedemptionCodeStatusEnabled).Updates(map[string]any{"redeemed_time": common.GetTimestamp(), "status": common.RedemptionCodeStatusUsed, "used_user_id": userID})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrRedeemFailed
		}
		return creditTopUpQuota(tx, userID, redemption.Quota, nil)
	})
	if err != nil {
		if ctx.Err() != nil {
			return 0, errors.Join(err, ctx.Err())
		}
		return 0, ErrRedeemFailed
	}
	// A committed credit must still be published if the browser leaves now.
	syncCreditUserQuotaCache(userID, redemption.Quota, "redemption")
	RecordLog(userID, LogTypeTopup, fmt.Sprintf("通过兑换码充值 %s，兑换码ID %d", logger.LogQuota(redemption.Quota), redemption.Id))
	return redemption.Quota, nil
}

func Redeem(key string, userID int) (int, error) {
	return RedeemContext(context.Background(), key, userID)
}
