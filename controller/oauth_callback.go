package controller

import (
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/i18n"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/oauth"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
)

type oauthCallbackState struct {
	providerName string
	provider     oauth.Provider
	state        string
	flow         *model.AuthFlow
	match        model.AuthFlowMatch
	payload      oauthFlowPayload
}

func rejectOAuthState(c *gin.Context) {
	c.JSON(http.StatusForbidden, gin.H{"success": false, "message": i18n.T(c, i18n.MsgOAuthStateInvalid)})
}

func prepareOAuthCallback(c *gin.Context) (*oauthCallbackState, bool) {
	if c.Request.Context().Err() != nil {
		writeSecurityOperationError(c, c.Request.Context().Err())
		return nil, false
	}
	name := c.Param("provider")
	provider := oauth.GetProvider(name)
	if provider == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": i18n.T(c, i18n.MsgOAuthUnknownProvider)})
		return nil, false
	}
	flow, err := model.GetAuthFlowContext(c.Request.Context(), c.Query("state"), model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth, Provider: name})
	if err != nil {
		rejectOAuthState(c)
		return nil, false
	}
	callback := &oauthCallbackState{providerName: name, provider: provider, state: c.Query("state"), flow: flow, match: model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth, Provider: name, Intent: flow.Intent}}
	if flow.Intent == model.AuthFlowIntentBind || name == "telegram" {
		if err := common.UnmarshalJsonStr(flow.Payload, &callback.payload); err != nil {
			writeSecurityOperationError(c, model.ErrAuthFlowInvalid)
			return nil, false
		}
	}
	if flow.Intent == model.AuthFlowIntentBind || flow.Intent == model.AuthFlowIntentVerify {
		identity, valid := middleware.GetSessionAuthIdentity(c)
		if !valid || identity.UserID != flow.UserId || identity.SessionID != flow.SessionId {
			rejectOAuthState(c)
			return nil, false
		}
		callback.match.UserId, callback.match.SessionId = identity.UserID, identity.SessionID
		if flow.Intent == model.AuthFlowIntentBind {
			proofContext, err := common.Marshal(service.AccountBindingContext{Provider: name})
			if err != nil {
				writeSecurityOperationError(c, err)
				return nil, false
			}
			if err := service.ValidateFlowAuthorization(identity, service.VerificationOperation{Scope: service.VerificationScopeAccountBind, Context: proofContext}, callback.payload.Authorization); err != nil {
				writeSecurityOperationError(c, err)
				return nil, false
			}
		}
	} else if flow.Intent != model.AuthFlowIntentLogin {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return nil, false
	} else if !validateOAuthBrowserBinding(c, callback.state) {
		rejectOAuthState(c)
		return nil, false
	}
	if name == "telegram" {
		if err := oauth.TelegramConfigurationError(); err != nil {
			writeSecurityOperationError(c, err)
			return nil, false
		}
		if callback.payload.Telegram == nil {
			writeSecurityOperationError(c, model.ErrAuthFlowInvalid)
			return nil, false
		}
		if flow.Intent != model.AuthFlowIntentLogin {
			identity, _ := middleware.GetSessionAuthIdentity(c)
			if callback.payload.SessionIdentity == nil || *callback.payload.SessionIdentity != identity {
				writeSecurityOperationError(c, model.ErrAuthFlowInvalid)
				return nil, false
			}
			if _, _, err := service.ValidateLoginSession(identity); err != nil {
				writeSecurityOperationError(c, err)
				return nil, false
			}
		}
		c.Set(oauth.TelegramOAuthFlowContextKey, callback.payload.Telegram)
	}
	if !provider.IsEnabled() {
		common.ApiErrorI18n(c, i18n.MsgOAuthNotEnabled, providerParams(provider.GetName()))
		return nil, false
	}
	return callback, true
}

func (callback *oauthCallbackState) providerFailure(c *gin.Context, err error) {
	if callback.providerName == "telegram" || c.Request.Context().Err() != nil {
		writeSecurityOperationError(c, err)
	} else {
		handleOAuthError(c, err)
	}
}

func HandleOAuth(c *gin.Context) {
	if c.Param("provider") == "linuxdo" && strings.Contains(c.GetHeader("Accept"), "text/html") {
		c.Header("Cache-Control", "no-store")
		c.Header("Referrer-Policy", "no-referrer")
		c.Redirect(http.StatusFound, "/oauth/linuxdo?"+c.Request.URL.Query().Encode())
		return
	}
	callback, valid := prepareOAuthCallback(c)
	if !valid {
		return
	}
	bound, notificationFailed := false, false
	if callback.flow.Intent == model.AuthFlowIntentBind {
		defer func() {
			recordUserSecurityAudit(c, callback.flow.UserId, "user.binding_bind", map[string]any{"provider": callback.providerName, "success": bound, "notification_failed": notificationFailed})
		}()
	}
	if refusal := c.Query("error"); refusal != "" {
		if _, err := model.ConsumeAuthFlowContext(c.Request.Context(), callback.state, callback.match); err != nil {
			rejectOAuthState(c)
			return
		}
		if callback.flow.Intent == model.AuthFlowIntentLogin {
			clearOAuthBrowserBinding(c, callback.state)
		}
		message := c.Query("error_description")
		if message == "" {
			message = refusal
		}
		common.ApiErrorMsg(c, message)
		return
	}
	token, err := callback.provider.ExchangeToken(c.Request.Context(), c.Query("code"), c)
	if err != nil {
		callback.providerFailure(c, err)
		return
	}
	remote, err := callback.provider.GetUserInfo(c.Request.Context(), token)
	if err != nil {
		callback.providerFailure(c, err)
		return
	}
	if err := c.Request.Context().Err(); err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	if callback.flow.Intent == model.AuthFlowIntentBind {
		bound, notificationFailed = handleOAuthBind(c, callback.providerName, callback.provider, remote, callback.flow, callback.state, callback.match)
		return
	}
	flow, err := model.ConsumeAuthFlowContext(c.Request.Context(), callback.state, callback.match)
	if err != nil {
		rejectOAuthState(c)
		return
	}
	if flow.Intent == model.AuthFlowIntentLogin {
		clearOAuthBrowserBinding(c, callback.state)
	}
	switch flow.Intent {
	case model.AuthFlowIntentLogin:
		handleOAuthLogin(c, callback.provider, remote, token, flow)
	case model.AuthFlowIntentVerify:
		handleOAuthVerification(c, callback.providerName, remote, flow)
	}
}
