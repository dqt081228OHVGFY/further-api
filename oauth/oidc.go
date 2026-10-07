package oauth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/i18n"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

func init() {
	Register("oidc", &OIDCProvider{})
}

// OIDCProvider implements OAuth for OIDC
type OIDCProvider struct{}

type oidcOAuthResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

type oidcUser struct {
	OpenID            string `json:"sub"`
	Email             string `json:"email"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
	Picture           string `json:"picture"`
}

func (p *OIDCProvider) GetName() string {
	return system_setting.GetOIDCSettings().GetEffectiveDisplayName()
}

func (p *OIDCProvider) IsEnabled() bool {
	return system_setting.GetOIDCSettings().Enabled
}

func (p *OIDCProvider) ExchangeToken(ctx context.Context, code string, c *gin.Context) (*OAuthToken, error) {
	if code == "" {
		return nil, NewOAuthError(i18n.MsgOAuthInvalidCode, nil)
	}

	settings := system_setting.GetOIDCSettings()
	redirectUri := fmt.Sprintf("%s/oauth/oidc", system_setting.ServerAddress)
	values := url.Values{}
	values.Set("client_id", settings.ClientId)
	values.Set("client_secret", settings.ClientSecret)
	values.Set("code", code)
	values.Set("grant_type", "authorization_code")
	values.Set("redirect_uri", redirectUri)

	req, err := http.NewRequestWithContext(ctx, "POST", settings.TokenEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, providerFailure(ctx, "oidc", "token", i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "OIDC"}, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := http.Client{
		Timeout: 5 * time.Second,
	}
	res, err := doOAuthRequest(ctx, &client, req)
	if err != nil {
		return nil, providerFailure(ctx, "oidc", "token", i18n.MsgOAuthConnectFailed, map[string]any{"Provider": "OIDC"}, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, providerFailure(ctx, "oidc", "token", i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "OIDC"}, errOAuthResponseInvalid)
	}

	var oidcResponse oidcOAuthResponse
	err = decodeOAuthResponse(ctx, res, &oidcResponse)
	if err != nil {
		return nil, providerFailure(ctx, "oidc", "token", i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "OIDC"}, err)
	}

	if oidcResponse.AccessToken == "" {
		return nil, NewOAuthError(i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "OIDC"})
	}

	return &OAuthToken{
		AccessToken:  oidcResponse.AccessToken,
		TokenType:    oidcResponse.TokenType,
		RefreshToken: oidcResponse.RefreshToken,
		ExpiresIn:    oidcResponse.ExpiresIn,
		Scope:        oidcResponse.Scope,
		IDToken:      oidcResponse.IDToken,
	}, nil
}

func (p *OIDCProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error) {
	settings := system_setting.GetOIDCSettings()

	req, err := http.NewRequestWithContext(ctx, "GET", settings.UserInfoEndpoint, nil)
	if err != nil {
		return nil, providerFailure(ctx, "oidc", "userinfo", i18n.MsgOAuthGetUserErr, map[string]any{"Provider": "OIDC"}, err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	client := http.Client{
		Timeout: 5 * time.Second,
	}
	res, err := doOAuthRequest(ctx, &client, req)
	if err != nil {
		return nil, providerFailure(ctx, "oidc", "userinfo", i18n.MsgOAuthConnectFailed, map[string]any{"Provider": "OIDC"}, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, NewOAuthError(i18n.MsgOAuthGetUserErr, nil)
	}

	var oidcUser oidcUser
	err = decodeOAuthResponse(ctx, res, &oidcUser)
	if err != nil {
		return nil, providerFailure(ctx, "oidc", "userinfo", i18n.MsgOAuthGetUserErr, map[string]any{"Provider": "OIDC"}, err)
	}

	if oidcUser.OpenID == "" || oidcUser.Email == "" {
		return nil, NewOAuthError(i18n.MsgOAuthUserInfoEmpty, map[string]any{"Provider": "OIDC"})
	}

	return &OAuthUser{
		ProviderUserID: oidcUser.OpenID,
		Username:       oidcUser.PreferredUsername,
		DisplayName:    oidcUser.Name,
		Email:          oidcUser.Email,
	}, nil
}

func (p *OIDCProvider) IsUserIDTaken(providerUserID string) bool {
	return model.IsOidcIdAlreadyTaken(providerUserID)
}

func (p *OIDCProvider) FillUserByProviderID(user *model.User, providerUserID string) error {
	user.OidcId = providerUserID
	return user.FillUserByOidcId()
}

func (p *OIDCProvider) SetProviderUserID(user *model.User, providerUserID string) {
	user.OidcId = providerUserID
}

func (p *OIDCProvider) GetProviderPrefix() string {
	return "oidc_"
}

// ProviderUserIDColumn returns the users-table column storing this provider's user ID.
func (p *OIDCProvider) ProviderUserIDColumn() string {
	return "oidc_id"
}
