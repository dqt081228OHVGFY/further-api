package model

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"gorm.io/gorm"
)

// AccountProfileInput contains only administrator-editable account fields.
// Monetary counters, credentials, external identities and timestamps are not
// accepted through a profile edit.
type AccountProfileInput struct {
	ID               int                        `json:"id"`
	Username         string                     `json:"username"`
	DisplayName      string                     `json:"display_name"`
	Password         string                     `json:"password"`
	Role             *int                       `json:"role"`
	Group            *string                    `json:"group"`
	Remark           *string                    `json:"remark"`
	AdminPermissions map[string]map[string]bool `json:"admin_permissions"`
}

type AccountAuthorizationMutation func(tx *gorm.DB, userID, userRole, operatorRole int) error

func validateAccountProfile(input *AccountProfileInput, requirePassword bool) error {
	input.Username = strings.TrimSpace(input.Username)
	if length := utf8.RuneCountInString(input.Username); length == 0 || length > UserNameMaxLength {
		return errors.New("用户名需为 1 至 20 个字符")
	}
	if utf8.RuneCountInString(input.DisplayName) > 20 {
		return errors.New("显示名称不能超过 20 个字符")
	}
	if requirePassword || input.Password != "" {
		if length := utf8.RuneCountInString(input.Password); length < 8 || length > 128 {
			return errors.New("密码需为 8 至 128 个字符")
		}
	}
	if input.Remark != nil && utf8.RuneCountInString(*input.Remark) > 255 {
		return errors.New("账户备注不能超过 255 个字符")
	}
	if input.Group != nil {
		group := strings.TrimSpace(*input.Group)
		if group == "" || len(group) > 64 {
			return errors.New("用户分组无效")
		}
		input.Group = &group
	}
	return nil
}

func canManageAccount(operatorRole, targetRole int) bool {
	return operatorRole == common.RoleRootUser || operatorRole > targetRole
}

func EditManagedAccount(ctx context.Context, operator AccountSecurityOperator, input AccountProfileInput, authorization AccountAuthorizationMutation) (*User, error) {
	if input.ID <= 0 {
		return nil, errors.New("用户编号无效")
	}
	if err := validateAccountProfile(&input, false); err != nil {
		return nil, err
	}
	passwordHash := ""
	if input.Password != "" {
		var err error
		passwordHash, err = common.HashAccountPasswordContext(ctx, input.Password)
		if err != nil {
			return nil, err
		}
	}
	var account User
	authChanged := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		target, operatorRole, err := lockManagedAccountOperator(tx, operator, input.ID, false)
		if err != nil {
			return err
		}
		account = *target
		if input.Role != nil && *input.Role != 0 && *input.Role != account.Role {
			return errors.New("请通过角色管理操作修改用户角色")
		}
		updates := map[string]any{"username": input.Username, "display_name": input.DisplayName}
		if input.Group != nil {
			updates["group"] = *input.Group
			authChanged = account.Group != *input.Group
		}
		if input.Remark != nil {
			updates["remark"] = *input.Remark
		}
		if passwordHash != "" {
			updates["password"] = passwordHash
			authChanged = true
		}
		if authChanged {
			if _, err := IncrementUserAuthVersionWithTx(tx, account.Id); err != nil {
				return err
			}
		}
		if err := tx.Model(&account).Updates(updates).Error; err != nil {
			return err
		}
		if authorization != nil {
			if err := authorization(tx, account.Id, account.Role, operatorRole); err != nil {
				return err
			}
		}
		return tx.First(&account, account.Id).Error
	})
	if err != nil {
		return nil, err
	}
	if err := PublishUserAuthCache(account.Id); err != nil {
		return nil, err
	}
	if authChanged {
		if _, err := RevokeAllUserSessions(account.Id, "admin_user_update"); err != nil {
			return nil, err
		}
	}
	return &account, nil
}

