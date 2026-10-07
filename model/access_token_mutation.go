package model

import (
	"context"

	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Personal-token mutations revalidate the proof's browser identity under the
// user/session locks. Consuming a proof must not keep authorizing a request
// after a concurrent logout or account-security change.
func UpdateUserAccessTokenForSessionContext(ctx context.Context, identity AuthSessionIdentity, token string) error {
	if token == "" {
		return ErrUserSessionInvalid
	}
	return DB.WithContext(ctx).Session(&gorm.Session{Logger: DB.Logger.LogMode(gormlogger.Silent)}).Transaction(func(tx *gorm.DB) error {
		if err := ValidateAuthSessionWithTx(tx, identity); err != nil {
			return err
		}
		result := tx.Model(&User{}).Where("id = ?", identity.UserID).Updates(map[string]any{"access_token": token, "access_token_created_at": common.GetTimestamp()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func RevokeUserAccessTokenForSessionContext(ctx context.Context, identity AuthSessionIdentity) (string, error) {
	var ref string
	err := DB.WithContext(ctx).Session(&gorm.Session{Logger: DB.Logger.LogMode(gormlogger.Silent)}).Transaction(func(tx *gorm.DB) error {
		if err := ValidateAuthSessionWithTx(tx, identity); err != nil {
			return err
		}
		var user User
		if err := tx.Select("id", "access_token").First(&user, identity.UserID).Error; err != nil {
			return err
		}
		ref = AccessTokenFingerprint(user.GetAccessToken())
		if ref == "" {
			return nil
		}
		return tx.Model(&User{}).Where("id = ?", identity.UserID).Updates(map[string]any{"access_token": nil, "access_token_created_at": nil}).Error
	})
	return ref, err
}
