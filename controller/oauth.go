package controller

import (
	"errors"
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/i18n"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/oauth"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/service/business"
	"github.com/gin-gonic/gin"
	"net/http"
)

type oauthStateRequest = business.OAuthFlowRequest
type oauthFlowPayload = business.OAuthFlowPayload
type OAuthUserDeletedError = business.OAuthUserDeletedError
type OAuthRegistrationDisabledError = business.OAuthRegistrationDisabledError
type OAuthEmailAlreadyTakenError = business.OAuthEmailAlreadyTakenError
type OAuthLegacyBindingNotConfirmedError = business.OAuthLegacyBindingNotConfirmedError

func providerParams(name string) map[string]any { return map[string]any{"Provider": name} }

func GenerateOAuthCode(c *gin.Context) {
	var input oauthStateRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := input.Validate(); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	var identity *service.AuthIdentity
	var authorization *model.AuthFlowAuthorization
	success := false
	if input.Intent != model.AuthFlowIntentLogin {
		current, valid := middleware.GetSessionAuthIdentity(c)
		if !valid {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "绑定操作需要登录"})
			return
		}
		identity = &current
		if input.Intent == model.AuthFlowIntentBind {
			defer func() {
				recordUserSecurityAudit(c, current.UserID, "user.binding_start", map[string]any{"provider": input.Provider, "success": success})
			}()
			encoded, err := common.Marshal(service.AccountBindingContext{Provider: input.Provider})
			if err != nil {
				writeSecurityOperationError(c, err)
				return
			}
			authorization = middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeAccountBind, Context: encoded})
			if authorization == nil {
				return
			}
		}
	}
	result, err := business.StartOAuthFlow(c.Request.Context(), input, identity, authorization, oauth.LinuxDORedirectURI(c))
	if err != nil {
		if errors.Is(err, business.ErrOAuthProviderUnavailable) {
			common.ApiErrorI18n(c, i18n.MsgOAuthNotEnabled, providerParams("Linux DO"))
		} else {
			writeSecurityOperationError(c, err)
		}
		return
	}
	success = true
	if input.Intent == model.AuthFlowIntentLogin {
		writeOAuthBrowserBinding(c, result.FlowToken, result.ExpiresAt)
	}
	common.ApiSuccess(c, result)
}

func handleOAuthVerification(c *gin.Context, provider string, remote *oauth.OAuthUser, flow *model.AuthFlow) {
	var payload oauthFlowPayload
	if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	identity, valid := middleware.GetSessionAuthIdentity(c)
	if !valid {
		writeSecurityOperationError(c, service.ErrAuthTokenInvalid)
		return
	}
	proof, err := service.FinishOAuthVerification(identity, provider, remote.ProviderUserID, payload.Verification)
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	recordUserSecurityAudit(c, identity.UserID, "user.security_verify", map[string]any{"method": proof.Method, "scope": proof.Scope, "provider": provider})
	common.ApiSuccess(c, proof)
}

func findOrCreateOAuthUser(c *gin.Context, provider oauth.Provider, remote *oauth.OAuthUser, token *oauth.OAuthToken, affiliate string, admission ...*model.FurtherRegistrationAdmission) (*model.User, *service.LegacyGitHubMigration, error) {
	return business.ResolveOAuthLogin(c.Request.Context(), business.OAuthAccountRequest{Provider: provider, Remote: remote, Token: token, AffiliateCode: affiliate, Admissions: admission, MigrationAudit: func(user *model.User, success bool, params map[string]any) {
		recordLegacyGitHubBindingAudit(c, user, success, params)
	}})
}

func handleOAuthLogin(c *gin.Context, provider oauth.Provider, remote *oauth.OAuthUser, token *oauth.OAuthToken, flow *model.AuthFlow) {
	var payload oauthFlowPayload
	if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	account, migration, err := findOrCreateOAuthUser(c, provider, remote, token, payload.AffiliateCode, payload.RegistrationAdmission)
	if err != nil {
		switch err.(type) {
		case *OAuthUserDeletedError:
			common.ApiErrorI18n(c, i18n.MsgOAuthUserDeleted)
		case *OAuthRegistrationDisabledError:
			common.ApiErrorI18n(c, i18n.MsgUserRegisterDisabled)
		case *OAuthEmailAlreadyTakenError:
			common.ApiErrorI18n(c, i18n.MsgUserEmailAlreadyTaken)
		case *OAuthLegacyBindingNotConfirmedError:
			common.ApiErrorI18n(c, i18n.MsgOAuthNotAutoLinked, providerParams(provider.GetName()))
		default:
			writeSecurityOperationError(c, err)
		}
		return
	}
	if account.Status != common.UserStatusEnabled {
		common.ApiErrorI18n(c, i18n.MsgOAuthUserBanned)
		return
	}
	setupLogin(account, migration, c)
}

func handleOAuthBind(c *gin.Context, name string, provider oauth.Provider, remote *oauth.OAuthUser, flow *model.AuthFlow, state string, match model.AuthFlowMatch) (bool, bool) {
	identity, valid := middleware.GetSessionAuthIdentity(c)
	if !valid {
		writeSecurityOperationError(c, service.ErrAuthTokenInvalid)
		return false, false
	}
	var payload oauthFlowPayload
	if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil {
		writeSecurityOperationError(c, model.ErrAuthFlowInvalid)
		return false, false
	}
	success, err := business.BindOAuthAccount(c.Request.Context(), identity, name, provider, remote.ProviderUserID, state, match, payload)
	if err != nil {
		if errors.Is(err, business.ErrOAuthAlreadyBound) {
			common.ApiErrorI18n(c, i18n.MsgOAuthAlreadyBound, providerParams(provider.GetName()))
		} else {
			writeSecurityOperationError(c, err)
		}
		return false, false
	}
	account, err := model.GetUserByIdContext(c.Request.Context(), identity.UserID, false)
	if err != nil {
		writeSecurityOperationError(c, err)
		return success, true
	}
	warning := service.NotifyAccountSecurityChange(account.Email, "Login account linked: "+provider.GetName()) != nil
	common.ApiSuccessI18n(c, i18n.MsgOAuthBindSuccess, gin.H{"action": "bind", "notification_warning": warning})
	return success, warning
}

func recordLegacyGitHubBindingAudit(c *gin.Context, user *model.User, success bool, params map[string]any) {
	params["provider"], params["legacy_migration"], params["success"] = "github", true, success
	model.RecordOperationAuditLog(user.Id, user.Role, auditContentEN("user.binding_bind", params), c.ClientIP(), "user.binding_bind", params, nil, &model.AuditRequestInfo{Method: c.Request.Method, Route: c.FullPath(), Path: c.FullPath(), Status: c.Writer.Status(), Success: success}, c)
}

func handleOAuthError(c *gin.Context, err error) {
	switch failure := err.(type) {
	case *oauth.OAuthError:
		common.ApiErrorI18n(c, failure.MsgKey, failure.Params)
	case *oauth.AccessDeniedError:
		common.ApiErrorMsg(c, failure.Message)
	case *oauth.TrustLevelError:
		common.ApiErrorI18n(c, i18n.MsgOAuthTrustLevelLow)
	default:
		writeSecurityOperationError(c, err)
	}
}