func CreateManagedAccount(ctx context.Context, operator AccountSecurityOperator, input AccountProfileInput, authorization AccountAuthorizationMutation) (*User, error) {
	if err := validateAccountProfile(&input, true); err != nil {
		return nil, err
	}
	role := common.RoleCommonUser
	if input.Role != nil && *input.Role != 0 {
		role = *input.Role
	}
	if role != common.RoleCommonUser && role != common.RoleAdminUser {
		return nil, errors.New("无权创建该角色的账户")
	}
	password, err := common.HashAccountPasswordContext(ctx, input.Password)
	if err != nil {
		return nil, err
	}
	account := User{Username: input.Username, DisplayName: input.DisplayName, Password: password, Role: role, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Quota: common.QuotaForNewUser, AffCode: common.GetRandomString(16)}
	if account.DisplayName == "" {
		account.DisplayName = account.Username
	}
	account.SetSetting(dto.UserSetting{SidebarModules: defaultAccountNavigation(role)})
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, operatorRole, err := lockManagedAccountOperator(tx, operator, 0, false)
		if err != nil {
			return err
		}
		if role >= operatorRole {
			return ErrAccountSecurityPermission
		}
		if err := tx.Create(&account).Error; err != nil {
			return err
		}
		if authorization != nil {
			return authorization(tx, account.Id, account.Role, operatorRole)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if account.Quota > 0 {
		RecordLog(account.Id, LogTypeSystem, "新用户注册赠送 "+logger.LogQuota(account.Quota))
	}
	return &account, nil
}

type AccountActionInput struct {
	ID     int    `json:"id"`
	Action string `json:"action"`
}

// ApplyManagedAccountAction changes one field under the user lock. It cannot
// restore a stale profile, balance or credential from an HTTP snapshot.
func ApplyManagedAccountAction(ctx context.Context, operator AccountSecurityOperator, input AccountActionInput, authorization AccountAuthorizationMutation) (*User, error) {
	if input.ID <= 0 {
		return nil, errors.New("用户编号无效")
	}
	switch input.Action {
	case "enable", "disable", "promote", "demote", "delete":
	default:
		return nil, errors.New("用户管理操作无效")
	}
	var account User
	var nextVersion int64
	changed := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		target, operatorRole, err := lockManagedAccountOperator(tx, operator, input.ID, false)
		if err != nil {
			return err
		}
		account = *target
		if account.Role == common.RoleRootUser && input.Action != "enable" {
			return errors.New("不能禁用、降级或删除超级管理员")
		}
		updates := map[string]any{}
		switch input.Action {
		case "enable", "disable":
			status := common.UserStatusEnabled
			if input.Action == "disable" {
				status = common.UserStatusDisabled
			}
			changed = account.Status != status
			updates["status"] = status
		case "promote":
			if operatorRole != common.RoleRootUser || account.Role >= common.RoleAdminUser {
				return errors.New("仅超级管理员可将普通账户设为管理员")
			}
			changed, updates["role"] = true, common.RoleAdminUser
		case "demote":
			if account.Role <= common.RoleCommonUser {
				return errors.New("该账户已经是普通用户")
			}
			changed, updates["role"] = true, common.RoleCommonUser
		case "delete":
			changed = true
		}
		if changed {
			var err error
			nextVersion, err = IncrementUserAuthVersionWithTx(tx, account.Id)
			if err != nil {
				return err
			}
		}
		if input.Action == "delete" {
			return tx.Delete(&account).Error
		}
		if err := tx.Model(&account).Updates(updates).Error; err != nil {
			return err
		}
		if authorization != nil {
			if err := authorization(tx, account.Id, account.Role, operatorRole); err != nil {
				return err
			}
		}
		return tx.First(&account, account.Id).Error
	})
	if err != nil {
		return nil, err
	}
	if input.Action == "delete" {
		if err := publishCommittedUserAuthVersion(account.Id, nextVersion); err != nil {
			return nil, err
		}
	} else if err := PublishUserAuthCache(account.Id); err != nil {
		return nil, err
	}
	if changed {
		reason := "user_security_changed"
		if input.Action == "demote" {
			reason = "admin_demote"
		} else if input.Action == "delete" {
			reason = "user_deleted"
		}
		if _, err := RevokeAllUserSessions(account.Id, reason); err != nil {
			return nil, err
		}
	}
	if err := InvalidateUserTokensCache(account.Id); err != nil {
		return nil, err
	}
	if input.Action == "delete" {
		if err := invalidateUserCache(account.Id); err != nil {
			return nil, err
		}
	}
	return &account, nil
}

func EraseManagedAccount(ctx context.Context, operator AccountSecurityOperator, userID int) (*User, error) {
	if userID <= 0 {
		return nil, errors.New("用户编号无效")
	}
	var account User
	var tokens []Token
	var tombstone int64
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		target, operatorRole, err := lockManagedAccountOperator(tx, operator, userID, true)
		if err != nil {
			return err
		}
		account = *target
		if operatorRole <= account.Role {
			return errors.New("无权删除该用户")
		}
		tombstone, err = IncrementUserAuthVersionWithTx(tx, userID)
		if err != nil {
			return err
		}
		if common.RedisEnabled {
			if err := tx.Unscoped().Select("id", commonKeyCol).Where("user_id = ?", userID).Find(&tokens).Error; err != nil {
				return err
			}
		}
		if err := deleteUserAuthenticationData(tx, userID); err != nil {
			return err
		}
		return tx.Unscoped().Delete(&account).Error
	})
	if err != nil {
		return nil, err
	}
	if err := publishCommittedUserAuthVersion(userID, tombstone); err != nil {
		common.SysError("failed to publish deleted account auth version: " + err.Error())
	}
	if err := invalidateTokensCache(tokens); err != nil {
		common.SysError("failed to revoke deleted account token caches: " + err.Error())
	}
	if err := invalidateUserCache(userID); err != nil {
		common.SysError("failed to invalidate deleted account cache: " + err.Error())
	}
	return &account, nil
}
