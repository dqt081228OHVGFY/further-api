package model

import (
	"context"
	"errors"
	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
	"time"
)

const authFlowPurposePasswordReset = "password-reset"

type passwordResetEvidence struct {
	Email       string `json:"email"`
	AuthVersion int64  `json:"auth_version"`
}

func CreatePasswordResetFlowContext(ctx context.Context, email string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	email = NormalizeEmail(email)
	var accounts []User
	if err := DB.WithContext(ctx).Select("id", "auth_version").Where("LOWER(email) = ?", email).Limit(2).Find(&accounts).Error; err != nil {
		return "", err
	}
	if len(accounts) == 0 {
		return "", ErrEmailNotFound
	}
	if len(accounts) != 1 {
		return "", ErrEmailAmbiguous
	}
	payload, err := common.Marshal(passwordResetEvidence{Email: email, AuthVersion: accounts[0].AuthVersion})
	if err != nil {
		return "", err
	}
	token, _, err := CreateAuthFlowContext(ctx, AuthFlowCreate{Purpose: authFlowPurposePasswordReset, UserId: accounts[0].Id, Payload: string(payload), ExpiresAt: time.Now().Add(time.Duration(common.VerificationValidMinutes) * time.Minute)})
	return token, err
}

func ResetPasswordWithFlowContext(ctx context.Context, email, token, password string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	email = NormalizeEmail(email)
	hash, err := common.HashAccountPasswordContext(ctx, password)
	if err != nil {
		return err
	}
	var userID int
	_, err = ConsumeAuthFlowWithActionContext(ctx, token, AuthFlowMatch{Purpose: authFlowPurposePasswordReset}, func(tx *gorm.DB, flow *AuthFlow) error {
		var evidence passwordResetEvidence
		if err := common.UnmarshalJsonStr(flow.Payload, &evidence); err != nil || evidence.Email != email {
			return ErrAuthFlowInvalid
		}
		var account User
		if err := lockForUpdate(tx).First(&account, flow.UserId).Error; err != nil {
			return err
		}
		if NormalizeEmail(account.Email) != email || account.AuthVersion != evidence.AuthVersion {
			return ErrAuthFlowInvalid
		}
		userID = account.Id
		if _, err := IncrementUserAuthVersionWithTx(tx, userID); err != nil {
			return err
		}
		return tx.Model(&User{}).Where("id = ?", userID).Update("password", hash).Error
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
