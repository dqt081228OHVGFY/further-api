package controller

import (
	"fmt"
	"strconv"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/i18n"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service/authz"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func updateAdminPermissionsForUserInTx(operatorRole int, tx *gorm.DB, userID int, userRole int, permissions map[string]map[string]bool) (bool, error) {
	if permissions == nil {
		if userRole < common.RoleAdminUser && operatorRole == common.RoleRootUser {
			return true, authz.ClearUserAuthorizationInTx(tx, userID)
		}
		return false, nil
	}
	if operatorRole != common.RoleRootUser {
		return false, fmt.Errorf("only root can update admin permissions")
	}
	if userRole < common.RoleAdminUser {
		return true, authz.ClearUserAuthorizationInTx(tx, userID)
	}
	return true, authz.SetUserPermissionsInTx(tx, userID, permissions)
}

func saveManagedAccount(c *gin.Context, creating bool) {
	var input model.AccountProfileInput
	if err := c.ShouldBindJSON(&input); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	policyChanged := false
	permissions := func(tx *gorm.DB, userID, role, operatorRole int) error {
		changed, err := updateAdminPermissionsForUserInTx(operatorRole, tx, userID, role, input.AdminPermissions)
		policyChanged = policyChanged || changed
		return err
	}
	var account *model.User
	var err error
	if creating {
		account, err = model.CreateManagedAccount(c.Request.Context(), managedSecurityOperator(c), input, permissions)
	} else {
		account, err = model.EditManagedAccount(c.Request.Context(), managedSecurityOperator(c), input, permissions)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if policyChanged {
		if err := authz.ReloadPolicy(); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	action := "user.update"
	params := map[string]any{"username": account.Username, "id": account.Id}
	if creating {
		action = "user.create"
		params = map[string]any{"username": account.Username, "role": account.Role}
	}
	recordManageAuditFor(c, account.Id, action, params)
	common.ApiSuccess(c, nil)
}

func UpdateUser(c *gin.Context) { saveManagedAccount(c, false) }

func CreateUser(c *gin.Context) { saveManagedAccount(c, true) }

type ManageRequest struct {
	Id     int    `json:"id"`
	Action string `json:"action"`
	Value  int    `json:"value"`
	Mode   string `json:"mode"`
}

func ManageUser(c *gin.Context) {
	var input ManageRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if input.Action == "add_quota" {
		manageUserQuota(c, input)
		return
	}
	var authorization model.AccountAuthorizationMutation
	if input.Action == "demote" {
		authorization = func(tx *gorm.DB, userID, role, operatorRole int) error {
			return authz.ClearUserAuthorizationInTx(tx, userID)
		}
	}
	account, err := model.ApplyManagedAccountAction(c.Request.Context(), managedSecurityOperator(c), model.AccountActionInput{ID: input.Id, Action: input.Action}, authorization)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if authorization != nil {
		if err := authz.ReloadPolicy(); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	recordManageAuditFor(c, account.Id, "user.manage", map[string]any{"action": input.Action, "username": account.Username, "id": account.Id})
	if input.Action == "delete" {
		common.ApiSuccess(c, nil)
		return
	}
	common.ApiSuccess(c, gin.H{"role": account.Role, "status": account.Status})
}

func DeleteUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	account, err := model.EraseManagedAccount(c.Request.Context(), managedSecurityOperator(c), id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, account.Id, "user.delete", map[string]any{"username": account.Username, "id": account.Id})
	common.ApiSuccess(c, nil)
}
