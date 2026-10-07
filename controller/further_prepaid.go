package controller

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func furtherPrepaidAdmin(c *gin.Context) bool {
	if c.GetInt("id") <= 0 || c.GetInt("role") < common.RoleAdminUser {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": "需要管理员权限"})
		return false
	}
	return true
}

func FurtherPrepaidList(c *gin.Context) {
	if !furtherPrepaidAdmin(c) {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "25"))
	page, size = max(1, page), min(100, max(1, size))
	result, err := model.ListFurtherPrepaidCards(page, size, strings.TrimSpace(c.Query("keyword")), c.Query("status"))
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func FurtherPrepaidGenerate(c *gin.Context) {
	if !furtherPrepaidAdmin(c) {
		return
	}
	var body struct {
		Name           string `json:"name"`
		UserGroup      string `json:"user_group"`
		Count          int    `json:"count"`
		Quota          int    `json:"quota"`
		ValidityDays   int    `json:"validity_days"`
		ClaimExpiresAt string `json:"claim_expires_at"`
	}
	if !furtherBusinessJSON(c, &body) {
		return
	}
	body.UserGroup = strings.TrimSpace(body.UserGroup)
	if _, exists := ratio_setting.GetGroupRatioCopy()[body.UserGroup]; !exists {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请选择网关中实际存在的用户组"})
		return
	}
	var expiry int64
	if body.ClaimExpiresAt != "" {
		parsed, err := time.Parse(time.RFC3339, body.ClaimExpiresAt)
		if err != nil {
			parsed, err = time.ParseInLocation("2006-01-02T15:04", body.ClaimExpiresAt, time.FixedZone("UTC+8", 8*3600))
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "兑换截止时间格式无效"})
			return
		}
		expiry = parsed.Unix()
	}
	cards, err := model.GenerateFurtherPrepaidCards(model.FurtherPrepaidGenerateInput{Name: body.Name,
		UserGroup: body.UserGroup, Count: body.Count, Quota: body.Quota, ValidityDays: body.ValidityDays,
		ClaimExpiresAt: expiry, CreatedBy: c.GetInt("id")})
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	codes := make([]string, len(cards))
	for index, card := range cards {
		codes[index] = card.Code
	}
	recordManageAuditFor(c, 0, "further.prepaid.generate", map[string]any{"batch_id": cards[0].BatchID, "count": len(cards), "quota": body.Quota})
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, gin.H{"codes": codes, "count": len(cards), "batch_id": cards[0].BatchID})
}

func FurtherPrepaidDelete(c *gin.Context) {
	if !furtherPrepaidAdmin(c) {
		return
	}
	id, _ := strconv.Atoi(c.Param("id"))
	count, err := model.DeleteFurtherPrepaidCards(id, c.Query("batch_id"), false)
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, id, "further.prepaid.delete_unused", map[string]any{"deleted_count": count})
	common.ApiSuccess(c, gin.H{"deleted_count": count})
}

func FurtherPrepaidDeleteUsed(c *gin.Context) {
	if !furtherPrepaidAdmin(c) {
		return
	}
	count, err := model.DeleteFurtherPrepaidCards(0, "", true)
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, 0, "further.prepaid.cleanup", map[string]any{"deleted_count": count})
	common.ApiSuccess(c, gin.H{"deleted_count": count})
}

func FurtherPrepaidRedeem(c *gin.Context) {
	if c.GetInt("id") <= 0 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "请先登录"})
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if !furtherBusinessJSON(c, &body) {
		return
	}
	result, err := model.RedeemFurtherPrepaidCard(c.GetInt("id"), body.Code)
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, result)
}

func FurtherPrepaidBalance(c *gin.Context) {
	if c.GetInt("id") <= 0 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "请先登录"})
		return
	}
	result, err := model.GetFurtherPrepaidBalance(c.GetInt("id"))
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, result)
}
