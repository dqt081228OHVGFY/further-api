package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFurtherWelcomeRequiresAuthenticatedCurrentAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/further/account/welcome?user_id=1", strings.NewReader(`{"user_id":1,"welcome_completed":true}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.Contains(t, response.Header().Get("Cache-Control"), "no-store")
	for _, route := range engine.Routes() {
		if route.Method == http.MethodPost && route.Path == "/api/further/account/welcome" {
			require.Contains(t, route.Handler, "CompleteFurtherWelcome")
			return
		}
	}
	t.Fatal("authenticated welcome completion route is missing")
}
