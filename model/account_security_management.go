package model

import (
	"context"
	"crypto/hmac"
	"errors"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
)

var ErrAccountSecurityPermission = errors.New("无权修改该账户的安全设置")

type AccountSecurityOperator struct {
	UserID                 int
	Role                   int
	Session                *AuthSessionIdentity
	PersonalAccessTokenRef string
}

type ManagedSecurityMutation struct {
	TargetID    int
	Action      string
	BindingType string
	ProviderID  int
}

// lockManagedAccountOperator serializes authorization with the account change.
// A role copied by HTTP middleware cannot authorize a mutation after the actor
// has been disabled, demoted, logged out, or had their personal token rotated.
func lockManagedAccountOperator(tx *gorm.DB, operator AccountSecurityOperator, targetID int, includeDeletedTarget bool) (*User, int, error) {
	if operator.UserID <= 0 || operator.Role < common.RoleAdminUser || (operator.Session == nil && operator.PersonalAccessTokenRef == "") {
		return nil, 0, ErrAccountSecurityPermission
	}
	query := lockForUpdate(tx)
	if includeDeletedTarget {
		query = query.Unscoped()
	}
	var rows []User
	if err := query.Where("id IN ?", []int{operator.UserID, targetID}).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	var actor, target *User
	for index := range rows {
		if rows[index].Id == operator.UserID {
			actor = &rows[index]
		}
		if rows[index].Id == targetID {
			target = &rows[index]
		}
	}
	if targetID > 0 && target == nil {
		return nil, 0, gorm.ErrRecordNotFound
	}
	if actor == nil || actor.DeletedAt.Valid || actor.Status != common.UserStatusEnabled || actor.Role < common.RoleAdminUser {
		return nil, 0, ErrAccountSecurityPermission
	}
	role := min(actor.Role, operator.Role)
	if operator.PersonalAccessTokenRef != "" && !hmac.Equal([]byte(AccessTokenFingerprint(actor.GetAccessToken())), []byte(operator.PersonalAccessTokenRef)) {
		return nil, 0, ErrAccountSecurityPermission
	}
	if target != nil && !canManageAccount(role, target.Role) {
		return nil, 0, ErrAccountSecurityPermission
	}
	if operator.Session != nil {
		if operator.Session.UserID != operator.UserID {
			return nil, 0, ErrUserSessionInactive
		}
		if err := ValidateAuthSessionWithTx(tx, *operator.Session); err != nil {
			return nil, 0, err
		}
	}
	return target, role, nil
}

func lockManagedSecurityAccounts(tx *gorm.DB, operator AccountSecurityOperator, targetID int) (*User, error) {
	target, _, err := lockManagedAccountOperator(tx, operator, targetID, false)
	return target, err
}

// Administrative credential removal authorizes both locked account records.
// Snapshot roles from the HTTP request never authorize the committed mutation.
func MutateManagedAccountSecurityContext(ctx context.Context, operator AccountSecurityOperator, input ManagedSecurityMutation) (*User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	if input.TargetID <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var target *User
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		target, err = lockManagedSecurityAccounts(tx, operator, input.TargetID)
		if err != nil {
			return err
		}
		switch input.Action {
		case "oauth-unbind":
			if input.ProviderID <= 0 {
				return ErrAccountBindingChanged
			}
			var binding UserOAuthBinding
			if err := lockForUpdate(tx).Where("user_id = ? AND provider_id = ?", target.Id, input.ProviderID).First(&binding).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrAccountBindingChanged
				}
				return err
			}
			if _, err := IncrementUserAuthVersionWithTx(tx, target.Id); err != nil {
				return err
			}
			return tx.Delete(&binding).Error
		case "twofa-disable":
			var factor TwoFA
			if err := lockForUpdate(tx).Where("user_id = ? AND is_enabled = ?", target.Id, true).First(&factor).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrTwoFANotEnabled
				}
				return err
			}
			if _, err := IncrementUserAuthVersionWithTx(tx, target.Id); err != nil {
				return err
			}
			if err := tx.Unscoped().Where("user_id = ?", target.Id).Delete(&TwoFABackupCode{}).Error; err != nil {
				return err
			}
			return tx.Unscoped().Delete(&factor).Error
		case "passkey-reset":
			var credential PasskeyCredential
			if err := lockForUpdate(tx).Where("user_id = ?", target.Id).First(&credential).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrPasskeyNotFound
				}
				return err
			}
			if _, err := IncrementUserAuthVersionWithTx(tx, target.Id); err != nil {
				return err
			}
			return tx.Unscoped().Delete(&credential).Error
		case "binding-clear":
			binding := strings.ToLower(strings.TrimSpace(input.BindingType))
			column, valid := map[string]string{"email": "email", "github": "github_id", "discord": "discord_id", "oidc": "oidc_id", "wechat": "wechat_id", "telegram": "telegram_id", "linuxdo": "linux_do_id"}[binding]
			if !valid {
				return ErrAccountBindingChanged
			}
			if _, err := IncrementUserAuthVersionWithTx(tx, target.Id); err != nil {
				return err
			}
			if err := tx.Model(&User{}).Where("id = ?", target.Id).Update(column, "").Error; err != nil {
				return err
			}
			if binding == ExternalIdentityProviderTelegram || binding == ExternalIdentityProviderWeChat {
				return ReleaseExternalIdentityWithTx(tx, binding, target.Id)
			}
			return nil
		default:
			return ErrAccountBindingChanged
		}
	})
	if err != nil {
		return nil, err
	}
	if err := PublishUserAuthCache(target.Id); err != nil {
		return nil, err
	}
	if _, err := RevokeAllUserSessions(target.Id, "admin_security_"+input.Action); err != nil {
		return nil, err
	}
	return target, nil
}
