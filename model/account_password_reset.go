package model

import (
	"context"
	"errors"

	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
)

func ResetUserPasswordByEmailContext(ctx context.Context, email, password string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	email = NormalizeEmail(email)
	if email == "" || password == "" {
		return errors.New("邮箱地址或密码为空")
	}
	hash, err := common.HashAccountPasswordContext(ctx, password)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var userID int
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return withNormalizedEmailLock(tx, email, func(tx *gorm.DB) error {
			var matches []User
			if err := lockForUpdate(tx).Where("LOWER(email) = ?", email).Limit(2).Find(&matches).Error; err != nil {
				return err
			}
			if len(matches) == 0 {
				return ErrEmailNotFound
			}
			if len(matches) != 1 {
				return ErrEmailAmbiguous
			}
			userID = matches[0].Id
			if _, err := IncrementUserAuthVersionWithTx(tx, userID); err != nil {
				return err
			}
			return tx.Model(&User{}).Where("id = ? AND LOWER(email) = ?", userID, email).Update("password", hash).Error
		})
	})
	if err != nil {
		return errors.Join(err, ctx.Err())
	}
	if err := PublishUserAuthCache(userID); err != nil {
		return err
	}
	_, err = RevokeAllUserSessions(userID, "password_reset")
	return err
}

func ResetUserPasswordByEmail(email, password string) error {
	return ResetUserPasswordByEmailContext(context.Background(), email, password)
}
