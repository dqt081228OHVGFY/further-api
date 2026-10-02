package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayForBusinessWritesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	require.NotPanics(t, func() { SetRelayForBusinessRouter(engine.Group("/api")) })
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/api/user/public-benefits/token-loan/borrow"},
		{http.MethodPost, "/api/user/public-benefits/token-loan/check-in"},
		{http.MethodPost, "/api/user/public-benefits/token-loan/loss-pool/cut"},
		{http.MethodPost, "/api/admin/registration-codes/generate"},
		{http.MethodDelete, "/api/admin/registration-codes/used"},
		{http.MethodPut, "/api/admin/settings/token-loan"},
		{http.MethodPatch, "/api/admin/settings/registration"},
	} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(route.method, route.path, nil))
		require.Equal(t, http.StatusUnauthorized, response.Code, route.path)
	}
}
