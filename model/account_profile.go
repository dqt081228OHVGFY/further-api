package model

import (
	"errors"

	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
)

func writeUserProfileTx(tx *gorm.DB, desired *User, password, editor bool) (bool, error) {
	if tx == nil || desired == nil || desired.Id <= 0 {
		return false, gorm.ErrRecordNotFound
	}
	var current User
	if err := lockForUpdate(tx).First(&current, desired.Id).Error; err != nil {
		return false, err
	}
	updates := make(map[string]any)
	if editor {
		updates["username"], updates["display_name"] = desired.Username, desired.DisplayName
		updates["group"], updates["remark"] = desired.Group, desired.Remark
	} else {
		for _, field := range []struct {
			column string
			value  string
		}{
			{"username", desired.Username}, {"display_name", desired.DisplayName},
			{"email", desired.Email}, {"github_id", desired.GitHubId}, {"discord_id", desired.DiscordId},
			{"oidc_id", desired.OidcId}, {"wechat_id", desired.WeChatId}, {"telegram_id", desired.TelegramId},
			{"linux_do_id", desired.LinuxDOId}, {"group", desired.Group}, {"aff_code", desired.AffCode},
			{"setting", desired.Setting}, {"remark", desired.Remark}, {"stripe_customer", desired.StripeCustomer},
		} {
			if field.value != "" {
				updates[field.column] = field.value
			}
		}
		for _, field := range []struct {
			column string
			value  int
		}{
			{"role", desired.Role}, {"status", desired.Status}, {"inviter_id", desired.InviterId},
		} {
			if field.value != 0 {
				updates[field.column] = field.value
			}
		}
	}
	if password {
		hash, err := common.HashAccountPasswordContext(tx.Statement.Context, desired.Password)
		if err != nil {
			return false, err
		}
		updates["password"] = hash
	}
	changed := password
	for _, field := range []struct {
		column string
		value  any
	}{
		{"role", current.Role}, {"status", current.Status}, {"group", current.Group},
	} {
		if incoming, present := updates[field.column]; present && incoming != field.value {
			changed = true
		}
	}
	if changed {
		if _, err := IncrementUserAuthVersionWithTx(tx, current.Id); err != nil {
			return false, err
		}
	}
	if len(updates) > 0 {
		if err := tx.Model(&User{}).Where("id = ?", current.Id).Updates(updates).Error; err != nil {
			return false, err
		}
	}
	if err := tx.First(desired, current.Id).Error; err != nil {
		return false, err
	}
	return changed, nil
}

func persistUserProfile(desired *User, password, editor bool) error {
	if desired == nil || desired.Id <= 0 {
		return errors.New("用户编号无效")
	}
	changed := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		changed, err = writeUserProfileTx(tx, desired, password, editor)
		return err
	})
	if err != nil {
		return err
	}
	if err := updateUserCache(*desired); err != nil {
		return err
	}
	if changed {
		_, err := RevokeAllUserSessions(desired.Id, "user_security_changed")
		return err
	}
	return nil
}

func (user *User) Update(password bool) error { return persistUserProfile(user, password, false) }

func (user *User) Edit(password bool) error { return persistUserProfile(user, password, true) }

func (user *User) UpdateWithTx(tx *gorm.DB, password bool) error {
	_, err := writeUserProfileTx(tx, user, password, false)
	return err
}

func (user *User) EditWithTx(tx *gorm.DB, password bool) error {
	_, err := writeUserProfileTx(tx, user, password, true)
	return err
}
