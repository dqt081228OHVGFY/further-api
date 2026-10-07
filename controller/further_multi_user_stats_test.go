package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFurtherMultiUserQueryValidatesExactDatesAndNames(t *testing.T) {
	valid := url.Values{"usernames": {"alice, bob，alice"}, "start_time": {"2026-10-01"}, "end_time": {"2026-10-02"}}
	query := func(values url.Values) ([]string, int64, int64, error) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/?"+values.Encode(), nil)
		return furtherMultiUserQuery(c)
	}
	names, start, end, err := query(valid)
	require.NoError(t, err)
	require.Equal(t, []string{"alice", "bob"}, names)
	require.Equal(t, time.Date(2026, 9, 30, 16, 0, 0, 0, time.UTC).Unix(), start)
	require.Equal(t, time.Date(2026, 10, 2, 16, 0, 0, 0, time.UTC).Unix(), end)
	for _, input := range []struct{ key, value string }{
		{"usernames", " , "}, {"usernames", strings.Repeat("a", 65)}, {"usernames", "alice\x00"},
		{"start_time", "2026-02-30"}, {"start_time", "2026-10-03"}, {"start_time", "2025-01-01"},
		{"end_time", ""}, {"end_time", "2026-1-02"},
	} {
		changed := url.Values{}
		for key, values := range valid {
			changed[key] = append([]string{}, values...)
		}
		changed.Set(input.key, input.value)
		_, _, _, err := query(changed)
		require.Error(t, err, input.key+"="+input.value)
	}
}

func TestFurtherMultiUserQueryAcceptsJSONNamesWithoutSplittingCommas(t *testing.T) {
	query := func(input string) ([]string, error) {
		values := url.Values{"usernames_json": {input}, "usernames": {"wrong,fallback"}, "start_time": {"2026-10-01"}, "end_time": {"2026-10-01"}}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/?"+values.Encode(), nil)
		names, _, _, err := furtherMultiUserQuery(c)
		return names, err
	}
	names, err := query(`["alice,bob","张，三","carol","alice,bob"]`)
	require.NoError(t, err)
	require.Equal(t, []string{"alice,bob", "张，三", "carol"}, names)
	for _, invalid := range []string{``, `not-json`, `{"name":"alice"}`, `["alice",5]`, `["alice",null]`, `[]`, `null`} {
		_, err := query(invalid)
		require.Error(t, err, invalid)
	}
}

func TestFurtherMultiUserStatsRequiresAdminAndReturnsRealUsage(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}, &model.AuditLog{}))
	adminToken, memberToken := "multi-stats-admin-pat", "multi-stats-member-pat"
	require.NoError(t, db.Create(&[]model.User{
		{Id: 8801, Username: "multi-admin", AffCode: "multi-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, AccessToken: &adminToken},
		{Id: 8802, Username: "multi-member", AffCode: "multi-member", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AccessToken: &memberToken},
		{Id: 8803, Username: "name,with，commas", AffCode: "multi-comma", Role: common.RoleCommonUser, Status: common.UserStatusEnabled},
	}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: 8802, Username: "multi-member", ModelName: "real-model", CreatedAt: time.Date(2026, 10, 1, 5, 0, 0, 0, time.UTC).Unix(), Type: model.LogTypeConsume, Quota: 120, PromptTokens: 80, CompletionTokens: 20, UseTime: 2}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: 8803, Username: "name,with，commas", ModelName: "comma-model", CreatedAt: time.Date(2026, 10, 1, 5, 0, 0, 0, time.UTC).Unix(), Type: model.LogTypeConsume, Quota: 77}).Error)
	engine := gin.New()
	admin := engine.Group("/api/admin", middleware.AdminAuth())
	admin.GET("/multi-user-stats", FurtherAdminMultiUserStats)
	path := "/api/admin/multi-user-stats?usernames=multi-member&start_time=2026-10-01&end_time=2026-10-01"
	request := func(token string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		engine.ServeHTTP(response, req)
		return response
	}
	require.Equal(t, http.StatusUnauthorized, request("").Code)
	require.Equal(t, http.StatusForbidden, request(memberToken).Code)
	response := request(adminToken)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var payload struct {
		Success bool                        `json:"success"`
		Data    model.FurtherMultiUserStats `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data.Items, 1)
	require.EqualValues(t, 120, payload.Data.Items[0].Quota)
	require.EqualValues(t, 2000, payload.Data.Items[0].RequestTime)
	require.Equal(t, "real-model", payload.Data.ModelUsage[0].ModelName)
	require.NotContains(t, response.Body.String(), memberToken)
	path = "/api/admin/multi-user-stats?" + url.Values{"usernames_json": {`["name,with，commas"]`}, "start_time": {"2026-10-01"}, "end_time": {"2026-10-01"}}.Encode()
	commaResponse := request(adminToken)
	require.Equal(t, http.StatusOK, commaResponse.Code)
	require.NoError(t, json.Unmarshal(commaResponse.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Items, 1)
	require.Equal(t, "name,with，commas", payload.Data.Items[0].Username)
	require.EqualValues(t, 77, payload.Data.Items[0].Quota)
	require.Empty(t, payload.Data.UnknownUsernames)
}
