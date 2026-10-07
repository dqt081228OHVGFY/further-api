package router

import (
	"bytes"
	"encoding/json"
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFurtherActivityProtectedEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetFurtherActivitiesRouter(engine.Group("/api"))
	for _, request := range []struct{ method, path string }{{"GET", "/api/user/activities"}, {"POST", "/api/user/activities/claim"}, {"PUT", "/api/user/activities/subscription"}, {"GET", "/api/admin/activities"}, {"PUT", "/api/admin/activities"}, {"PUT", "/api/admin/activities/checkin"}} {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(request.method, request.path, nil))
		require.Equal(t, http.StatusUnauthorized, recorder.Code, request.path)
	}
}

func TestFurtherActivityRetiredFollowAndRootConfiguration(t *testing.T) {
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
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AuditLog{}, &model.FurtherHolidayCampaign{}, &model.FurtherHolidayClaim{}, &model.SubscriptionPlan{}))
	memberToken, rootToken := "activity-member-pat", "activity-root-pat"
	member := model.User{Username: "activity-member", AffCode: "activity-member", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AccessToken: &memberToken}
	root := model.User{Username: "activity-root", AffCode: "activity-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AccessToken: &rootToken}
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, db.Create(&root).Error)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetFurtherActivitiesRouter(engine.Group("/api"))
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
	for _, body := range []string{``, `{`, `{}`, `{"key":"chongyang"}`, `{"key":"chongyang","subscribed":null}`, `{"key":"chongyang","subscribed":"yes"}`, `{"key":"unknown","subscribed":true}`, `{"key":"chongyang","subscribed":true,"user_id":999}`, `{"key":"chongyang","subscribed":false}`} {
		response := request(http.MethodPut, "/api/user/activities/subscription", memberToken, body)
		require.Equal(t, http.StatusGone, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), `"code":"HOLIDAY_FOLLOW_RETIRED"`)
		require.Contains(t, response.Body.String(), `"success":false`)
		require.NotContains(t, response.Body.String(), `"subscribed"`)
	}
	require.False(t, db.Migrator().HasTable("further_holiday_subscriptions"))
	plan := model.SubscriptionPlan{Title: "节日精选套餐", PriceAmount: 9.9}
	require.NoError(t, db.Create(&plan).Error)
	campaign := model.FurtherDefaultHolidayCampaigns()[3]
	campaign.Details, campaign.Rules = strings.Repeat("节", 4000), strings.Repeat("规", 2000)
	campaign.SubscriptionPlanIDs = []int{plan.Id}
	body, err := json.Marshal(campaign)
	require.NoError(t, err)
	require.Greater(t, len(body), 16384)
	response := request(http.MethodPut, "/api/admin/activities", rootToken, string(body))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.True(t, bytes.Contains(response.Body.Bytes(), []byte(`"success":true`)))
	for _, read := range []struct{ path, token string }{{"/api/admin/activities", rootToken}, {"/api/user/activities", memberToken}, {"/api/further/activities/theme", ""}} {
		response = request(http.MethodGet, read.path, read.token, "")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		for _, field := range []string{"subscription_enabled", "subscribed", "can_subscribe", "subscription_count"} {
			require.NotContains(t, response.Body.String(), `"`+field+`"`)
		}
		require.Contains(t, response.Body.String(), `"subscription_plan_ids":[1]`)
		require.NotContains(t, response.Body.String(), memberToken)
	}
	require.False(t, db.Migrator().HasTable("further_holiday_subscriptions"))
}
