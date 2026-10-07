package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
)

const ExternalIdentityProviderWeChat = "wechat"

var ErrWeChatRegistrationDisabled = errors.New("当前站点未开放新账户注册")

func wechatClaimSubject(subject string) string {
	digest := sha256.Sum256([]byte(subject))
	return hex.EncodeToString(digest[:])
}

// Compare provider subjects as bytes even on a case-insensitive SQL collation.
func readWeChatAccountTx(tx *gorm.DB, subject string) (*User, error) {
	var candidates []User
	if err := tx.Unscoped().Where("wechat_id = ?", subject).Find(&candidates).Error; err != nil {
		return nil, err
	}
	var account *User
	for index := range candidates {
		if candidates[index].WeChatId != subject {
			continue
		}
		if account != nil {
			return nil, ErrExternalIdentityAlreadyClaimed
		}
		account = &candidates[index]
	}
	return account, nil
}

func ResolveWeChatAccountContext(ctx context.Context, subject string, allowRegistration bool) (*User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	subject = strings.TrimSpace(subject)
	if subject == "" || len(subject) > 128 {
		return nil, ErrAccountBindingChanged
	}
	var account *User
	created := false
	attemptedCreation := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		account, err = readWeChatAccountTx(tx, subject)
		if err != nil {
			return err
		}
		if account != nil {
			if account.DeletedAt.Valid || account.Status != common.UserStatusEnabled {
				return ErrUserSessionInactive
			}
			return ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderWeChat, wechatClaimSubject(subject), account.Id)
		}
		if !allowRegistration {
			return ErrWeChatRegistrationDisabled
		}
		attemptedCreation = true
		account = &User{Username: "wechat_" + common.GetRandomString(12), DisplayName: "微信账户", WeChatId: subject, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1}
		if err := account.InsertWithTx(tx, 0); err != nil {
			return err
		}
		if err := ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderWeChat, wechatClaimSubject(subject), account.Id); err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil && attemptedCreation && ctx.Err() == nil {
		var winner *User
		lookupErr := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			winner, err = readWeChatAccountTx(tx, subject)
			if err != nil || winner == nil {
				return err
			}
			if winner.DeletedAt.Valid || winner.Status != common.UserStatusEnabled {
				return ErrUserSessionInactive
			}
			return ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderWeChat, wechatClaimSubject(subject), winner.Id)
		})
		if lookupErr != nil {
			return nil, lookupErr
		}
		if winner != nil && !winner.DeletedAt.Valid && winner.Status == common.UserStatusEnabled {
			return winner, nil
		}
	}
	if err != nil {
		return nil, err
	}
	if created {
		account.FinalizeOAuthUserCreation(0)
	}
	return account, nil
}

func BindWeChatAccountContext(ctx context.Context, identity AuthSessionIdentity, subject string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	subject = strings.TrimSpace(subject)
	if subject == "" || len(subject) > 128 {
		return ErrAccountBindingChanged
	}
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ValidateAuthSessionWithTx(tx, identity); err != nil {
			return err
		}
		owner, err := readWeChatAccountTx(tx, subject)
		if err != nil {
			return err
		}
		if owner != nil && owner.Id != identity.UserID {
			return ErrExternalIdentityAlreadyClaimed
		}
		var current User
		if err := tx.Select("id", "wechat_id").First(&current, identity.UserID).Error; err != nil {
			return err
		}
		if current.WeChatId != subject {
			if err := ReleaseExternalIdentityWithTx(tx, ExternalIdentityProviderWeChat, identity.UserID); err != nil {
				return err
			}
		}
		if err := ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderWeChat, wechatClaimSubject(subject), identity.UserID); err != nil {
			return err
		}
		return tx.Model(&User{}).Where("id = ?", identity.UserID).Update("wechat_id", subject).Error
	})
	if err != nil {
		return err
	}
	return invalidateUserCache(identity.UserID)
}
