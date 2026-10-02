package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRiskHandlersValidateInputBeforeDatabaseAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/risk/users", ListRiskUsers)
	engine.GET("/risk/users/:id", GetRiskUser)
	engine.PATCH("/risk/users/:id/status", UpdateRiskUserStatus)
	for _, test := range []struct{ method, path, body string }{
		{http.MethodGet, "/risk/users?window=24h", ""},
		{http.MethodGet, "/risk/users?page=0", ""},
		{http.MethodGet, "/risk/users?page_size=101", ""},
		{http.MethodGet, "/risk/users?level=invalid", ""},
		{http.MethodGet, "/risk/users?status=disabled", ""},
		{http.MethodGet, "/risk/users?username=" + strings.Repeat("a", 65), ""},
		{http.MethodGet, "/risk/users/-1", ""},
		{http.MethodGet, "/risk/users/1?window=8d", ""},
		{http.MethodPatch, "/risk/users/1/status", `{}`},
		{http.MethodPatch, "/risk/users/1/status", `{"status":"disabled"}`},
		{http.MethodPatch, "/risk/users/1/status", `{"status":"` + strings.Repeat("a", 1024) + `"}`},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		request.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusBadRequest, recorder.Code, test.path)
	}
}
