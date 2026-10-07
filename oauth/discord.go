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
	Register("discord", &DiscordProvider{})
}

// DiscordProvider implements OAuth for Discord
type DiscordProvider struct{}

type discordOAuthResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

type discordUser struct {
	UID  string `json:"id"`
	ID   string `json:"username"`
	Name string `json:"global_name"`
}

func (p *DiscordProvider) GetName() string {
	return "Discord"
}

func (p *DiscordProvider) IsEnabled() bool {
	return system_setting.GetDiscordSettings().Enabled
}

func (p *DiscordProvider) ExchangeToken(ctx context.Context, code string, c *gin.Context) (*OAuthToken, error) {
	if code == "" {
		return nil, NewOAuthError(i18n.MsgOAuthInvalidCode, nil)
	}

	settings := system_setting.GetDiscordSettings()
	redirectUri := fmt.Sprintf("%s/oauth/discord", system_setting.ServerAddress)
	values := url.Values{}
	values.Set("client_id", settings.ClientId)
	values.Set("client_secret", settings.ClientSecret)
	values.Set("code", code)
	values.Set("grant_type", "authorization_code")
	values.Set("redirect_uri", redirectUri)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://discord.com/api/v10/oauth2/token", strings.NewReader(values.Encode()))
	if err != nil {
		return nil, providerFailure(ctx, "discord", "token", i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "Discord"}, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := http.Client{
		Timeout: 5 * time.Second,
	}
	res, err := doOAuthRequest(ctx, &client, req)
	if err != nil {
		return nil, providerFailure(ctx, "discord", "token", i18n.MsgOAuthConnectFailed, map[string]any{"Provider": "Discord"}, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, providerFailure(ctx, "discord", "token", i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "Discord"}, errOAuthResponseInvalid)
	}

	var discordResponse discordOAuthResponse
	err = decodeOAuthResponse(ctx, res, &discordResponse)
	if err != nil {
		return nil, providerFailure(ctx, "discord", "token", i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "Discord"}, err)
	}

	if discordResponse.AccessToken == "" {
		return nil, NewOAuthError(i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "Discord"})
	}

	return &OAuthToken{
		AccessToken:  discordResponse.AccessToken,
		TokenType:    discordResponse.TokenType,
		RefreshToken: discordResponse.RefreshToken,
		ExpiresIn:    discordResponse.ExpiresIn,
		Scope:        discordResponse.Scope,
		IDToken:      discordResponse.IDToken,
	}, nil
}

func (p *DiscordProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://discord.com/api/v10/users/@me", nil)
	if err != nil {
		return nil, providerFailure(ctx, "discord", "userinfo", i18n.MsgOAuthGetUserErr, map[string]any{"Provider": "Discord"}, err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	client := http.Client{
		Timeout: 5 * time.Second,
	}
	res, err := doOAuthRequest(ctx, &client, req)
	if err != nil {
		return nil, providerFailure(ctx, "discord", "userinfo", i18n.MsgOAuthConnectFailed, map[string]any{"Provider": "Discord"}, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, NewOAuthError(i18n.MsgOAuthGetUserErr, nil)
	}

	var discordUser discordUser
	err = decodeOAuthResponse(ctx, res, &discordUser)
	if err != nil {
		return nil, providerFailure(ctx, "discord", "userinfo", i18n.MsgOAuthGetUserErr, map[string]any{"Provider": "Discord"}, err)
	}

	if discordUser.UID == "" || discordUser.ID == "" {
		return nil, NewOAuthError(i18n.MsgOAuthUserInfoEmpty, map[string]any{"Provider": "Discord"})
	}

	return &OAuthUser{
		ProviderUserID: discordUser.UID,
		Username:       discordUser.ID,
		DisplayName:    discordUser.Name,
	}, nil
}

func (p *DiscordProvider) IsUserIDTaken(providerUserID string) bool {
	return model.IsDiscordIdAlreadyTaken(providerUserID)
}

func (p *DiscordProvider) FillUserByProviderID(user *model.User, providerUserID string) error {
	user.DiscordId = providerUserID
	return user.FillUserByDiscordId()
}

func (p *DiscordProvider) SetProviderUserID(user *model.User, providerUserID string) {
	user.DiscordId = providerUserID
}

func (p *DiscordProvider) GetProviderPrefix() string {
	return "discord_"
}

// ProviderUserIDColumn returns the users-table column storing this provider's user ID.
func (p *DiscordProvider) ProviderUserIDColumn() string {
	return "discord_id"
}
