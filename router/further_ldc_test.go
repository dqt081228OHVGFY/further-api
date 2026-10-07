package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFurtherLDCRoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetFurtherLDCRouter(engine.Group("/api"))
	for _, item := range []struct{ method, path string }{
		{"GET", "/api/user/ldc/info"}, {"POST", "/api/user/ldc/quote"}, {"POST", "/api/user/ldc/pay"},
		{"GET", "/api/user/ldc/orders"}, {"GET", "/api/user/ldc/orders/unknown"}, {"POST", "/api/subscription/ldc/pay"},
		{"GET", "/api/admin/ldc/config"}, {"PUT", "/api/admin/ldc/config"},
	} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(item.method, item.path, nil))
		require.Equal(t, http.StatusUnauthorized, response.Code, item.path)
	}
}

func TestFurtherLDCRetiresObsoleteCheckoutRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)
	for _, path := range []string{
		"/api/user/amount", "/api/user/stripe/amount", "/api/user/waffo/amount", "/api/user/waffo-pancake/amount",
		"/api/subscription/balance/pay", "/api/subscription/epay/pay", "/api/subscription/stripe/pay", "/api/subscription/creem/pay", "/api/subscription/waffo-pancake/pay",
		"/api/option/waffo-pancake/catalog", "/api/option/waffo-pancake/pair", "/api/option/waffo-pancake/save", "/api/option/waffo-pancake/subscription-product", "/api/option/waffo-pancake/subscription-product-options",
	} {
		method := http.MethodPost
		if strings.HasSuffix(path, "/catalog") || strings.HasSuffix(path, "/subscription-product-options") {
			method = http.MethodGet
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(method, path, nil))
		require.Equal(t, http.StatusNotFound, response.Code, method+" "+path)
	}
	routes := map[string]string{}
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = route.Handler
	}
	for _, path := range []string{"/api/user/ldc/pay", "/api/subscription/ldc/pay"} {
		require.Contains(t, routes, "POST "+path)
		require.Contains(t, routes["POST "+path], "LDC")
	}
	for _, route := range []string{"POST /api/stripe/webhook", "POST /api/creem/webhook", "POST /api/waffo/webhook", "POST /api/waffo-pancake/webhook/:env", "GET /api/user/epay/notify", "GET /api/subscription/epay/notify"} {
		require.Contains(t, routes, route, "previously paid orders retain their verification callback")
	}
}

func TestGatewayCheckoutRoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)
	for _, path := range []string{"/api/user/checkout", "/api/user/pay", "/api/user/stripe/pay", "/api/user/creem/pay", "/api/user/waffo/pay", "/api/user/waffo-pancake/pay"} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"amount":"10"}`)))
		require.Equal(t, http.StatusUnauthorized, response.Code, path)
		require.Contains(t, response.Header().Get("Cache-Control"), "no-store", path)
	}
}
