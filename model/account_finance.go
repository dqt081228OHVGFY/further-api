package model

import (
	"context"
	"errors"
	"math"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"gorm.io/gorm"
)

func accountAccountingSnapshot(ctx context.Context, id int) (*User, error) {
	if err := ctx.Err(); err != nil {
		return &User{}, err
	}
	if id <= 0 {
		return &User{}, gorm.ErrRecordNotFound
	}
	var row User
	err := DB.WithContext(ctx).Select("id", "username", "email", "group", "setting", "role", "quota", "used_quota", "request_count", "aff_quota", "aff_history").First(&row, id).Error
	return &row, err
}

func GetUserQuota(id int, fromDB bool) (int, error) {
	return GetUserQuotaContext(context.Background(), id, fromDB)
}
func GetUserUsedQuota(id int) (int, error) {
	row, err := accountAccountingSnapshot(context.Background(), id)
	return row.UsedQuota, err
}
func GetUserEmail(id int) (string, error) {
	row, err := accountAccountingSnapshot(context.Background(), id)
	return row.Email, err
}

func GetUserGroup(id int, fromDB bool) (string, error) {
	if !fromDB && common.RedisEnabled {
		row, err := GetUserCacheContext(context.Background(), id)
		if err != nil {
			return "", err
		}
		return row.Group, nil
	}
	row, err := accountAccountingSnapshot(context.Background(), id)
	return row.Group, err
}

func GetUserSetting(id int, fromDB bool) (dto.UserSetting, error) {
	if !fromDB && common.RedisEnabled {
		row, err := GetUserCacheContext(context.Background(), id)
		if err != nil {
			return dto.UserSetting{}, err
		}
		return row.GetSetting(), nil
	}
	row, err := accountAccountingSnapshot(context.Background(), id)
	if err != nil {
		return dto.UserSetting{}, err
	}
	return row.GetSetting(), nil
}

func GetUsernameById(id int, fromDB bool) (string, error) {
	if !fromDB && common.RedisEnabled {
		row, err := GetUserCacheContext(context.Background(), id)
		if err != nil {
			return "", err
		}
		return row.Username, nil
	}
	row, err := accountAccountingSnapshot(context.Background(), id)
	return row.Username, err
}

func IncreaseUserQuota(id, quota int, syncDB bool) error {
	return IncreaseUserQuotaContext(context.Background(), id, quota, syncDB)
}
func DecreaseUserQuota(id, quota int, syncDB bool) error {
	return DecreaseUserQuotaContext(context.Background(), id, quota, syncDB)
}

func increaseUserQuota(id, quota int) error {
	if quota < 0 {
		return errors.New("入账额度不能为负数")
	}
	result := DB.Model(&User{}).Where("id = ? AND quota <= ?", id, common.MaxWalletQuota-quota).Update("quota", gorm.Expr("quota + ?", quota))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var count int64
	if err := DB.Model(&User{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return ErrWalletQuotaLimitExceeded
}

func decreaseUserQuota(id, quota int) error {
	if quota < 0 {
		return errors.New("扣减额度不能为负数")
	}
	return applyLedgerDeltaContext(context.Background(), ledgerReservationInput{id: id}, -quota, false)
}

func DeltaUpdateUserQuota(id, delta int) error {
	if delta >= 0 {
		return IncreaseUserQuota(id, delta, false)
	}
	if delta == math.MinInt {
		return ErrWalletQuotaLimitExceeded
	}
	return DecreaseUserQuota(id, -delta, false)
}

type AccountUsageDelta struct {
	Wallet   int
	Used     int
	Requests int
}

func ApplyAccountUsageDeltaContext(ctx context.Context, id int, delta AccountUsageDelta) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if id <= 0 {
		return gorm.ErrRecordNotFound
	}
	if delta.Wallet == 0 && delta.Used == 0 && delta.Requests == 0 {
		return nil
	}
	return DB.WithContext(ctx).Model(&User{}).Where("id = ?", id).Updates(map[string]any{"quota": gorm.Expr("quota + ?", delta.Wallet), "used_quota": gorm.Expr("used_quota + ?", delta.Used), "request_count": gorm.Expr("request_count + ?", delta.Requests)}).Error
}

func UpdateUserUsedQuotaAndRequestCount(id, quota int) {
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUsedQuota, id, quota)
		addNewRecord(BatchUpdateTypeRequestCount, id, 1)
		return
	}
	updateUserUsedQuotaAndRequestCount(id, quota, 1)
}

func UpdateUserUsedQuota(id, quota int) {
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUsedQuota, id, quota)
		return
	}
	updateUserUsedQuotaAndRequestCount(id, quota, 0)
}

func updateUserUsedQuotaAndRequestCount(id, quota, count int) {
	if err := ApplyAccountUsageDeltaContext(context.Background(), id, AccountUsageDelta{Used: quota, Requests: count}); err != nil {
		common.SysError("failed to publish account usage: " + err.Error())
	}
}

func updateUserQuotaUsedQuotaAndRequestCount(id, quota, used, requests int) {
	if err := ApplyAccountUsageDeltaContext(context.Background(), id, AccountUsageDelta{Wallet: quota, Used: used, Requests: requests}); err != nil {
		common.SysError("failed to flush account usage: " + err.Error())
	}
}

func inviteUser(id int) error {
	if id <= 0 || common.QuotaForInviter < 0 || common.QuotaForInviter > common.MaxWalletQuota {
		return ErrWalletQuotaLimitExceeded
	}
	result := DB.Model(&User{}).Where("id = ? AND aff_quota <= ? AND aff_history <= ?", id, common.MaxWalletQuota-common.QuotaForInviter, common.MaxWalletQuota-common.QuotaForInviter).Updates(map[string]any{"aff_count": gorm.Expr("aff_count + 1"), "aff_quota": gorm.Expr("aff_quota + ?", common.QuotaForInviter), "aff_history": gorm.Expr("aff_history + ?", common.QuotaForInviter)})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func GetRootUser() *User {
	var row User
	_ = DB.Where("role = ?", common.RoleRootUser).Order("id").First(&row).Error
	return &row
}

func RootUserExists() bool {
	var count int64
	return DB.Model(&User{}).Where("role = ?", common.RoleRootUser).Count(&count).Error == nil && count > 0
}

func UpdateUserLastLoginAt(id int) {
	if id > 0 {
		_ = DB.Model(&User{}).Where("id = ?", id).Update("last_login_at", common.GetTimestamp()).Error
	}
}
