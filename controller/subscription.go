package controller

import (
	"fmt"
	"strconv"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/gin-gonic/gin"
)

type SubscriptionPlanDTO struct {
	Plan model.SubscriptionPlan `json:"plan"`
}

type BillingPreferenceRequest struct {
	BillingPreference string `json:"billing_preference"`
}

type AdminUpsertSubscriptionPlanRequest struct {
	Plan model.SubscriptionPlanInput `json:"plan"`
}

type AdminUpdateSubscriptionPlanStatusRequest struct {
	Enabled *bool `json:"enabled"`
}

type AdminBindSubscriptionRequest struct {
	UserId int `json:"user_id"`
	PlanId int `json:"plan_id"`
}

type AdminCreateUserSubscriptionRequest struct {
	PlanId int `json:"plan_id"`
}

type AdminResetSubscriptionRequest struct {
	PlanId           int   `json:"plan_id"`
	AdvanceResetTime *bool `json:"advance_reset_time"`
}

func subscriptionResourceID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "请提供有效的资源编号")
		return 0, false
	}
	return id, true
}

func writeSubscriptionCatalog(c *gin.Context, enabledOnly bool) {
	plans, err := model.ListSubscriptionCatalog(c.Request.Context(), enabledOnly)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result := make([]SubscriptionPlanDTO, len(plans))
	for index := range plans {
		result[index] = SubscriptionPlanDTO{Plan: plans[index]}
	}
	common.ApiSuccess(c, result)
}

func GetSubscriptionPlans(c *gin.Context) { writeSubscriptionCatalog(c, true) }

func AdminListSubscriptionPlans(c *gin.Context) { writeSubscriptionCatalog(c, false) }

func GetSubscriptionSelf(c *gin.Context) {
	workspace, err := model.ReadSubscriptionWorkspace(c.Request.Context(), c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, workspace)
}

func UpdateSubscriptionPreference(c *gin.Context) {
	var input BillingPreferenceRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		common.ApiErrorMsg(c, "扣费方式参数无效")
		return
	}
	preference, err := model.SetSubscriptionBillingPreference(c.Request.Context(), c.GetInt("id"), input.BillingPreference)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"billing_preference": preference})
}

func saveSubscriptionCatalog(c *gin.Context, id int) {
	var input AdminUpsertSubscriptionPlanRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		common.ApiErrorMsg(c, "套餐参数无效")
		return
	}
	plan, err := model.SaveSubscriptionCatalog(c.Request.Context(), id, input.Plan)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, plan)
}

func AdminCreateSubscriptionPlan(c *gin.Context) { saveSubscriptionCatalog(c, 0) }

func AdminUpdateSubscriptionPlan(c *gin.Context) {
	if id, valid := subscriptionResourceID(c); valid {
		saveSubscriptionCatalog(c, id)
	}
}

func AdminUpdateSubscriptionPlanStatus(c *gin.Context) {
	id, valid := subscriptionResourceID(c)
	if !valid {
		return
	}
	var input AdminUpdateSubscriptionPlanStatusRequest
	if err := c.ShouldBindJSON(&input); err != nil || input.Enabled == nil {
		common.ApiErrorMsg(c, "请指定套餐是否启用")
		return
	}
	if err := model.SetSubscriptionCatalogEnabled(c.Request.Context(), id, *input.Enabled); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func writeSubscriptionGrant(c *gin.Context, userID, planID int) {
	if userID <= 0 || planID <= 0 {
		common.ApiErrorMsg(c, "请选择用户与套餐")
		return
	}
	message, err := model.AdminBindSubscriptionContext(c.Request.Context(), userID, planID, "")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	writeSubscriptionMutation(c, message)
}

func AdminBindSubscription(c *gin.Context) {
	var input AdminBindSubscriptionRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		common.ApiErrorMsg(c, "订阅发放参数无效")
		return
	}
	writeSubscriptionGrant(c, input.UserId, input.PlanId)
}

func AdminCreateUserSubscription(c *gin.Context) {
	userID, valid := subscriptionResourceID(c)
	if !valid {
		return
	}
	var input AdminCreateUserSubscriptionRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		common.ApiErrorMsg(c, "请选择要发放的套餐")
		return
	}
	writeSubscriptionGrant(c, userID, input.PlanId)
}

func AdminListUserSubscriptions(c *gin.Context) {
	userID, valid := subscriptionResourceID(c)
	if !valid {
		return
	}
	subscriptions, err := model.GetAllUserSubscriptionsContext(c.Request.Context(), userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, subscriptions)
}

func resolveAdvanceResetTime(value *bool) bool { return value == nil || *value }

func recordSubscriptionResetUserLogs(c *gin.Context, result *model.SubscriptionResetResult, adminInfo *model.AuditAdminInfo) {
	if result == nil || result.ResetCount == 0 {
		return
	}
	content := fmt.Sprintf("管理员重置订阅套餐 %s（ID: %d）额度", result.PlanTitle, result.PlanId)
	for _, userID := range result.AffectedUserIds {
		model.RecordLogWithAdminInfo(userID, model.LogTypeManage, content, adminInfo, nil, c)
	}
}

func resetSubscriptionQuota(c *gin.Context, allUsers bool) {
	resourceID, valid := subscriptionResourceID(c)
	if !valid {
		return
	}
	var input AdminResetSubscriptionRequest
	if err := c.ShouldBindJSON(&input); err != nil || (!allUsers && input.PlanId <= 0) {
		common.ApiErrorMsg(c, "订阅重置参数无效")
		return
	}
	var result *model.SubscriptionResetResult
	var err error
	if allUsers {
		result, err = model.AdminResetPlanSubscriptionsContext(c.Request.Context(), resourceID, resolveAdvanceResetTime(input.AdvanceResetTime))
	} else {
		result, err = model.AdminResetUserSubscriptionsByPlanContext(c.Request.Context(), resourceID, input.PlanId, resolveAdvanceResetTime(input.AdvanceResetTime))
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordSubscriptionResetUserLogs(c, result, auditOperatorInfo(c))
	params := map[string]any{"plan_id": result.PlanId, "plan_title": result.PlanTitle, "reset_count": result.ResetCount, "user_count": result.UserCount, "advance_reset_time": result.AdvanceResetTime}
	if allUsers {
		recordManageAudit(c, "subscription.plan_reset", params)
	} else {
		params["target_user_id"] = resourceID
		recordManageAuditFor(c, resourceID, "subscription.user_plan_reset", params)
	}
	common.ApiSuccess(c, result)
}

func AdminResetUserSubscriptionsByPlan(c *gin.Context) { resetSubscriptionQuota(c, false) }

func AdminResetPlanSubscriptions(c *gin.Context) { resetSubscriptionQuota(c, true) }

func writeSubscriptionMutation(c *gin.Context, message string) {
	if message == "" {
		common.ApiSuccess(c, nil)
	} else {
		common.ApiSuccess(c, gin.H{"message": message})
	}
}

func removeSubscription(c *gin.Context, erase bool) {
	id, valid := subscriptionResourceID(c)
	if !valid {
		return
	}
	var message string
	var err error
	if erase {
		message, err = model.AdminDeleteUserSubscriptionContext(c.Request.Context(), id)
	} else {
		message, err = model.AdminInvalidateUserSubscriptionContext(c.Request.Context(), id)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	writeSubscriptionMutation(c, message)
}

func AdminInvalidateUserSubscription(c *gin.Context) { removeSubscription(c, false) }

func AdminDeleteUserSubscription(c *gin.Context) { removeSubscription(c, true) }
