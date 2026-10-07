package controller

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/gin-gonic/gin"
)

func furtherBusinessError(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
}

func furtherBusinessJSON(c *gin.Context, destination any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	if err := c.ShouldBindJSON(destination); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请求参数无效"})
		return false
	}
	return true
}

func FurtherGetLoanSettings(c *gin.Context) {
	settings, err := model.GetFurtherLoanSettings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"settings": settings.View()})
}

func FurtherPutLoanSettings(c *gin.Context) {
	var settings model.FurtherLoanSettings
	if !furtherBusinessJSON(c, &settings) {
		return
	}
	if err := model.SaveFurtherLoanSettings(settings); err != nil {
		furtherBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, 0, "further.loan.settings", map[string]any{"enabled": settings.Enabled, "max_borrow_quota": settings.MaxBorrowQuota})
	common.ApiSuccess(c, gin.H{"settings": settings.View()})
}

func FurtherGetLoan(c *gin.Context) {
	status, err := model.FurtherLoanStatus(c.GetInt("id"), time.Now().UTC())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, status)
}

func FurtherBorrow(c *gin.Context) {
	var body struct {
		Quota int `json:"quota"`
	}
	if !furtherBusinessJSON(c, &body) {
		return
	}
	loan, err := model.FurtherBorrow(c.GetInt("id"), body.Quota, time.Now().UTC())
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeSystem, "词元贷到账，借款ID "+strconv.FormatInt(loan.ID, 10)+"，额度 "+strconv.Itoa(loan.Quota))
	common.ApiSuccess(c, gin.H{"loan": loan, "message": "词元贷额度已到账"})
}

func FurtherLoanCheckIn(c *gin.Context) {
	record, err := model.FurtherLoanCheckIn(c.GetInt("id"), time.Now().UTC())
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	status, err := model.FurtherLoanStatus(c.GetInt("id"), time.Now().UTC())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"checkin": record, "status": status, "message": "今日签到还款已完成"})
}

func FurtherCutLoanPool(c *gin.Context) {
	record, err := model.FurtherCutLossPool(c.GetInt("id"), time.Now().UTC())
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	status, err := model.FurtherLoanStatus(c.GetInt("id"), time.Now().UTC())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"cut": record, "status": status, "message": "已完成本次砍价"})
}

func FurtherGetRegistrationConfig(c *gin.Context) {
	settings, err := model.GetFurtherRegistrationSettings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	view, err := model.FurtherRegistrationConfig(settings, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func FurtherGetRegistrationSettings(c *gin.Context) {
	settings, err := model.GetFurtherRegistrationSettings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	view, err := model.FurtherRegistrationConfig(settings, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"settings": view, "linuxdo": gin.H{"client_id": common.LinuxDOClientId}})
}

func FurtherPatchRegistrationSettings(c *gin.Context) {
	var body struct {
		model.FurtherRegistrationSettings
		NewCode string `json:"new_registration_code"`
	}
	// PATCH preserves fields omitted by newer/older clients.
	settings, err := model.GetFurtherRegistrationSettings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// LinuxDO login is managed by site settings. Seed PATCH from the live
	// provider option, not the historical registration-table copy.
	settings.LinuxdoLoginEnabled = common.LinuxDOOAuthEnabled
	body.FurtherRegistrationSettings = settings
	if !furtherBusinessJSON(c, &body) {
		return
	}
	if err := model.SaveFurtherRegistrationSettings(body.FurtherRegistrationSettings, strings.TrimSpace(body.NewCode)); err != nil {
		furtherBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, 0, "further.registration.settings", map[string]any{"registration_code_required": body.RegistrationCodeRequired, "shared_code_changed": body.NewCode != ""})
	FurtherGetRegistrationSettings(c)
}

func FurtherGenerateRegistrationCodes(c *gin.Context) {
	var body struct {
		Name  string `json:"name"`
		Group string `json:"user_group"`
		Count int    `json:"count"`
	}
	if !furtherBusinessJSON(c, &body) {
		return
	}
	codes, err := model.GenerateFurtherRegistrationCodes(body.Name, body.Group, body.Count, time.Now().UTC())
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	values := make([]string, len(codes))
	for index, code := range codes {
		values[index] = code.Code
	}
	recordManageAuditFor(c, 0, "further.registration.generate", map[string]any{"count": len(codes), "user_group": body.Group})
	common.ApiSuccess(c, gin.H{"codes": values, "count": len(codes), "batch_id": codes[0].BatchID})
}

func FurtherListRegistrationCodes(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	size, err := strconv.Atoi(c.DefaultQuery("page_size", "25"))
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	result, err := model.ListFurtherRegistrationCodes(page, size, strings.TrimSpace(c.Query("keyword")), c.Query("status"), c.Query("user_group"))
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func FurtherDeleteRegistrationCodes(c *gin.Context) {
	var id int64
	if c.Param("id") != "" {
		var err error
		id, err = strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "注册码ID无效"})
			return
		}
	}
	used := strings.HasSuffix(c.FullPath(), "/used")
	count, err := model.DeleteFurtherRegistrationCodes(id, c.Query("batch_id"), used)
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, 0, "further.registration.delete", map[string]any{"id": id, "used": used, "deleted_count": count})
	common.ApiSuccess(c, gin.H{"deleted_count": count})
}
