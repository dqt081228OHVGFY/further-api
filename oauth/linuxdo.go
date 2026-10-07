package oauth

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/i18n"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

func init() {
	Register("linuxdo", &LinuxDOProvider{})
}

// LinuxDOProvider implements OAuth for Linux DO
type LinuxDOProvider struct{}

type linuxdoUser struct {
	Id         int    `json:"id"`
	Username   string `json:"username"`
	Name       string `json:"name"`
	Active     bool   `json:"active"`
	TrustLevel int    `json:"trust_level"`
	Silenced   bool   `json:"silenced"`
}

func (p *LinuxDOProvider) GetName() string {
	return "Linux DO"
}

func (p *LinuxDOProvider) IsEnabled() bool {
	return common.LinuxDOOAuthEnabled
}

func LinuxDORedirectURI(c *gin.Context) string {
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	if base == "" || base == "http://localhost:3000" {
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
		base = scheme + "://" + c.Request.Host
	}
	return base + "/api/oauth/linuxdo"
}

func (p *LinuxDOProvider) ExchangeToken(ctx context.Context, code string, c *gin.Context) (*OAuthToken, error) {
	if code == "" {
		return nil, NewOAuthError(i18n.MsgOAuthInvalidCode, nil)
	}

	// Get access token using Basic auth
	tokenEndpoint := common.GetEnvOrDefaultString("LINUX_DO_TOKEN_ENDPOINT", "https://connect.linux.do/oauth2/token")
	credentials := common.LinuxDOClientId + ":" + common.LinuxDOClientSecret
	basicAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(credentials))

	redirectURI := LinuxDORedirectURI(c)

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, providerFailure(ctx, "linuxdo", "token", i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "Linux DO"}, err)
	}
	req.Header.Set("Authorization", basicAuth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := http.Client{Timeout: 5 * time.Second}
	res, err := doOAuthRequest(ctx, &client, req)
	if err != nil {
		return nil, providerFailure(ctx, "linuxdo", "token", i18n.MsgOAuthConnectFailed, map[string]any{"Provider": "Linux DO"}, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, providerFailure(ctx, "linuxdo", "token", i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "Linux DO"}, errOAuthResponseInvalid)
	}

	var tokenRes struct {
		AccessToken string `json:"access_token"`
		Message     string `json:"message"`
	}
	if err := decodeOAuthResponse(ctx, res, &tokenRes); err != nil {
		return nil, providerFailure(ctx, "linuxdo", "token", i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "Linux DO"}, err)
	}

	if tokenRes.AccessToken == "" {
		return nil, providerFailure(ctx, "linuxdo", "token", i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "Linux DO"}, errOAuthResponseInvalid)
	}

	return &OAuthToken{
		AccessToken: tokenRes.AccessToken,
	}, nil
}

func (p *LinuxDOProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error) {
	userEndpoint := common.GetEnvOrDefaultString("LINUX_DO_USER_ENDPOINT", "https://connect.linux.do/api/user")

	req, err := http.NewRequestWithContext(ctx, "GET", userEndpoint, nil)
	if err != nil {
		return nil, providerFailure(ctx, "linuxdo", "userinfo", i18n.MsgOAuthGetUserErr, map[string]any{"Provider": "Linux DO"}, err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/json")

	client := http.Client{Timeout: 5 * time.Second}
	res, err := doOAuthRequest(ctx, &client, req)
	if err != nil {
		return nil, providerFailure(ctx, "linuxdo", "userinfo", i18n.MsgOAuthConnectFailed, map[string]any{"Provider": "Linux DO"}, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, providerFailure(ctx, "linuxdo", "userinfo", i18n.MsgOAuthGetUserErr, map[string]any{"Provider": "Linux DO"}, errOAuthResponseInvalid)
	}

	var linuxdoUser linuxdoUser
	if err := decodeOAuthResponse(ctx, res, &linuxdoUser); err != nil {
		return nil, providerFailure(ctx, "linuxdo", "userinfo", i18n.MsgOAuthGetUserErr, map[string]any{"Provider": "Linux DO"}, err)
	}

	if linuxdoUser.Id == 0 {
		return nil, NewOAuthError(i18n.MsgOAuthUserInfoEmpty, map[string]any{"Provider": "Linux DO"})
	}

	// Check trust level
	if linuxdoUser.TrustLevel < common.LinuxDOMinimumTrustLevel {
		logger.LogWarn(ctx, "OAuth request rejected: provider=linuxdo stage=userinfo category=trust_level")
		return nil, &TrustLevelError{
			Required: common.LinuxDOMinimumTrustLevel,
			Current:  linuxdoUser.TrustLevel,
		}
	}

	return &OAuthUser{
		ProviderUserID: strconv.Itoa(linuxdoUser.Id),
		Username:       linuxdoUser.Username,
		DisplayName:    linuxdoUser.Name,
		Extra: map[string]any{
			"trust_level": linuxdoUser.TrustLevel,
			"active":      linuxdoUser.Active,
			"silenced":    linuxdoUser.Silenced,
		},
	}, nil
}

func (p *LinuxDOProvider) IsUserIDTaken(providerUserID string) bool {
	return model.IsLinuxDOIdAlreadyTaken(providerUserID)
}

func (p *LinuxDOProvider) FillUserByProviderID(user *model.User, providerUserID string) error {
	user.LinuxDOId = providerUserID
	return user.FillUserByLinuxDOId()
}

func (p *LinuxDOProvider) SetProviderUserID(user *model.User, providerUserID string) {
	user.LinuxDOId = providerUserID
}

func (p *LinuxDOProvider) GetProviderPrefix() string {
	return "linuxdo_"
}

// ProviderUserIDColumn returns the users-table column storing this provider's user ID.
func (p *LinuxDOProvider) ProviderUserIDColumn() string {
	return "linux_do_id"
}

// TrustLevelError indicates the user's trust level is too low
type TrustLevelError struct {
	Required int
	Current  int
}

func (e *TrustLevelError) Error() string {
	return "trust level too low"
}
