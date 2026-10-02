package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var webResourceTestIPSequence atomic.Uint32

func freshWebResourceTestIP() string {
	sequence := webResourceTestIPSequence.Add(1)
	return fmt.Sprintf("192.0.%d.%d", 2+sequence/250, 1+sequence%250)
}

func webResourceRateLimitRouter(t *testing.T) *gin.Engine {
	t.Helper()
	webEnabled, webMaximum, webDuration := common.GlobalWebRateLimitEnable, common.GlobalWebRateLimitNum, common.GlobalWebRateLimitDuration
	apiEnabled, apiMaximum, apiDuration := common.GlobalApiRateLimitEnable, common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration
	redisEnabled := common.RedisEnabled
	common.GlobalWebRateLimitEnable, common.GlobalWebRateLimitNum, common.GlobalWebRateLimitDuration = true, 1, 180
	common.GlobalApiRateLimitEnable, common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration = true, 1, 180
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.GlobalWebRateLimitEnable, common.GlobalWebRateLimitNum, common.GlobalWebRateLimitDuration = webEnabled, webMaximum, webDuration
		common.GlobalApiRateLimitEnable, common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration = apiEnabled, apiMaximum, apiDuration
		common.RedisEnabled = redisEnabled
	})
	assets := fstest.MapFS{
		"web/dist/index.html":                     {Data: []byte("<html>home</html>")},
		"web/dist/console/index.html":             {Data: []byte("<html>console</html>")},
		"web/dist/static/js/app.js":               {Data: []byte("window.applicationLoaded = true;")},
		"web/dist/relayfor-assets/app.css":        {Data: []byte("body { color: blue; }")},
		"web/dist/relayfor-assets/icons.woff2":    {Data: []byte("test-font")},
		"web/dist/relayfor-logo.svg":              {Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)},
		"web/dist/api/private.js":                 {Data: []byte("must not shadow a backend URL")},
		"web/dist/v1/private.js":                  {Data: []byte("must not shadow a relay URL")},
		"web/dist/relayfor-assets/folder.js/file": {Data: []byte("directory must not bypass the limiter")},
	}
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	api := router.Group("/api", middleware.GlobalAPIRateLimit())
	api.GET("/probe.js", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"backend": true}) })
	SetWebRouter(router, WebAssets{BuildFS: assets, IndexPage: []byte("dashboard")}, func(c *gin.Context) { c.Next() })
	return router
}

func webResourceRequest(router *gin.Engine, method, path, clientIP string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, nil)
	request.RemoteAddr = clientIP + ":12345"
	router.ServeHTTP(response, request)
	return response
}

func TestExistingWebResourcesDoNotExhaustDocumentRateLimit(t *testing.T) {
	router := webResourceRateLimitRouter(t)
	clientIP := freshWebResourceTestIP()
	for i := 0; i < 8; i++ {
		for _, path := range []string{"/static/js/app.js?v=1", "/relayfor-assets/app.css", "/relayfor-assets/icons.woff2", "/relayfor-logo.svg"} {
			response := webResourceRequest(router, http.MethodGet, path, clientIP)
			require.Equal(t, http.StatusOK, response.Code, path)
			assert.Equal(t, "max-age=604800", response.Header().Get("Cache-Control"))
			assert.NotEmpty(t, response.Header().Get("Cache-Version"))
		}
	}
	page := webResourceRequest(router, http.MethodGet, "/", clientIP)
	require.Equal(t, http.StatusOK, page.Code)
	assert.Equal(t, "<html>home</html>", page.Body.String())
	assert.Equal(t, "no-cache", page.Header().Get("Cache-Control"))
	for _, path := range []string{"/console/", "/console/index.html", "/personal-settings/profile"} {
		response := webResourceRequest(router, http.MethodGet, path, clientIP)
		assert.Equal(t, http.StatusTooManyRequests, response.Code, path)
		assert.NotEmpty(t, response.Header().Get("Retry-After"))
		assert.NotContains(t, response.Header().Get("Cache-Control"), "604800")
	}
	// Already-limited navigation must not break the scripts needed to log in.
	asset := webResourceRequest(router, http.MethodGet, "/static/js/app.js", clientIP)
	assert.Equal(t, http.StatusOK, asset.Code)
	assert.Equal(t, "window.applicationLoaded = true;", asset.Body.String())
	head := webResourceRequest(router, http.MethodHead, "/relayfor-assets/app.css", clientIP)
	assert.Equal(t, http.StatusOK, head.Code)
	assert.Empty(t, head.Body.String())
}

func TestWebResourceFastPathKeepsAPIAndMissingPathLimits(t *testing.T) {
	router := webResourceRateLimitRouter(t)
	t.Run("registered API retains independent API limit", func(t *testing.T) {
		clientIP := freshWebResourceTestIP()
		response := webResourceRequest(router, http.MethodGet, "/api/probe.js", clientIP)
		assert.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), `"backend":true`)
		response = webResourceRequest(router, http.MethodGet, "/api/probe.js", clientIP)
		assert.Equal(t, http.StatusTooManyRequests, response.Code)
		// API requests did not spend the separate HTML allowance.
		assert.Equal(t, http.StatusOK, webResourceRequest(router, http.MethodGet, "/", clientIP).Code)
	})
	for _, path := range []string{"/api/private.js", "/v1/private.js", "/assets/missing.js", "/relayfor-assets/folder.js", "/%61pi/private.js"} {
		t.Run(path, func(t *testing.T) {
			clientIP := freshWebResourceTestIP()
			first := webResourceRequest(router, http.MethodGet, path, clientIP)
			assert.NotEqual(t, http.StatusTooManyRequests, first.Code)
			if strings.HasPrefix(path, "/api") || strings.HasPrefix(path, "/v1") || strings.HasPrefix(path, "/assets") || strings.HasPrefix(path, "/%61pi") {
				assert.Equal(t, http.StatusNotFound, first.Code)
				assert.Contains(t, first.Header().Get("Cache-Control"), "no-store")
			}
			second := webResourceRequest(router, http.MethodGet, path, clientIP)
			assert.Equal(t, http.StatusTooManyRequests, second.Code)
		})
	}
	t.Run("POST to an existing file is not an asset bypass", func(t *testing.T) {
		clientIP := freshWebResourceTestIP()
		first := webResourceRequest(router, http.MethodPost, "/static/js/app.js", clientIP)
		assert.NotEqual(t, http.StatusTooManyRequests, first.Code)
		second := webResourceRequest(router, http.MethodPost, "/static/js/app.js", clientIP)
		assert.Equal(t, http.StatusTooManyRequests, second.Code)
	})
}
