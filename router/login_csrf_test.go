package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEveryBrowserLoginEntrypointRejectsCrossSiteAndMissingOrigin(t *testing.T) {
	previousSecure, previousOrigins := common.SessionCookieSecure, common.SessionCookieTrustedURLs
	common.SessionCookieSecure, common.SessionCookieTrustedURLs = true, []string{"https://gateway.example.com"}
	t.Cleanup(func() { common.SessionCookieSecure, common.SessionCookieTrustedURLs = previousSecure, previousOrigins })
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodPost, "/api/setup"},
		{http.MethodPost, "/api/user/login"},
		{http.MethodPost, "/api/user/login/2fa"},
		{http.MethodPost, "/api/user/login/verify"},
		{http.MethodPost, "/api/user/login/passkey/finish"},
		{http.MethodPost, "/api/user/passkey/login/finish"},
		{http.MethodPost, "/api/oauth/state"},
		{http.MethodGet, "/api/oauth/wechat?code=attacker-valid-code"},
	} {
		for _, origin := range []string{"https://attacker.example.com", "null", ""} {
			t.Run(endpoint.path+"/"+origin, func(t *testing.T) {
				// A top-level text/plain form may encode JSON by placing the
				// form's '=' inside an ignored string field. Reject it before
				// credential/code exchange or any session issuance.
				request := httptest.NewRequest(endpoint.method, "https://gateway.example.com"+endpoint.path, strings.NewReader(`{"username":"attacker","password":"attacker-password","confirmPassword":"attacker-password","ignored":"=x"}`))
				request.Header.Set("Content-Type", "text/plain")
				if origin != "" {
					request.Header.Set("Origin", origin)
				}
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				require.Equal(t, http.StatusForbidden, response.Code)
				require.Contains(t, response.Body.String(), "AUTH_ORIGIN_FORBIDDEN")
				require.Empty(t, response.Header().Values("Set-Cookie"))
			})
		}
	}
}

func TestHTTPBrowserLoginEntrypointsRejectCrossSiteOrigin(t *testing.T) {
	previousSecure, previousOrigins := common.SessionCookieSecure, common.SessionCookieTrustedURLs
	common.SessionCookieSecure, common.SessionCookieTrustedURLs = false, nil
	t.Cleanup(func() { common.SessionCookieSecure, common.SessionCookieTrustedURLs = previousSecure, previousOrigins })
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)
	for _, path := range []string{
		"/api/setup",
		"/api/user/login", "/api/user/login/2fa", "/api/user/login/verify",
		"/api/user/login/passkey/finish", "/api/user/passkey/login/finish",
		"/api/oauth/state", "/api/user/auth/refresh", "/api/user/auth/logout",
	} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://localhost:3000"+path,
				strings.NewReader(`{"username":"attacker","password":"attacker-password","confirmPassword":"attacker-password","ignored":"=x"}`))
			request.Header.Set("Content-Type", "text/plain")
			request.Header.Set("Origin", "https://attacker.example.com")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			require.Equal(t, http.StatusForbidden, response.Code)
			require.Contains(t, response.Body.String(), "AUTH_ORIGIN_FORBIDDEN")
			require.Empty(t, response.Header().Values("Set-Cookie"))
		})
	}
}
