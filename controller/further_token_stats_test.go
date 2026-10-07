package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFurtherTokenUsageRequiresAdminAndUsesUTC8Period(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}, &model.AuditLog{}))
	adminToken, userToken := "token-stats-admin-pat", "token-stats-member-pat"
	require.NoError(t, db.Create(&[]model.User{
		{Id: 701, Username: "stats-admin", AffCode: "stats-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, AccessToken: &adminToken},
		{Id: 702, Username: "stats-member", AffCode: "stats-member", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AccessToken: &userToken},
	}).Error)
	today := model.FurtherTodayStart(time.Now())
	require.NoError(t, db.Create(&[]model.Log{
		{UserId: 701, Username: "stats-admin", TokenId: 9, TokenName: "admin-key", CreatedAt: today, Type: model.LogTypeConsume, Quota: 40, PromptTokens: 20},
		{UserId: 702, Username: "stats-member", TokenId: 10, TokenName: "member-key", CreatedAt: today, Type: model.LogTypeConsume, Quota: 60, PromptTokens: 30},
		{UserId: 702, Username: "stats-member", TokenId: 10, TokenName: "member-key", CreatedAt: today - 1, Type: model.LogTypeConsume, Quota: 2000},
	}).Error)
	engine := gin.New()
	admin := engine.Group("/api/admin", middleware.AdminAuth())
	admin.GET("/token-usage", FurtherAdminTokenUsage)
	query := func(path, token string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		engine.ServeHTTP(response, request)
		return response
	}
	require.Equal(t, http.StatusUnauthorized, query("/api/admin/token-usage", "").Code)
	require.Equal(t, http.StatusForbidden, query("/api/admin/token-usage", userToken).Code)
	for _, invalid := range []string{"days=0", "days=999", "days=abc", "page=0", "page_size=101", "keyword=" + strings.Repeat("a", 201)} {
		require.Equal(t, http.StatusBadRequest, query("/api/admin/token-usage?"+invalid, adminToken).Code, invalid)
	}
	response := query("/api/admin/token-usage?days=1&page_size=1", adminToken)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var payload struct {
		Success bool                        `json:"success"`
		Data    model.FurtherTokenUsagePage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.EqualValues(t, 2, payload.Data.Total)
	require.Len(t, payload.Data.Items, 1)
	require.EqualValues(t, 2, payload.Data.Summary.ActiveUsers)
	require.EqualValues(t, 100, payload.Data.Summary.Quota)
	require.EqualValues(t, 50, payload.Data.Summary.PromptTokens)
	require.NotContains(t, response.Body.String(), adminToken)
	require.NotContains(t, response.Body.String(), userToken)
}
