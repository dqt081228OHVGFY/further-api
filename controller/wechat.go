package controller

import (
	"encoding/json"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/service/business"
	"github.com/gin-gonic/gin"
)

type wechatBindRequest struct {
	Code string `json:"code"`
}

func WeChatAuth(c *gin.Context) {
	assertion, err := business.ExchangeWeChatCodeContext(c.Request.Context(), c.Query("code"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.OptionMapRWMutex.RLock()
	allowRegistration := common.RegisterEnabled
	common.OptionMapRWMutex.RUnlock()
	account, err := model.ResolveWeChatAccountContext(c.Request.Context(), assertion.Subject, allowRegistration)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	setupLogin(account, nil, c)
}

func WeChatBind(c *gin.Context) {
	identity, valid := middleware.GetSessionAuthIdentity(c)
	if !valid {
		writeSecurityOperationError(c, service.ErrAuthTokenInvalid)
		return
	}
	succeeded, notificationFailed := false, false
	defer func() {
		recordUserSecurityAudit(c, identity.UserID, "user.binding_bind", map[string]any{"provider": "wechat", "success": succeeded, "notification_failed": notificationFailed})
	}()
	var request wechatBindRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&request); err != nil {
		common.ApiErrorMsg(c, "微信绑定请求格式无效")
		return
	}
	request.Code = strings.TrimSpace(request.Code)
	binding, err := common.Marshal(service.AccountBindingContext{Provider: "wechat", Code: request.Code})
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	if middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeAccountBind, Context: binding}) == nil {
		return
	}
	assertion, err := business.ExchangeWeChatCodeContext(c.Request.Context(), request.Code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.BindWeChatAccountContext(c.Request.Context(), identity, assertion.Subject); err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	succeeded = true
	account, err := model.GetUserByIdContext(c.Request.Context(), identity.UserID, false)
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	notificationFailed = service.NotifyAccountSecurityChange(account.Email, "微信账户已绑定") != nil
	common.ApiSuccess(c, gin.H{"notification_warning": notificationFailed})
}
