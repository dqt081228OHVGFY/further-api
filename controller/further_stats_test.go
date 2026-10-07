package controller

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFurtherStatsRejectsInvalidQueriesAndPreferenceFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/logs", FurtherUserLogs)
	engine.GET("/overview", FurtherAdminOverview)
	engine.PATCH("/prefs", FurtherPatchPreferences)
	for _, test := range []struct{ method, path, body string }{
		{"GET", "/logs?page=0", ""}, {"GET", "/logs?page_size=10000", ""}, {"GET", "/logs?status_code=-1", ""},
		{"GET", "/logs?sort_by=other", ""}, {"GET", "/logs?sort_order=DROP", ""}, {"GET", "/logs?start_timestamp=100&end_timestamp=99", ""},
		{"GET", "/logs?only_errors=maybe", ""},
		{"GET", "/overview?days=999", ""}, {"PATCH", "/prefs", `{"user_id":2}`}, {"PATCH", "/prefs", `{"page_size":-1}`},
		{"PATCH", "/prefs", `{"reduced_motion":"yes"}`}, {"PATCH", "/prefs", `{"default_page":"tokens|models"}`},
		{"PATCH", "/prefs", `{"theme":"dark"} {"theme":"light"}`}, {"PATCH", "/prefs", `{"theme":"` + strings.Repeat("a", 4096) + `"}`},
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		request.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(response, request)
		require.Equal(t, http.StatusBadRequest, response.Code, test.path+test.body)
	}
}

func TestFurtherOnlyErrorsQueryPreservesUserScopeAndLegacyFilter(t *testing.T) {
	for _, value := range []string{"true", "1", "false", "0"} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Set("id", 91)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/logs?only_errors="+value+"&exclude_status_code=200", nil)
		query, err := furtherQuery(ctx, false)
		require.NoError(t, err)
		require.Equal(t, value == "true" || value == "1", query.OnlyErrors)
		require.Equal(t, 91, query.UserID)
		require.Equal(t, 200, query.ExcludeStatusCode)
	}
}

func TestFurtherTokenCreationReturnsCreatedIDWithoutSecret(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	require.NoError(t, db.Create(&model.User{Id: 71, Username: "token-owner", Status: common.UserStatusEnabled}).Error)
	engine := gin.New()
	engine.POST("/token", func(c *gin.Context) { c.Set("id", 71); c.Next() }, AddToken)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(`{"name":"new-key","expired_time":-1,"unlimited_quota":true}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success, response.Body.String())
	require.Positive(t, payload.Data.ID)
	var created model.Token
	require.NoError(t, model.DB.First(&created, payload.Data.ID).Error)
	require.Equal(t, 71, created.UserId)
	require.Equal(t, "new-key", created.Name)
	require.NotContains(t, response.Body.String(), created.Key)
}

func TestFurtherCSVExportOnlyIncludesOwnerAndNeutralizesFormulas(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	previousLogDB := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = previousLogDB })
	require.NoError(t, db.Create(&[]model.Log{
		{UserId: 71, Type: model.LogTypeConsume, CreatedAt: 10, TokenName: "=SUM(1,2)", ModelName: "owned-model", Quota: 2, RequestId: "own-request"},
		{UserId: 72, Type: model.LogTypeConsume, CreatedAt: 11, ModelName: "private-other-model", RequestId: "foreign-request"},
	}).Error)
	engine := gin.New()
	engine.GET("/export", func(c *gin.Context) { c.Set("id", 71); c.Next() }, FurtherExportUserLogs)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/export?user_id=72", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Header().Get("Content-Type"), "text/csv")
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(response.Body.String(), "\ufeff"))).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "'=SUM(1,2)", rows[1][1])
	require.Equal(t, "owned-model", rows[1][2])
	require.NotContains(t, response.Body.String(), "foreign-request")
}
