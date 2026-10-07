package business

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/oauth"
	"github.com/dqt081228OHVGFY/further-api/service"
	"net/url"
	"strings"
	"time"
)

var ErrOAuthInput = errors.New("OAuth flow request is invalid")
var ErrOAuthProviderUnavailable = errors.New("OAuth provider is unavailable")

type OAuthFlowRequest struct {
	Provider         string          `json:"provider"`
	Intent           string          `json:"intent"`
	Aff              string          `json:"aff,omitempty"`
	Scope            string          `json:"scope,omitempty"`
	Context          json.RawMessage `json:"context,omitempty"`
	RegistrationCode string          `json:"registration_code,omitempty"`
}

func (request *OAuthFlowRequest) Validate() error {
	request.Provider, request.Intent, request.Aff = strings.TrimSpace(request.Provider), strings.TrimSpace(request.Intent), strings.TrimSpace(request.Aff)
	if oauth.GetProvider(request.Provider) == nil || len(request.Aff) > 32 || len(request.RegistrationCode) > 128 {
		return ErrOAuthInput
	}
	switch request.Intent {
	case model.AuthFlowIntentLogin:
		if request.Scope != "" || len(request.Context) > 0 || request.RegistrationCode != "" && request.Provider != "linuxdo" {
			return ErrOAuthInput
		}
	case model.AuthFlowIntentBind:
		if request.Aff != "" || request.RegistrationCode != "" || request.Scope != "" || len(request.Context) > 0 {
			return ErrOAuthInput
		}
	case model.AuthFlowIntentVerify:
		if request.Aff != "" || request.RegistrationCode != "" {
			return ErrOAuthInput
		}
	default:
		return ErrOAuthInput
	}
	return nil
}

type OAuthFlowPayload struct {
	AffiliateCode         string                              `json:"affiliate_code,omitempty"`
	Verification          *service.OAuthVerificationFlow      `json:"verification,omitempty"`
	Telegram              *oauth.TelegramOAuthFlow            `json:"telegram,omitempty"`
	SessionIdentity       *service.AuthIdentity               `json:"session_identity,omitempty"`
	Authorization         *model.AuthFlowAuthorization        `json:"authorization,omitempty"`
	RegistrationAdmission *model.FurtherRegistrationAdmission `json:"registration_admission,omitempty"`
}

type OAuthFlowResult struct {
	FlowToken        string `json:"flow_token"`
	ExpiresAt        int64  `json:"expires_at"`
	AuthorizationURL string `json:"authorization_url,omitempty"`
}

func StartOAuthFlow(ctx context.Context, request OAuthFlowRequest, identity *service.AuthIdentity, authorization *model.AuthFlowAuthorization, redirectURI string) (*OAuthFlowResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if request.Provider == "linuxdo" && request.Intent == model.AuthFlowIntentLogin && (!common.LinuxDOOAuthEnabled || common.LinuxDOClientId == "" || common.LinuxDOClientSecret == "") {
		return nil, ErrOAuthProviderUnavailable
	}
	payload := OAuthFlowPayload{AffiliateCode: request.Aff}
	userID, sessionID := 0, ""
	if request.Intent != model.AuthFlowIntentLogin {
		if identity == nil {
			return nil, service.ErrAuthTokenInvalid
		}
		userID, sessionID = identity.UserID, identity.SessionID
		if request.Intent == model.AuthFlowIntentBind {
			bindingContext, err := common.Marshal(service.AccountBindingContext{Provider: request.Provider})
			if err != nil {
				return nil, err
			}
			if err := service.ValidateFlowAuthorization(*identity, service.VerificationOperation{Scope: service.VerificationScopeAccountBind, Context: bindingContext}, authorization); err != nil {
				return nil, err
			}
			payload.Authorization, payload.SessionIdentity = authorization, identity
		} else {
			verification, err := service.StartOAuthVerification(*identity, service.VerificationOperation{Scope: request.Scope, Context: request.Context}, request.Provider)
			if err != nil {
				return nil, err
			}
			payload.Verification = verification
		}
	}
	if request.Provider == "telegram" {
		telegram, err := oauth.NewTelegramOAuthFlow()
		if err != nil {
			return nil, err
		}
		payload.Telegram = telegram
		if identity != nil {
			if _, _, err := service.ValidateLoginSession(*identity); err != nil {
				return nil, err
			}
			payload.SessionIdentity = identity
		}
	}
	if request.RegistrationCode != "" {
		admission, err := model.PrepareFurtherRegistrationAdmission(request.RegistrationCode)
		if err != nil {
			return nil, err
		}
		payload.RegistrationAdmission = admission
	}
	var endpoint *url.URL
	if request.Provider == "linuxdo" && request.Intent == model.AuthFlowIntentLogin {
		var err error
		endpoint, err = url.Parse(common.GetEnvOrDefaultString("LINUX_DO_AUTHORIZATION_ENDPOINT", "https://connect.linux.do/oauth2/authorize"))
		if err != nil {
			return nil, err
		}
	}
	encoded, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	expires := time.Now().Add(10 * time.Minute)
	token, _, err := model.CreateAuthFlowContext(ctx, model.AuthFlowCreate{Purpose: model.AuthFlowPurposeOAuth, Provider: request.Provider, Intent: request.Intent, UserId: userID, SessionId: sessionID, Payload: string(encoded), ExpiresAt: expires})
	if err != nil {
		return nil, err
	}
	result := &OAuthFlowResult{FlowToken: token, ExpiresAt: expires.Unix()}
	if endpoint != nil {
		query := endpoint.Query()
		query.Set("response_type", "code")
		query.Set("client_id", common.LinuxDOClientId)
		query.Set("redirect_uri", redirectURI)
		query.Set("state", token)
		endpoint.RawQuery = query.Encode()
		result.AuthorizationURL = endpoint.String()
	}
	if payload.Telegram != nil {
		result.AuthorizationURL = payload.Telegram.AuthorizationURL(token)
	}
	return result, nil
}
