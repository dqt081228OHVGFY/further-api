package model

import (
	"errors"
	"strconv"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"

	"gorm.io/gorm"
)

type Redemption struct {
	Id           int            `json:"id"`
	UserId       int            `json:"user_id"`
	Key          string         `json:"key" gorm:"type:char(32);uniqueIndex"`
	Status       int            `json:"status" gorm:"default:1"`
	Name         string         `json:"name" gorm:"index"`
	BatchID      string         `json:"batch_id" gorm:"size:32;index;default:''"`
	Quota        int            `json:"quota" gorm:"default:100"`
	CreatedTime  int64          `json:"created_time" gorm:"bigint"`
	RedeemedTime int64          `json:"redeemed_time" gorm:"bigint"`
	Count        int            `json:"count" gorm:"-:all"` // only for api request
	UsedUserId   int            `json:"used_user_id"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
	ExpiredTime  int64          `json:"expired_time" gorm:"bigint"` // 过期时间，0 表示不过期
}

func GetAllRedemptions(startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	// 开始事务
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 获取总数
	err = tx.Model(&Redemption{}).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 获取分页数据
	err = tx.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 提交事务
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func SearchRedemptions(keyword string, status string, startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&Redemption{})

	if keyword != "" {
		if id, err := strconv.Atoi(keyword); err == nil {
			query = query.Where("id = ? OR name LIKE ?", id, keyword+"%")
		} else {
			query = query.Where("name LIKE ?", keyword+"%")
		}
	}

	if status != "" {
		now := common.GetTimestamp()
		switch status {
		case "expired":
			query = query.Where(
				"status = ? AND expired_time != 0 AND expired_time < ?",
				common.RedemptionCodeStatusEnabled,
				now,
			)
		case strconv.Itoa(common.RedemptionCodeStatusEnabled):
			query = query.Where(
				"status = ? AND (expired_time = 0 OR expired_time >= ?)",
				common.RedemptionCodeStatusEnabled,
				now,
			)
		case strconv.Itoa(common.RedemptionCodeStatusDisabled):
			query = query.Where("status = ?", common.RedemptionCodeStatusDisabled)
		case strconv.Itoa(common.RedemptionCodeStatusUsed):
			query = query.Where("status = ?", common.RedemptionCodeStatusUsed)
		}
	}

	// Get total count
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated data
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func GetRedemptionById(id int) (*Redemption, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	var err error = nil
	err = DB.First(&redemption, "id = ?", id).Error
	return &redemption, err
}

func (redemption *Redemption) Insert() error {
	if redemption.Quota <= 0 {
		return errors.New("redemption quota must be positive")
	}
	if err := common.ValidateWalletQuota(redemption.Quota); err != nil {
		return err
	}
	var err error
	err = DB.Create(redemption).Error
	return err
}

func (redemption *Redemption) SelectUpdate() error {
	// This can update zero values
	return DB.Model(redemption).Select("redeemed_time", "status").Updates(redemption).Error
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (redemption *Redemption) Update() error {
	if redemption.Quota <= 0 {
		return errors.New("redemption quota must be positive")
	}
	if err := common.ValidateWalletQuota(redemption.Quota); err != nil {
		return err
	}
	var err error
	err = DB.Model(redemption).Select("name", "status", "quota", "redeemed_time", "expired_time").Updates(redemption).Error
	return err
}

func (redemption *Redemption) Delete() error {
	var err error
	err = DB.Delete(redemption).Error
	return err
}

func DeleteRedemptionById(id int) (err error) {
	if id == 0 {
		return errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	err = DB.Where(redemption).First(&redemption).Error
	if err != nil {
		return err
	}
	return redemption.Delete()
}

func DeleteInvalidRedemptions() (int64, error) {
	now := common.GetTimestamp()
	result := DB.Where("status IN ? OR (status = ? AND expired_time != 0 AND expired_time < ?)", []int{common.RedemptionCodeStatusUsed, common.RedemptionCodeStatusDisabled}, common.RedemptionCodeStatusEnabled, now).Delete(&Redemption{})
	return result.RowsAffected, result.Error
}

// BatchDeleteRedemptions soft-deletes the selected codes in one statement.
func BatchDeleteRedemptions(ids []int) (int64, error) {
	if len(ids) == 0 || len(ids) > 1000 {
		return 0, errors.New("select between 1 and 1000 redemption codes")
	}
	for _, id := range ids {
		if id <= 0 {
			return 0, errors.New("redemption IDs must be positive")
		}
	}
	result := DB.Where("id IN ?", ids).Delete(&Redemption{})
	return result.RowsAffected, result.Error
}

type RedemptionBatch struct {
	BatchID string        `json:"batch_id"`
	Items   []*Redemption `json:"items"`
}

func GetRedemptionBatches(keyword, status string, startIdx, pageSize int) ([]RedemptionBatch, int64, error) {
	var batches []RedemptionBatch
	var total int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&Redemption{})
		if keyword != "" {
			pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(keyword) + "%"
			keyCol := "`key`"
			if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
				keyCol = "\"key\""
			}
			query = query.Where("(name LIKE ? ESCAPE '!' OR "+keyCol+" LIKE ? ESCAPE '!' OR batch_id = ?)", pattern, pattern, keyword)
		}
		now := common.GetTimestamp()
		switch status {
		case "1":
			query = query.Where("status = ? AND (expired_time = 0 OR expired_time >= ?)", common.RedemptionCodeStatusEnabled, now)
		case "2":
			query = query.Where("status = ?", common.RedemptionCodeStatusDisabled)
		case "3":
			query = query.Where("status = ?", common.RedemptionCodeStatusUsed)
		case "expired":
			query = query.Where("status = ? AND expired_time > 0 AND expired_time < ?", common.RedemptionCodeStatusEnabled, now)
		}
		grouped := query.Select("batch_id, MIN(id) AS legacy_id, MAX(id) AS latest_id").
			Group("batch_id, CASE WHEN batch_id IS NULL OR batch_id = '' THEN id ELSE 0 END")
		if err := tx.Table("(?) AS redemption_batches", grouped).Count(&total).Error; err != nil {
			return err
		}
		var selected []struct {
			BatchID  string
			LegacyID int
			LatestID int
		}
		if err := grouped.Order("latest_id DESC").Offset(startIdx).Limit(pageSize).Scan(&selected).Error; err != nil {
			return err
		}
		batches = make([]RedemptionBatch, len(selected))
		if len(selected) == 0 {
			return nil
		}
		batchIDs, legacyIDs := []string{}, []int{}
		indexes := map[string]int{}
		for index, row := range selected {
			key := row.BatchID
			if key == "" {
				key = "legacy-" + strconv.Itoa(row.LegacyID)
				legacyIDs = append(legacyIDs, row.LegacyID)
			} else {
				batchIDs = append(batchIDs, key)
			}
			batches[index] = RedemptionBatch{BatchID: key, Items: []*Redemption{}}
			indexes[key] = index
		}
		var codes []*Redemption
		if err := tx.Where("batch_id IN ? OR ((batch_id IS NULL OR batch_id = '') AND id IN ?)", batchIDs, legacyIDs).Order("id DESC").Find(&codes).Error; err != nil {
			return err
		}
		for _, code := range codes {
			key := code.BatchID
			if key == "" {
				key = "legacy-" + strconv.Itoa(code.Id)
			}
			batches[indexes[key]].Items = append(batches[indexes[key]].Items, code)
		}
		return nil
	})
	return batches, total, err
}

func DeleteUnusedRedemptionBatch(batchID string) (int64, error) {
	query := DB.Model(&Redemption{}).Where("status <> ?", common.RedemptionCodeStatusUsed)
	suffix, legacy := strings.CutPrefix(batchID, "legacy-")
	if !legacy {
		suffix, legacy = strings.CutPrefix(batchID, "further-")
	}
	if legacy {
		id, err := strconv.Atoi(suffix)
		if err != nil || id <= 0 {
			return 0, errors.New("invalid batch ID")
		}
		query = query.Where("(batch_id IS NULL OR batch_id = '') AND id = ?", id)
	} else {
		if len(batchID) != 32 {
			return 0, errors.New("invalid batch ID")
		}
		query = query.Where("batch_id = ?", batchID)
	}
	result := query.Delete(&Redemption{})
	return result.RowsAffected, result.Error
}

func DeleteUsedRedemptions() (int64, error) {
	result := DB.Where("status = ?", common.RedemptionCodeStatusUsed).Delete(&Redemption{})
	return result.RowsAffected, result.Error
}
