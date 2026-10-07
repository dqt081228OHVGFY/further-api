package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCanceledSubscriptionAdminRequestsDoNotChangeQuotaOrEntitlements(t *testing.T) {
	db := setupSubscriptionPlanGuardTest(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSubscription{}))
	account := model.User{Username: "cancel-admin-subscription", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default"}
	plan := model.SubscriptionPlan{Title: "取消请求测试套餐", Enabled: true, DurationUnit: model.SubscriptionDurationDay, DurationValue: 1, TotalAmount: 1000, QuotaResetPeriod: model.SubscriptionResetDaily}
	require.NoError(t, db.Create(&account).Error)
	require.NoError(t, db.Create(&plan).Error)
	now := time.Now().Unix()
	subscription := model.UserSubscription{UserId: account.Id, PlanId: plan.Id, AmountTotal: 1000, AmountUsed: 300, StartTime: now - 60, EndTime: now + 3600, Status: "active", LastResetTime: now - 60, NextResetTime: now + 300}
	require.NoError(t, db.Create(&subscription).Error)
	for _, test := range []struct {
		name, method, path, body string
		id                       int
		handler                  gin.HandlerFunc
	}{
		{"grant", http.MethodPost, "/api/subscription/admin/bind", fmt.Sprintf(`{"user_id":%d,"plan_id":%d}`, account.Id, plan.Id), 0, AdminBindSubscription},
		{"grant-to-user", http.MethodPost, "/api/subscription/admin/users/1/subscriptions", fmt.Sprintf(`{"plan_id":%d}`, plan.Id), account.Id, AdminCreateUserSubscription},
		{"list", http.MethodGet, "/api/subscription/admin/users/1/subscriptions", "", account.Id, AdminListUserSubscriptions},
		{"reset-user", http.MethodPost, "/api/subscription/admin/users/1/subscriptions/reset", fmt.Sprintf(`{"plan_id":%d}`, plan.Id), account.Id, AdminResetUserSubscriptionsByPlan},
		{"reset-plan", http.MethodPost, "/api/subscription/admin/plans/1/subscriptions/reset", `{}`, plan.Id, AdminResetPlanSubscriptions},
		{"invalidate", http.MethodPost, "/api/subscription/admin/user_subscriptions/1/invalidate", `{}`, subscription.Id, AdminInvalidateUserSubscription},
		{"delete", http.MethodDelete, "/api/subscription/admin/user_subscriptions/1", "", subscription.Id, AdminDeleteUserSubscription},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Request = httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)).WithContext(ctx)
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(test.id)}}
			test.handler(c)
			var result struct {
				Success bool `json:"success"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result), response.Body.String())
			require.False(t, result.Success, "a canceled request must not commit its subscription mutation")
			var persisted model.UserSubscription
			require.NoError(t, db.First(&persisted, subscription.Id).Error)
			require.Equal(t, subscription, persisted)
			var count int64
			require.NoError(t, db.Model(&model.UserSubscription{}).Where("user_id = ?", account.Id).Count(&count).Error)
			require.EqualValues(t, 1, count)
		})
	}
}
