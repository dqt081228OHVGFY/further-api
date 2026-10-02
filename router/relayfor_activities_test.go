package router

import (
	"bytes"
	"encoding/json"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRelayForActivityProtectedEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetRelayForActivitiesRouter(engine.Group("/api"))
	for _, request := range []struct{ method, path string }{{"GET", "/api/user/activities"}, {"POST", "/api/user/activities/claim"}, {"PUT", "/api/user/activities/subscription"}, {"GET", "/api/admin/activities"}, {"PUT", "/api/admin/activities"}, {"PUT", "/api/admin/activities/checkin"}} {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(request.method, request.path, nil))
		require.Equal(t, http.StatusUnauthorized, recorder.Code, request.path)
	}
}

func TestRelayForActivityAuthenticatedSubscriptionAndRootConfiguration(t *testing.T) {
	previousDB, previousLog := model.DB, model.LOG_DB
	previousType := common.MainDatabaseType()
	previousRedis, previousLimit := common.RedisEnabled, common.CriticalRateLimitEnable
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled, common.CriticalRateLimitEnable = false, false
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLog
		common.RedisEnabled, common.CriticalRateLimitEnable = previousRedis, previousLimit
		common.SetMainDatabaseType(previousType)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AuditLog{}, &model.RelayForHolidayCampaign{}, &model.RelayForHolidayClaim{}, &model.RelayForHolidaySubscription{}, &model.SubscriptionPlan{}))
	memberToken, rootToken := "activity-member-pat", "activity-root-pat"
	member := model.User{Username: "activity-member", AffCode: "activity-member", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AccessToken: &memberToken}
	root := model.User{Username: "activity-root", AffCode: "activity-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AccessToken: &rootToken}
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, db.Create(&root).Error)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetRelayForActivitiesRouter(engine.Group("/api"))
	request := func(method, path, token, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		engine.ServeHTTP(recorder, req)
		return recorder
	}
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		response := request(method, "/api/admin/activities", memberToken, `{}`)
		require.Equal(t, http.StatusForbidden, response.Code)
	}
	for _, body := range []string{`{}`, `{"key":"chongyang"}`, `{"key":"chongyang","subscribed":null}`, `{"key":"chongyang","subscribed":"yes"}`, `{"key":"unknown","subscribed":true}`} {
		response := request(http.MethodPut, "/api/user/activities/subscription", memberToken, body)
		require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	}
	for range 2 {
		response := request(http.MethodPut, "/api/user/activities/subscription", memberToken, `{"key":"chongyang","subscribed":true,"user_id":999}`)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), `"subscribed":true`)
	}
	var subscriptions []model.RelayForHolidaySubscription
	require.NoError(t, db.Find(&subscriptions).Error)
	require.Len(t, subscriptions, 1)
	require.Equal(t, member.Id, subscriptions[0].UserID)
	campaign := model.RelayForDefaultHolidayCampaigns()[3]
	campaign.Details, campaign.Rules = strings.Repeat("节", 4000), strings.Repeat("规", 2000)
	campaign.SubscriptionEnabled = false
	body, err := json.Marshal(campaign)
	require.NoError(t, err)
	require.Greater(t, len(body), 16384)
	response := request(http.MethodPut, "/api/admin/activities", rootToken, string(body))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.True(t, bytes.Contains(response.Body.Bytes(), []byte(`"success":true`)))
	response = request(http.MethodPut, "/api/user/activities/subscription", memberToken, `{"key":"chongyang","subscribed":true}`)
	require.Equal(t, http.StatusBadRequest, response.Code)
	response = request(http.MethodPut, "/api/user/activities/subscription", memberToken, `{"key":"chongyang","subscribed":false}`)
	require.Equal(t, http.StatusOK, response.Code)
	response = request(http.MethodGet, "/api/admin/activities", rootToken, "")
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"subscription_count":0`)
	response = request(http.MethodGet, "/api/relayfor/activities/theme", "", "")
	require.Equal(t, http.StatusOK, response.Code)
	require.NotContains(t, response.Body.String(), `"subscription_count"`)
	require.NotContains(t, response.Body.String(), memberToken)
}
