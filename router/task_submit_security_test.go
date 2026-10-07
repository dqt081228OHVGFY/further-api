package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGenericTaskSubmissionCannotBypassConfiguredRequestLimit(t *testing.T) {
	setupRelayRouterTestDB(t)
	previousEnabled, previousCount, previousSuccess, previousWindow := setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitCount, setting.ModelRequestRateLimitSuccessCount, setting.ModelRequestRateLimitDurationMinutes
	previousGroups := setting.ModelRequestRateLimitGroup2JSONString()
	setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitCount, setting.ModelRequestRateLimitSuccessCount, setting.ModelRequestRateLimitDurationMinutes = true, 1, 0, 1
	require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString("{}"))
	t.Cleanup(func() {
		setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitCount, setting.ModelRequestRateLimitSuccessCount, setting.ModelRequestRateLimitDurationMinutes = previousEnabled, previousCount, previousSuccess, previousWindow
		require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(previousGroups))
	})
	owner := model.User{Id: 84010, Username: "task-limit", AffCode: "task-limit", Status: common.UserStatusEnabled, Quota: 100, Group: "default"}
	require.NoError(t, model.DB.Create(&owner).Error)
	require.NoError(t, model.DB.Create(&model.Token{UserId: owner.Id, Key: "generictasklimitkey", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
	engine := gin.New()
	SetTaskRouter(engine)
	for _, expected := range []int{http.StatusBadRequest, http.StatusTooManyRequests} {
		request := httptest.NewRequest(http.MethodPost, "/v1/tasks/security-review-unregistered-plugin", strings.NewReader(`{"model":"isolated-fixture"}`))
		request.Header.Set("Authorization", "Bearer generictasklimitkey")
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		require.Equal(t, expected, response.Code)
	}
}
