package controller

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/i18n"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

type PaymentComplianceRequest struct {
	Confirmed bool `json:"confirmed"`
}

func requirePaymentCompliance(c *gin.Context) bool {
	if !operation_setting.IsPaymentComplianceConfirmed() {
		common.ApiErrorI18n(c, i18n.MsgPaymentComplianceRequired)
		return false
	}
	return true
}

func ConfirmPaymentCompliance(c *gin.Context) {
	identity, valid := middleware.GetSessionAuthIdentity(c)
	if !valid || c.GetBool("use_access_token") {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "code": "AUTH_SESSION_REQUIRED", "message": "请使用超级管理员的网页登录会话确认付款渠道条件"})
		return
	}
	var request PaymentComplianceRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&request); err != nil {
		common.ApiErrorMsg(c, "确认信息格式无效")
		return
	}
	if !request.Confirmed {
		common.ApiErrorMsg(c, "请阅读并明确确认付款渠道使用条件")
		return
	}
	confirmation, err := model.ConfirmGatewayPaymentTermsContext(c.Request.Context(), identity, c.ClientIP())
	if err != nil {
		if errors.Is(err, model.ErrPaymentConfirmationRootRequired) {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "code": "ROOT_REQUIRED", "message": err.Error()})
			return
		}
		if errors.Is(err, model.ErrUserSessionInactive) {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "code": "AUTH_UNAUTHORIZED", "message": "登录会话已失效，请重新登录"})
			return
		}
		common.SysError("确认付款渠道条件: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "确认未保存，请稍后重试"})
		return
	}
	recordManageAudit(c, "payment.compliance.confirm", map[string]any{"terms_version": confirmation.TermsVersion, "confirmed_at": confirmation.ConfirmedAt})
	common.ApiSuccess(c, confirmation)
}
