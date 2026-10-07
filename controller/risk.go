package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func riskWindow(c *gin.Context) (int, bool) {
	switch c.DefaultQuery("window", "7d") {
	case "3d":
		return 3, true
	case "7d":
		return 7, true
	default:
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "统计窗口仅支持最近 3 天或 7 天。"})
		return 0, false
	}
}

func riskUserID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "用户编号无效。"})
		return 0, false
	}
	return id, true
}

func ListRiskUsers(c *gin.Context) {
	windowDays, ok := riskWindow(c)
	if !ok {
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 10000 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "页码必须为 1 至 10000。"})
		return
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || pageSize < 1 || pageSize > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "每页数量必须为 1 至 100。"})
		return
	}
	username := strings.TrimSpace(c.Query("username"))
	if len(username) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "用户名筛选内容过长。"})
		return
	}
	level := c.Query("level")
	if level != "" && level != "attention" && level != "low" && level != "medium" && level != "high" && level != "critical" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "风险等级无效。"})
		return
	}
	status := c.Query("status")
	if status != "" && status != "unreviewed" && status != "watching" && status != "confirmed" && status != "dismissed" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "审查状态无效。"})
		return
	}
	result, err := service.ListRiskUsersContext(c.Request.Context(), service.RiskListQuery{
		WindowDays: windowDays, Page: page, PageSize: pageSize,
		Username: username, Level: level, Status: status,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func GetRiskUser(c *gin.Context) {
	id, ok := riskUserID(c)
	if !ok {
		return
	}
	windowDays, ok := riskWindow(c)
	if !ok {
		return
	}
	result, err := service.GetRiskUserContext(c.Request.Context(), id, windowDays)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "用户不存在。"})
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func UpdateRiskUserStatus(c *gin.Context) {
	id, ok := riskUserID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var body struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请提交有效的审查状态。"})
		return
	}
	if body.Status != "unreviewed" && body.Status != "watching" && body.Status != "confirmed" && body.Status != "dismissed" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "风险审查状态无效。"})
		return
	}
	if err := service.SetRiskReviewStatusContext(c.Request.Context(), id, c.GetInt("id"), body.Status); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "用户不存在。"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "更新风险审查状态失败，请稍后重试。"})
		return
	}
	recordManageAuditFor(c, id, "risk.review_status", map[string]any{"status": body.Status})
	common.ApiSuccess(c, gin.H{"user_id": id, "status": body.Status})
}
