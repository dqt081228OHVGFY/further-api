package controller

import (
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

func FurtherPublicActivityTheme(c *gin.Context) {
	data, err := model.FurtherActivitiesView(0, time.Now(), false)
	furtherRespond(c, data, err)
}
func FurtherUserActivities(c *gin.Context) {
	data, err := model.FurtherActivitiesView(c.GetInt("id"), time.Now(), false)
	furtherRespond(c, data, err)
}
func FurtherAdminActivities(c *gin.Context) {
	data, err := model.FurtherActivitiesView(0, time.Now(), true)
	furtherRespond(c, data, err)
}
func FurtherSaveActivity(c *gin.Context) {
	var body model.FurtherHolidayCampaign
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
	if err := model.SaveFurtherHolidayCampaign(body); err != nil {
		furtherBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, 0, "further.activity.configure", map[string]any{"key": body.Key, "reward_quota": body.RewardQuota})
	common.ApiSuccess(c, body)
}
func FurtherHolidayFollowRetired(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"success": false, "code": "HOLIDAY_FOLLOW_RETIRED", "message": "节日关注功能已停止提供"})
}
func FurtherClaimActivity(c *gin.Context) {
	var body struct {
		Key string `json:"key"`
	}
	if !furtherBusinessJSON(c, &body) {
		return
	}
	claim, err := model.ClaimFurtherHoliday(c.GetInt("id"), body.Key, time.Now())
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeSystem, "节日活动奖励："+claim.CampaignKey)
	common.ApiSuccess(c, claim)
}
func FurtherConfigureActivityCheckin(c *gin.Context) {
	var body operation_setting.CheckinSetting
	if !furtherBusinessJSON(c, &body) {
		return
	}
	if err := model.SaveFurtherActivityCheckinSettings(body); err != nil {
		furtherBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, 0, "further.activity.checkin.configure", map[string]any{"enabled": body.Enabled, "min_quota": body.MinQuota, "max_quota": body.MaxQuota})
	common.ApiSuccess(c, body)
}
