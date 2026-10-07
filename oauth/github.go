package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/i18n"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/gin-gonic/gin"
)

func init() {
	Register("github", &GitHubProvider{})
}

// GitHubProvider implements OAuth for GitHub
type GitHubProvider struct{}

type gitHubOAuthResponse struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
	TokenType   string `json:"token_type"`
}

type gitHubUser struct {
	Id    int64  `json:"id"`    // GitHub numeric ID (permanent, never changes)
	Login string `json:"login"` // GitHub username (can be changed by user)
	Name  string `json:"name"`
	Email string `json:"email"`
}

type gitHubEmail struct {
	Email    string `json:"email"`
	Verified bool   `json:"verified"`
}

func (p *GitHubProvider) GetName() string {
	return "GitHub"
}

func (p *GitHubProvider) IsEnabled() bool {
	return common.GitHubOAuthEnabled
}

func (p *GitHubProvider) ExchangeToken(ctx context.Context, code string, c *gin.Context) (*OAuthToken, error) {
	if code == "" {
		return nil, NewOAuthError(i18n.MsgOAuthInvalidCode, nil)
	}

	values := map[string]string{
		"client_id":     common.GitHubClientId,
		"client_secret": common.GitHubClientSecret,
		"code":          code,
	}
	jsonData, err := json.Marshal(values)
	if err != nil {
		return nil, providerFailure(ctx, "github", "token", i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "GitHub"}, err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://github.com/login/oauth/access_token", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, providerFailure(ctx, "github", "token", i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "GitHub"}, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := http.Client{
		Timeout: 20 * time.Second,
	}
	res, err := doOAuthRequest(ctx, &client, req)
	if err != nil {
		return nil, providerFailure(ctx, "github", "token", i18n.MsgOAuthConnectFailed, map[string]any{"Provider": "GitHub"}, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, providerFailure(ctx, "github", "token", i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "GitHub"}, errOAuthResponseInvalid)
	}

	var oAuthResponse gitHubOAuthResponse
	err = decodeOAuthResponse(ctx, res, &oAuthResponse)
	if err != nil {
		return nil, providerFailure(ctx, "github", "token", i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "GitHub"}, err)
	}

	if oAuthResponse.AccessToken == "" {
		return nil, NewOAuthError(i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "GitHub"})
	}

	return &OAuthToken{
		AccessToken: oAuthResponse.AccessToken,
		TokenType:   oAuthResponse.TokenType,
		Scope:       oAuthResponse.Scope,
	}, nil
}

func (p *GitHubProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, providerFailure(ctx, "github", "userinfo", i18n.MsgOAuthGetUserErr, map[string]any{"Provider": "GitHub"}, err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))

	client := http.Client{
		Timeout: 20 * time.Second,
	}
	res, err := doOAuthRequest(ctx, &client, req)
	if err != nil {
		return nil, providerFailure(ctx, "github", "userinfo", i18n.MsgOAuthConnectFailed, map[string]any{"Provider": "GitHub"}, err)
	}
	defer res.Body.Close()

	// Check for non-200 status codes before attempting to decode
	if res.StatusCode != http.StatusOK {
		return nil, providerFailure(ctx, "github", "userinfo", i18n.MsgOAuthGetUserErr, map[string]any{"Provider": "GitHub"}, errOAuthResponseInvalid)
	}

	var githubUser gitHubUser
	err = decodeOAuthResponse(ctx, res, &githubUser)
	if err != nil {
		return nil, providerFailure(ctx, "github", "userinfo", i18n.MsgOAuthGetUserErr, map[string]any{"Provider": "GitHub"}, err)
	}

	if githubUser.Id == 0 || githubUser.Login == "" {
		return nil, NewOAuthError(i18n.MsgOAuthUserInfoEmpty, map[string]any{"Provider": "GitHub"})
	}

	return &OAuthUser{
		ProviderUserID: strconv.FormatInt(githubUser.Id, 10), // Use numeric ID as primary identifier
		Username:       githubUser.Login,
		DisplayName:    githubUser.Name,
		Email:          githubUser.Email,
		Extra: map[string]any{
			"legacy_id": githubUser.Login, // Store login for migration from old accounts
		},
	}, nil
}

// GetVerifiedEmails lists the addresses GitHub has confirmed for the signed-in
// account. The email field of the user endpoint carries no confirmation status
// and is not used for this.
func (p *GitHubProvider) GetVerifiedEmails(ctx context.Context, token *OAuthToken) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return nil, providerFailure(ctx, "github", "emails", i18n.MsgOAuthGetUserErr, map[string]any{"Provider": "GitHub"}, err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	client := http.Client{
		Timeout: 20 * time.Second,
	}
	res, err := doOAuthRequest(ctx, &client, req)
	if err != nil {
		return nil, providerFailure(ctx, "github", "emails", i18n.MsgOAuthConnectFailed, map[string]any{"Provider": "GitHub"}, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, providerFailure(ctx, "github", "emails", i18n.MsgOAuthGetUserErr, map[string]any{"Provider": "GitHub"}, errOAuthResponseInvalid)
	}

	var emails []gitHubEmail
	if err := decodeOAuthResponse(ctx, res, &emails); err != nil {
		return nil, providerFailure(ctx, "github", "emails", i18n.MsgOAuthGetUserErr, map[string]any{"Provider": "GitHub"}, err)
	}
	verified := make([]string, 0, len(emails))
	for _, email := range emails {
		if email.Verified && email.Email != "" {
			verified = append(verified, email.Email)
		}
	}
	return verified, nil
}

func (p *GitHubProvider) IsUserIDTaken(providerUserID string) bool {
	return model.IsGitHubIdAlreadyTaken(providerUserID)
}

func (p *GitHubProvider) FillUserByProviderID(user *model.User, providerUserID string) error {
	user.GitHubId = providerUserID
	return user.FillUserByGitHubId()
}

func (p *GitHubProvider) SetProviderUserID(user *model.User, providerUserID string) {
	user.GitHubId = providerUserID
}

func (p *GitHubProvider) GetProviderPrefix() string {
	return "github_"
}

// ProviderUserIDColumn returns the users-table column storing this provider's user ID.
func (p *GitHubProvider) ProviderUserIDColumn() string {
	return "github_id"
}
