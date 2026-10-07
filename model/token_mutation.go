package model

import (
	"errors"

	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
)

var (
	ErrTokenLimitReached  = errors.New("token limit reached")
	ErrTokenOwnerDisabled = errors.New("token owner is disabled")
)

// UpdateStatus uses the current quota and expiry when enabling a token, and
// never writes accounting fields from the caller's snapshot.
func (token *Token) UpdateStatus() error {
	if token == nil || token.Id <= 0 || token.UserId <= 0 {
		return gorm.ErrRecordNotFound
	}
	switch token.Status {
	case common.TokenStatusEnabled, common.TokenStatusDisabled, common.TokenStatusExhausted, common.TokenStatusExpired:
	default:
		return ErrTokenInvalid
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var current Token
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", token.Id, token.UserId).First(&current).Error; err != nil {
			return err
		}
		if token.Status == common.TokenStatusEnabled &&
			((current.ExpiredTime != -1 && current.ExpiredTime <= common.GetTimestamp()) ||
				(!current.UnlimitedQuota && current.RemainQuota <= 0)) {
			return ErrTokenInvalid
		}
		if err := invalidateTokenCacheForMutation(current.Key); err != nil {
			return err
		}
		if current.Status == token.Status {
			return nil
		}
		result := tx.Model(&Token{}).Where("id = ? AND user_id = ?", token.Id, token.UserId).Update("status", token.Status)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// InsertWithUserLimit serializes token issuance across processes using the
// owner row, so two requests cannot both claim the last available slot.
func (token *Token) InsertWithUserLimit(maxTokens int) error {
	if token == nil || token.UserId <= 0 || maxTokens <= 0 {
		return ErrTokenInvalid
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id", "status").First(&user, token.UserId).Error; err != nil {
			return err
		}
		if user.Status != common.UserStatusEnabled {
			return ErrTokenOwnerDisabled
		}
		var count int64
		if err := tx.Model(&Token{}).Where("user_id = ?", token.UserId).Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(maxTokens) {
			return ErrTokenLimitReached
		}
		return tx.Create(token).Error
	})
}
