package model

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var ErrAccountNameUnavailable = errors.New("account name is unavailable")

type PublicRegistrationInput struct {
	Username                 string
	Password                 string
	Email                    string
	AffiliateCode            string
	RegistrationCode         string
	IssueDefaultToken        bool
	DefaultTokenAutoGroup    bool
	RequireEmailVerification bool
	EmailVerificationCode    string
}

func CreateRegisteredAccountContext(ctx context.Context, input PublicRegistrationInput) (*User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	input.Username = strings.TrimSpace(input.Username)
	input.Email = NormalizeEmail(input.Email)
	input.RegistrationCode = strings.TrimSpace(input.RegistrationCode)
	if length := utf8.RuneCountInString(input.Username); length < 1 || length > UserNameMaxLength {
		return nil, errors.New("用户名需为 1 至 20 个字符")
	}
	if len(input.RegistrationCode) > 128 || len(input.AffiliateCode) > 32 {
		return nil, errors.New("注册凭据无效")
	}
	hash, err := common.HashAccountPasswordContext(ctx, input.Password)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	defaultKey := ""
	if input.IssueDefaultToken {
		defaultKey, err = common.GenerateKey()
		if err != nil {
			return nil, err
		}
	}
	account := User{Username: input.Username, DisplayName: input.Username, Password: hash, Email: input.Email, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1, AffCode: common.GetRandomString(16)}
	account.SetSetting(dto.UserSetting{SidebarModules: defaultAccountNavigation(common.RoleCommonUser)})
	legacyEmailCode := false
	err = DB.WithContext(ctx).Session(&gorm.Session{Logger: DB.Logger.LogMode(gormlogger.Silent)}).Transaction(func(tx *gorm.DB) error {
		settings, err := furtherRegistrationSettings(tx)
		if err != nil {
			return err
		}
		if !settings.LocalRegistrationEnabled {
			return errors.New("此用户组未开放本地注册")
		}
		account.Group = "default"
		var code FurtherRegistrationCode
		oneUse, shared := false, false
		if input.RegistrationCode != "" {
			err = lockForUpdate(tx).Where("code_digest = ?", furtherCodeDigest(input.RegistrationCode)).First(&code).Error
			if err == nil {
				if code.Status != 1 {
					return errors.New("注册码已使用")
				}
				account.Group, oneUse = code.UserGroup, true
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			} else if settings.RegistrationCodeHash != "" {
				valid, err := common.ValidatePasswordAndHashContext(ctx, input.RegistrationCode, settings.RegistrationCodeHash)
				if err != nil {
					return err
				}
				if !valid {
					return errors.New("注册码无效")
				}
				shared = true
			} else {
				return errors.New("注册码无效")
			}
		}
		if settings.RegistrationCodeRequired && !oneUse && !shared {
			return errors.New("请提供有效注册码")
		}
		account.Quota = settings.PublicInitialQuota
		if input.AffiliateCode != "" {
			var inviter User
			if err := tx.Select("id").Where("aff_code = ?", input.AffiliateCode).First(&inviter).Error; err == nil {
				account.InviterId = inviter.Id
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if err := withNormalizedEmailLock(tx, account.Email, func(tx *gorm.DB) error {
			if input.RequireEmailVerification {
				var err error
				legacyEmailCode, err = consumeRegistrationEmailChallengeTx(tx, account.Email, input.EmailVerificationCode)
				if err != nil {
					return err
				}
			}
			if err := ensureEmailAvailableWithTx(tx, account.Email, 0); err != nil {
				return err
			}
			var existing int64
			if err := tx.Unscoped().Model(&User{}).Where("username = ?", account.Username).Count(&existing).Error; err != nil {
				return err
			}
			if existing > 0 {
				return ErrAccountNameUnavailable
			}
			return tx.Create(&account).Error
		}); err != nil {
			return err
		}
		if oneUse {
			result := tx.Model(&FurtherRegistrationCode{}).Where("id = ? AND status = 1", code.ID).Updates(map[string]any{"status": 2, "used_by_user_id": account.Id, "used_by_account": account.Username, "used_at": time.Now().UTC()})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errors.New("注册码已被使用")
			}
		}
		if input.IssueDefaultToken {
			now := common.GetTimestamp()
			token := Token{UserId: account.Id, Name: account.Username + "的初始令牌", Key: defaultKey, Status: common.TokenStatusEnabled, CreatedTime: now, AccessedTime: now, ExpiredTime: -1, RemainQuota: 500000, UnlimitedQuota: true}
			if input.DefaultTokenAutoGroup {
				token.Group = "auto"
			}
			return tx.Create(&token).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if legacyEmailCode {
		common.DeleteKey(account.Email, common.EmailVerificationPurpose)
	}
	account.FinishInsert(account.InviterId)
	return &account, nil
}
