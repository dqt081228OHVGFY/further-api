package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"gorm.io/gorm"
)

func defaultAccountNavigation(role int) string {
	navigation := map[string]any{
		"workspace": map[string]bool{"overview": true, "tokens": true, "usage": true, "tasks": true},
		"account":   map[string]bool{"subscriptions_benefits": true, "billing": true, "security": true, "settings": true},
	}
	if role >= common.RoleAdminUser {
		navigation["operations"] = map[string]bool{"channels": true, "models": true, "users": true, "activities": true, "risk": true, "audit": true, "site": role == common.RoleRootUser}
	}
	encoded, err := common.Marshal(navigation)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func (user *User) InsertWithTx(tx *gorm.DB, inviterID int) error {
	if tx == nil || user == nil {
		return errors.New("账户创建参数无效")
	}
	return withNormalizedEmailLock(tx, user.Email, func(tx *gorm.DB) error {
		if err := user.prepareForInsert(tx); err != nil {
			return err
		}
		user.Quota, user.AffCode = common.QuotaForNewUser, common.GetRandomString(16)
		if user.Role == 0 {
			user.Role = common.RoleCommonUser
		}
		preferences := user.GetSetting()
		if preferences.SidebarModules == "" {
			preferences.SidebarModules = defaultAccountNavigation(user.Role)
		}
		user.SetSetting(preferences)
		return tx.Create(user).Error
	})
}

func (user *User) Insert(inviterID int) error {
	if err := DB.Transaction(func(tx *gorm.DB) error { return user.InsertWithTx(tx, inviterID) }); err != nil {
		return err
	}
	user.finishInsert(inviterID)
	return nil
}

func (user *User) finishInsert(inviterID int) {
	if user == nil || user.Id <= 0 {
		return
	}
	if err := MutateAccountPreferences(context.Background(), user.Id, func(current *User, preferences *dto.UserSetting) error {
		if preferences.SidebarModules == "" {
			preferences.SidebarModules = defaultAccountNavigation(current.Role)
		}
		return nil
	}); err != nil {
		common.SysError("failed to initialize account navigation: " + err.Error())
	}
	if common.QuotaForNewUser > 0 {
		RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("新用户注册赠送 %s", logger.LogQuota(common.QuotaForNewUser)))
	}
	if inviterID <= 0 || !operation_setting.IsPaymentComplianceConfirmed() {
		return
	}
	if common.QuotaForInvitee > 0 {
		if err := IncreaseUserQuota(user.Id, common.QuotaForInvitee, true); err != nil {
			common.SysError("failed to award invitation allowance: " + err.Error())
		} else {
			RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("使用邀请码赠送 %s", logger.LogQuota(common.QuotaForInvitee)))
		}
	}
	if common.QuotaForInviter > 0 {
		if err := inviteUser(inviterID); err != nil {
			common.SysError("failed to award inviter allowance: " + err.Error())
		} else {
			RecordLog(inviterID, LogTypeSystem, fmt.Sprintf("邀请用户赠送 %s", logger.LogQuota(common.QuotaForInviter)))
		}
	}
}

func (user *User) FinishInsert(inviterID int) { user.finishInsert(inviterID) }

func (user *User) FinalizeOAuthUserCreation(inviterID int) { user.finishInsert(inviterID) }
