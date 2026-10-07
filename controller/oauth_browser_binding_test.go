package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/oauth"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func enableBrowserCookieSecurityFixture(t *testing.T) {
	t.Helper()
	previousSecure, previousOrigins := common.SessionCookieSecure, common.SessionCookieTrustedURLs
	common.SessionCookieSecure, common.SessionCookieTrustedURLs = true, []string{"https://example.com"}
	t.Cleanup(func() { common.SessionCookieSecure, common.SessionCookieTrustedURLs = previousSecure, previousOrigins })
}

func TestPasswordLoginAcceptsSameOriginButRejectsCrossSiteTextForm(t *testing.T) {
	user, _ := setupSecurityEnrollmentTest(t)
	enableBrowserCookieSecurityFixture(t)
	previousPassword := common.PasswordLoginEnabled
	common.PasswordLoginEnabled = true
	t.Cleanup(func() { common.PasswordLoginEnabled = previousPassword })
	engine := gin.New()
	engine.POST("/api/user/login", middleware.SessionCookieOriginGuard(), Login)
	for _, origin := range []string{"https://example.com", "https://attacker.example.com"} {
		request := httptest.NewRequest(http.MethodPost, "https://example.com/api/user/login", strings.NewReader(`{"username":"enrollment-user","password":"enrollment-password","ignored":"=x"}`))
		request.Header.Set("Origin", origin)
		request.Header.Set("Content-Type", "text/plain")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		if origin == "https://example.com" {
			require.Equal(t, http.StatusOK, response.Code)
			require.Contains(t, response.Body.String(), `"access_token"`)
			var refresh *http.Cookie
			for _, cookie := range response.Result().Cookies() {
				if cookie.Name == service.RefreshCookieName {
					refresh = cookie
				}
			}
			require.NotNil(t, refresh)
			require.True(t, refresh.HttpOnly && refresh.Secure)
		} else {
			require.Equal(t, http.StatusForbidden, response.Code)
			require.Empty(t, response.Header().Values("Set-Cookie"))
		}
	}
	count, err := model.CountActiveUserSessions(user.Id, time.Now().Unix())
	require.NoError(t, err)
	require.EqualValues(t, 2, count, "the cross-site request must not issue a session")
}

func TestOAuthLoginStateCannotBeTransferredToAnotherBrowser(t *testing.T) {
	for _, secure := range []bool{false, true} {
		name := "HTTP"
		if secure {
			name = "HTTPS"
		}
		t.Run(name, func(t *testing.T) { testOAuthBrowserBinding(t, secure) })
	}
}

func testOAuthBrowserBinding(t *testing.T, secure bool) {
	user, _ := setupSecurityEnrollmentTest(t)
	enableBrowserCookieSecurityFixture(t)
	common.SessionCookieSecure = secure
	provider := &boundLoginOAuthProvider{userID: user.Id}
	oauth.Register("browser-binding-fixture", provider)
	t.Cleanup(func() { oauth.Unregister("browser-binding-fixture") })
	engine := gin.New()
	engine.POST("/api/oauth/state", middleware.SessionCookieOriginGuard(), GenerateOAuthCode)
	engine.GET("/api/oauth/:provider", HandleOAuth)
	type initiated struct {
		state  string
		cookie *http.Cookie
	}
	initiate := func() initiated {
		request := httptest.NewRequest(http.MethodPost, "https://example.com/api/oauth/state", strings.NewReader(`{"provider":"browser-binding-fixture","intent":"login"}`))
		request.Header.Set("Origin", "https://example.com")
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		var result struct {
			Success bool `json:"success"`
			Data    struct {
				FlowToken string `json:"flow_token"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		require.True(t, result.Success, response.Body.String())
		cookies := response.Result().Cookies()
		require.Len(t, cookies, 1)
		require.Equal(t, secure, cookies[0].Secure)
		require.True(t, cookies[0].HttpOnly)
		require.Equal(t, http.SameSiteLaxMode, cookies[0].SameSite)
		require.Equal(t, "/", cookies[0].Path)
		require.Empty(t, cookies[0].Domain)
		require.Equal(t, secure, strings.HasPrefix(cookies[0].Name, "__Host-"))
		return initiated{result.Data.FlowToken, cookies[0]}
	}
	first, second := initiate(), initiate()
	require.NotEqual(t, first.cookie.Name, second.cookie.Name, "parallel login popups must not overwrite each other's binding")
	callback := func(flow initiated, cookie *http.Cookie) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "https://example.com/api/oauth/browser-binding-fixture?state="+flow.state+"&code=provider-code", nil)
		if cookie != nil {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response
	}
	wrong := *first.cookie
	wrong.Value = "forged-value"
	for _, cookie := range []*http.Cookie{nil, second.cookie, &wrong} {
		response := callback(first, cookie)
		require.Equal(t, http.StatusForbidden, response.Code)
		require.Zero(t, provider.exchangeCalls, "a transferred state must be rejected before contacting its provider")
		_, err := model.GetAuthFlow(first.state, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth, Provider: "browser-binding-fixture", Intent: model.AuthFlowIntentLogin})
		require.NoError(t, err, "another browser must not consume the legitimate flow")
	}
	for _, flow := range []initiated{first, second} {
		response := callback(flow, flow.cookie)
		require.Equal(t, http.StatusOK, response.Code)
		require.Contains(t, response.Body.String(), `"access_token"`)
		cleared := false
		for _, cookie := range response.Result().Cookies() {
			if cookie.Name == flow.cookie.Name && cookie.MaxAge < 0 {
				cleared = true
			}
		}
		require.True(t, cleared, "successful callback must clear only its own binding")
	}
	require.Equal(t, 2, provider.exchangeCalls)
	require.Equal(t, http.StatusForbidden, callback(first, first.cookie).Code, "a consumed provider flow remains single use")
}
