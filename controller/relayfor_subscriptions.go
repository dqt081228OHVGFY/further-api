package controller

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func relayForSubscriptionID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"success": false, "message": "无效的记录ID"})
		return 0, false
	}
	return id, true
}

func RelayForAdminSubscriptionPlans(c *gin.Context) {
	data, err := model.ListRelayForSubscriptionPlans(0, true)
	relayForRespond(c, data, err)
}
func RelayForUserSubscriptionPlans(c *gin.Context) {
	data, err := model.ListRelayForSubscriptionPlans(c.GetInt("id"), false)
	relayForRespond(c, data, err)
}
func RelayForSaveSubscriptionPlan(c *gin.Context) {
	id := 0
	if c.Param("id") != "" {
		var ok bool
		id, ok = relayForSubscriptionID(c)
		if !ok {
			return
		}
	}
	var body model.RelayForPlanInput
	if !relayForBusinessJSON(c, &body) {
		return
	}
	if body.QuotaGroup != "" {
		if _, exists := ratio_setting.GetGroupRatioCopy()[body.QuotaGroup]; !exists {
			c.JSON(400, gin.H{"success": false, "message": "用户组不存在"})
			return
		}
	}
	id, err := model.SaveRelayForSubscriptionPlan(id, body)
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id})
}

func RelayForSubscriptionPlanStatus(c *gin.Context) {
	id, ok := relayForSubscriptionID(c)
	if !ok {
		return
	}
	var body struct {
		Status int `json:"status"`
	}
	if !relayForBusinessJSON(c, &body) {
		return
	}
	if body.Status != 1 && body.Status != 2 {
		c.JSON(400, gin.H{"success": false, "message": "无效状态"})
		return
	}
	result := model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", id).Update("enabled", body.Status == 1)
	if result.Error != nil {
		relayForRespond(c, nil, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(404, gin.H{"success": false, "message": "套餐不存在"})
		return
	}
	model.InvalidateSubscriptionPlanCache(id)
	common.ApiSuccess(c, gin.H{"id": id, "status": body.Status})
}

func RelayForUserSubscriptions(c *gin.Context) {
	data, err := model.ListRelayForUserSubscriptions(c.GetInt("id"), false)
	relayForRespond(c, data, err)
}
func RelayForUserTimedQuotas(c *gin.Context) {
	data, err := model.ListRelayForUserSubscriptions(c.GetInt("id"), true)
	relayForRespond(c, data, err)
}
func RelayForDeleteUserSubscription(c *gin.Context) {
	id, ok := relayForSubscriptionID(c)
	if !ok {
		return
	}
	if err := model.HideRelayForUserSubscription(c.GetInt("id"), id); err != nil {
		relayForBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id})
}

func RelayForAbandonSubscription(c *gin.Context) {
	id, ok := relayForSubscriptionID(c)
	if !ok {
		return
	}
	var body struct {
		Mode     string `json:"mode"`
		TOTPCode string `json:"totp_code"`
	}
	if !relayForBusinessJSON(c, &body) {
		return
	}
	userID := c.GetInt("id")
	twoFA, err := model.GetTwoFAByUserId(userID)
	if err != nil {
		relayForRespond(c, nil, err)
		return
	}
	if twoFA != nil && twoFA.IsEnabled {
		errorType := "totp_required"
		valid := false
		if body.TOTPCode != "" {
			valid, err = twoFA.ValidateTOTPAndUpdateUsage(body.TOTPCode)
			errorType = "totp_invalid"
		}
		if !valid || err != nil {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "请验证当前账户的动态验证码", "error": gin.H{"type": errorType}})
			return
		}
	}
	data, err := model.AbandonRelayForSubscription(userID, id, body.Mode)
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	model.RecordLog(userID, model.LogTypeManage, "用户放弃订阅 "+strconv.Itoa(id)+"，处理方式 "+body.Mode)
	common.ApiSuccess(c, data)
}

func RelayForPatchBillingPreference(c *gin.Context) {
	var body struct {
		PreferTimedQuota *bool `json:"prefer_timed_quota"`
	}
	if !relayForBusinessJSON(c, &body) {
		return
	}
	if body.PreferTimedQuota == nil {
		c.JSON(400, gin.H{"success": false, "message": "必须提供扣费优先级"})
		return
	}
	if err := model.SetRelayForBillingPriority(c.GetInt("id"), *body.PreferTimedQuota); err != nil {
		relayForRespond(c, nil, err)
		return
	}
	priority := "subscription_first"
	if *body.PreferTimedQuota {
		priority = "timed_first"
	}
	common.ApiSuccess(c, gin.H{"billing_priority": priority, "prefer_timed_quota": *body.PreferTimedQuota})
}

func RelayForAdminBenefitCampaigns(c *gin.Context) {
	data, err := model.ListRelayForBenefitCampaigns()
	relayForRespond(c, data, err)
}

func RelayForCreateBenefitCampaign(c *gin.Context) {
	var body struct {
		model.RelayForBenefitCampaign
		TimedExpiry string `json:"timed_quota_expires_at"`
	}
	if !relayForBusinessJSON(c, &body) {
		return
	}
	for _, group := range []string{body.TargetUserGroup, body.QuotaGroup} {
		group = strings.TrimSpace(group)
		if group != "" {
			if _, ok := ratio_setting.GetGroupRatioCopy()[group]; !ok {
				c.JSON(400, gin.H{"success": false, "message": "用户组不存在"})
				return
			}
		}
	}
	if body.TimedExpiry != "" {
		expiry, err := time.Parse(time.RFC3339, body.TimedExpiry)
		if err != nil {
			c.JSON(400, gin.H{"success": false, "message": "无效的限时额度到期时间"})
			return
		}
		body.TimedQuotaExpiresAt = expiry.Unix()
	}
	body.CreatedBy = c.GetInt("id")
	data, err := model.CreateRelayForBenefitCampaign(body.RelayForBenefitCampaign)
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, "管理员发放订阅福利："+body.Name)
	common.ApiSuccess(c, data)
}

func RelayForRevokeBenefitCampaign(c *gin.Context) {
	id, ok := relayForSubscriptionID(c)
	if !ok {
		return
	}
	remove := !strings.HasSuffix(c.Request.URL.Path, "/pending")
	count, err := model.RevokeRelayForBenefitCampaign(id, remove)
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, "管理员撤回或删除福利批次 "+strconv.Itoa(id))
	common.ApiSuccess(c, gin.H{"revoked_count": count})
}
func RelayForUserBenefits(c *gin.Context) {
	data, err := model.ListRelayForUserBenefits(c.GetInt("id"))
	relayForRespond(c, data, err)
}
func RelayForClaimBenefit(c *gin.Context) {
	id, ok := relayForSubscriptionID(c)
	if !ok {
		return
	}
	data, err := model.ClaimRelayForBenefit(c.GetInt("id"), id)
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}
func RelayForDismissBenefit(c *gin.Context) {
	id, ok := relayForSubscriptionID(c)
	if !ok {
		return
	}
	if err := model.DismissRelayForBenefit(c.GetInt("id"), id); err != nil {
		relayForBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id})
}
func RelayForProcessSubscriptionResets(c *gin.Context) {
	count, err := model.ResetDueSubscriptions(1000)
	if err != nil {
		relayForRespond(c, nil, err)
		return
	}
	expired, err := model.ExpireDueSubscriptions(1000)
	if err != nil {
		relayForRespond(c, nil, err)
		return
	}
	common.ApiSuccess(c, gin.H{"result": gin.H{"reset": count, "expired": expired}})
}
