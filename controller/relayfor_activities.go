package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

func RelayForPublicActivityTheme(c *gin.Context) {
	data, err := model.RelayForActivitiesView(0, time.Now(), false)
	relayForRespond(c, data, err)
}
func RelayForUserActivities(c *gin.Context) {
	data, err := model.RelayForActivitiesView(c.GetInt("id"), time.Now(), false)
	relayForRespond(c, data, err)
}
func RelayForAdminActivities(c *gin.Context) {
	data, err := model.RelayForActivitiesView(0, time.Now(), true)
	relayForRespond(c, data, err)
}
func RelayForSaveActivity(c *gin.Context) {
	var body model.RelayForHolidayCampaign
	// Full Chinese details and rules can exceed the shared 16 KiB body limit.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 65536)
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请求参数无效"})
		return
	}
	if body.TargetGroup != "" {
		if _, ok := ratio_setting.GetGroupRatioCopy()[body.TargetGroup]; !ok {
			common.ApiErrorMsg(c, "目标用户组不存在")
			return
		}
	}
	body.UpdatedBy = c.GetInt("id")
	if err := model.SaveRelayForHolidayCampaign(body); err != nil {
		relayForBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, 0, "relayfor.activity.configure", map[string]any{"key": body.Key, "reward_quota": body.RewardQuota})
	common.ApiSuccess(c, body)
}
func RelayForSetActivitySubscription(c *gin.Context) {
	var body struct {
		Key        string `json:"key"`
		Subscribed *bool  `json:"subscribed"`
	}
	if !relayForBusinessJSON(c, &body) {
		return
	}
	if body.Subscribed == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请明确是否订阅活动"})
		return
	}
	if err := model.SetRelayForHolidaySubscription(c.GetInt("id"), body.Key, *body.Subscribed, time.Now()); err != nil {
		relayForBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"key": body.Key, "subscribed": *body.Subscribed})
}
func RelayForClaimActivity(c *gin.Context) {
	var body struct {
		Key string `json:"key"`
	}
	if !relayForBusinessJSON(c, &body) {
		return
	}
	claim, err := model.ClaimRelayForHoliday(c.GetInt("id"), body.Key, time.Now())
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeSystem, "节日活动奖励："+claim.CampaignKey)
	common.ApiSuccess(c, claim)
}
func RelayForConfigureActivityCheckin(c *gin.Context) {
	var body operation_setting.CheckinSetting
	if !relayForBusinessJSON(c, &body) {
		return
	}
	if err := model.SaveRelayForActivityCheckinSettings(body); err != nil {
		relayForBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, 0, "relayfor.activity.checkin.configure", map[string]any{"enabled": body.Enabled, "min_quota": body.MinQuota, "max_quota": body.MaxQuota})
	common.ApiSuccess(c, body)
}
