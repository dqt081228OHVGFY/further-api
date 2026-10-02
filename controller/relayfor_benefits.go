package controller

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func relayForBusinessError(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
}

func relayForBusinessJSON(c *gin.Context, destination any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	if err := c.ShouldBindJSON(destination); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请求参数无效"})
		return false
	}
	return true
}

func RelayForGetLoanSettings(c *gin.Context) {
	settings, err := model.GetRelayForLoanSettings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"settings": settings.View()})
}

func RelayForPutLoanSettings(c *gin.Context) {
	var settings model.RelayForLoanSettings
	if !relayForBusinessJSON(c, &settings) {
		return
	}
	if err := model.SaveRelayForLoanSettings(settings); err != nil {
		relayForBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, 0, "relayfor.loan.settings", map[string]any{"enabled": settings.Enabled, "max_borrow_quota": settings.MaxBorrowQuota})
	common.ApiSuccess(c, gin.H{"settings": settings.View()})
}

func RelayForGetLoan(c *gin.Context) {
	status, err := model.RelayForLoanStatus(c.GetInt("id"), time.Now().UTC())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, status)
}

func RelayForBorrow(c *gin.Context) {
	var body struct {
		Quota int `json:"quota"`
	}
	if !relayForBusinessJSON(c, &body) {
		return
	}
	loan, err := model.RelayForBorrow(c.GetInt("id"), body.Quota, time.Now().UTC())
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeSystem, "词元贷到账，借款ID "+strconv.FormatInt(loan.ID, 10)+"，额度 "+strconv.Itoa(loan.Quota))
	common.ApiSuccess(c, gin.H{"loan": loan, "message": "词元贷额度已到账"})
}

func RelayForLoanCheckIn(c *gin.Context) {
	record, err := model.RelayForLoanCheckIn(c.GetInt("id"), time.Now().UTC())
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	status, err := model.RelayForLoanStatus(c.GetInt("id"), time.Now().UTC())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"checkin": record, "status": status, "message": "今日签到还款已完成"})
}

func RelayForCutLoanPool(c *gin.Context) {
	record, err := model.RelayForCutLossPool(c.GetInt("id"), time.Now().UTC())
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	status, err := model.RelayForLoanStatus(c.GetInt("id"), time.Now().UTC())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"cut": record, "status": status, "message": "已完成本次砍价"})
}

func RelayForGetRegistrationConfig(c *gin.Context) {
	settings, err := model.GetRelayForRegistrationSettings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	view, err := model.RelayForRegistrationConfig(settings, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func RelayForGetRegistrationSettings(c *gin.Context) {
	settings, err := model.GetRelayForRegistrationSettings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	view, err := model.RelayForRegistrationConfig(settings, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"settings": view, "linuxdo": gin.H{"client_id": common.LinuxDOClientId}})
}

func RelayForPatchRegistrationSettings(c *gin.Context) {
	var body struct {
		model.RelayForRegistrationSettings
		NewCode string `json:"new_registration_code"`
	}
	// PATCH preserves fields omitted by newer/older clients.
	settings, err := model.GetRelayForRegistrationSettings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	body.RelayForRegistrationSettings = settings
	if !relayForBusinessJSON(c, &body) {
		return
	}
	if err := model.SaveRelayForRegistrationSettings(body.RelayForRegistrationSettings, strings.TrimSpace(body.NewCode)); err != nil {
		relayForBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, 0, "relayfor.registration.settings", map[string]any{"registration_code_required": body.RegistrationCodeRequired, "shared_code_changed": body.NewCode != ""})
	RelayForGetRegistrationSettings(c)
}

func RelayForGenerateRegistrationCodes(c *gin.Context) {
	var body struct {
		Name  string `json:"name"`
		Group string `json:"user_group"`
		Count int    `json:"count"`
	}
	if !relayForBusinessJSON(c, &body) {
		return
	}
	codes, err := model.GenerateRelayForRegistrationCodes(body.Name, body.Group, body.Count, time.Now().UTC())
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	values := make([]string, len(codes))
	for index, code := range codes {
		values[index] = code.Code
	}
	recordManageAuditFor(c, 0, "relayfor.registration.generate", map[string]any{"count": len(codes), "user_group": body.Group})
	common.ApiSuccess(c, gin.H{"codes": values, "count": len(codes), "batch_id": codes[0].BatchID})
}

func RelayForListRegistrationCodes(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	size, err := strconv.Atoi(c.DefaultQuery("page_size", "25"))
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	result, err := model.ListRelayForRegistrationCodes(page, size, strings.TrimSpace(c.Query("keyword")), c.Query("status"), c.Query("user_group"))
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func RelayForDeleteRegistrationCodes(c *gin.Context) {
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
	count, err := model.DeleteRelayForRegistrationCodes(id, c.Query("batch_id"), used)
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, 0, "relayfor.registration.delete", map[string]any{"id": id, "used": used, "deleted_count": count})
	common.ApiSuccess(c, gin.H{"deleted_count": count})
}
