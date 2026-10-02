package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRiskRoutesRequireAuthenticatedAdministrator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerRiskRoutes(engine.Group("/api"))
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/risk/users"},
		{http.MethodGet, "/api/risk/users/1"},
		{http.MethodPatch, "/api/risk/users/1/status"},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"status":"confirmed"}`))
		request.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusUnauthorized, recorder.Code, route.path)
		require.Contains(t, recorder.Header().Get("Cache-Control"), "no-store")
	}
}
