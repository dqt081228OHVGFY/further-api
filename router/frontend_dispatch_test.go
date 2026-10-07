package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestFrontendDispatchUsesSectionEntrypoints(t *testing.T) {
	previous := common.GlobalWebRateLimitEnable
	common.GlobalWebRateLimitEnable = false
	t.Cleanup(func() { common.GlobalWebRateLimitEnable = previous })
	assets := fstest.MapFS{
		"web/dist/index.html":         {Data: []byte("home")},
		"web/dist/admin/index.html":   {Data: []byte("admin")},
		"web/dist/console/index.html": {Data: []byte("console")},
		"web/dist/prepaid/index.html": {Data: []byte("prepaid")},
		"web/dist/auth/index.html":    {Data: []byte(`<body data-page="auth">login</body>`)},
		"web/dist/gateway.html":       {Data: []byte("retired gateway")},
	}
	engine := gin.New()
	SetWebRouter(engine, WebAssets{BuildFS: assets, IndexPage: []byte("home with analytics")}, nil)
	for _, tc := range []struct{ path, body string }{
		{"/", "home with analytics"}, {"/admin/users", "admin"},
		{"/console/tokens", "console"}, {"/prepaid/redeem", "prepaid"},
		{"/auth/callback", "login"}, {"/sign-in", "login"},
		{"/sign-up", `data-auth-mode="register"`}, {"/admin/index.html", "admin"},
		{"/apiculture", "home with analytics"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			response := webResourceRequest(engine, http.MethodGet, tc.path, freshWebResourceTestIP())
			assert.Equal(t, http.StatusOK, response.Code)
			assert.Contains(t, response.Body.String(), tc.body)
			assert.Equal(t, "no-cache", response.Header().Get("Cache-Control"))
		})
	}
	for _, requestPath := range []string{"/gateway.html", "/missing.html", "/api/missing", "/v1beta/missing", "/dashboard/billing/missing", "/fast/mj/missing", "/further-assets/missing.css", "/admin/../console"} {
		response := webResourceRequest(engine, http.MethodGet, requestPath, freshWebResourceTestIP())
		assert.Equal(t, http.StatusNotFound, response.Code, requestPath)
		assert.Contains(t, response.Header().Get("Cache-Control"), "no-store", requestPath)
		assert.NotContains(t, response.Body.String(), "retired gateway")
	}
	head := webResourceRequest(engine, http.MethodHead, "/admin/users", freshWebResourceTestIP())
	assert.Equal(t, http.StatusOK, head.Code)
	assert.Empty(t, head.Body.String())
}

func TestMissingSectionDoesNotFallBackToHome(t *testing.T) {
	previous := common.GlobalWebRateLimitEnable
	common.GlobalWebRateLimitEnable = false
	t.Cleanup(func() { common.GlobalWebRateLimitEnable = previous })
	engine := gin.New()
	SetWebRouter(engine, WebAssets{IndexPage: []byte("home")}, nil)
	response := webResourceRequest(engine, http.MethodGet, "/admin/users", freshWebResourceTestIP())
	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.Contains(t, response.Body.String(), "FRONTEND_UNAVAILABLE")
	assert.NotContains(t, response.Body.String(), "home")
}

func TestRemoteFrontendDispatchPreservesBackendFailures(t *testing.T) {
	engine := gin.New()
	engine.NoRoute(redirectFrontend("https://frontend.example/app/"))
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "/console/tokens?q=hello%20world", http.StatusTemporaryRedirect},
		{http.MethodHead, "/auth", http.StatusTemporaryRedirect},
		{http.MethodGet, "/api/missing", http.StatusNotFound},
		{http.MethodGet, "/v1beta/missing", http.StatusNotFound},
		{http.MethodGet, "/fast/mj/missing", http.StatusNotFound},
		{http.MethodGet, "/dashboard/billing/missing", http.StatusNotFound},
		{http.MethodGet, "/gateway.html", http.StatusNotFound},
		{http.MethodPost, "/console", http.StatusNotFound},
	} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
		assert.Equal(t, tc.status, response.Code, tc.method+" "+tc.path)
		if tc.status == http.StatusTemporaryRedirect {
			assert.Equal(t, "https://frontend.example/app"+tc.path, response.Header().Get("Location"))
		} else {
			assert.Empty(t, response.Header().Get("Location"))
			assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
		}
	}
	for _, base := range []string{"javascript:alert(1)", "//frontend.example", "https://user:secret@frontend.example", "https://frontend.example/?override=1"} {
		invalid := gin.New()
		invalid.NoRoute(redirectFrontend(base))
		response := httptest.NewRecorder()
		invalid.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", strings.NewReader("")))
		assert.Equal(t, http.StatusServiceUnavailable, response.Code, base)
		assert.Empty(t, response.Header().Get("Location"))
	}
}
