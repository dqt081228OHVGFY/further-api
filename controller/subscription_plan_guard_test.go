package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSubscriptionPlanGuardTest(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, _ := newAuditTestDatabase(t, "sqlite", "")
	oldDB, oldRedis := model.DB, common.RedisEnabled
	model.DB, common.RedisEnabled = db, false
	t.Cleanup(func() {
		model.DB, common.RedisEnabled = oldDB, oldRedis
		connection, err := db.DB()
		if err == nil {
			_ = connection.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.RelayForSubscriptionPolicy{}, &model.RelayForPrepaidCard{}))
	confirmPaymentComplianceForTest(t)
	return db
}

func callSubscriptionPlanHandler(t *testing.T, method, path, body, id string, handler func(*gin.Context)) struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
} {
	t.Helper()
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	if id != "" {
		context.Params = gin.Params{{Key: "id", Value: id}}
	}
	handler(context)
	var result struct {
		Success bool            `json:"success"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result), response.Body.String())
	return result
}

func TestSubscriptionPlanCatalogExcludesInternalAndPrepaid(t *testing.T) {
	db := setupSubscriptionPlanGuardTest(t)
	ordinary := model.SubscriptionPlan{Title: "Public VIP", Enabled: true}
	internal := model.SubscriptionPlan{Title: "Internal grant", Enabled: true}
	prepaid := model.SubscriptionPlan{Title: "Prepaid fulfillment", Enabled: true}
	for _, plan := range []*model.SubscriptionPlan{&ordinary, &internal, &prepaid} {
		require.NoError(t, db.Create(plan).Error)
	}
	require.NoError(t, db.Create(&model.RelayForSubscriptionPolicy{PlanID: internal.Id, Code: "internal-test", Internal: true}).Error)
	card := model.RelayForPrepaidCard{PlanID: prepaid.Id, Code: "rfp_test", CodeHash: "test-hash", Name: "Prepaid"}
	require.NoError(t, db.Create(&card).Error)
	require.NoError(t, db.Delete(&card).Error, "soft-deleted prepaid cards still reserve their plan")

	for _, handler := range []func(*gin.Context){GetSubscriptionPlans, AdminListSubscriptionPlans} {
		result := callSubscriptionPlanHandler(t, http.MethodGet, "/api/subscription/plans", "", "", handler)
		require.True(t, result.Success, result.Message)
		var plans []SubscriptionPlanDTO
		require.NoError(t, json.Unmarshal(result.Data, &plans))
		require.Len(t, plans, 1)
		require.Equal(t, ordinary.Id, plans[0].Plan.Id)
	}
}

func TestAdminSubscriptionPlanMutationRejectsInternalAndPrepaid(t *testing.T) {
	db := setupSubscriptionPlanGuardTest(t)
	ordinary := model.SubscriptionPlan{Title: "Ordinary", Enabled: true}
	internal := model.SubscriptionPlan{Title: "Internal", Enabled: true}
	prepaid := model.SubscriptionPlan{Title: "Prepaid", Enabled: true}
	for _, plan := range []*model.SubscriptionPlan{&ordinary, &internal, &prepaid} {
		require.NoError(t, db.Create(plan).Error)
	}
	require.NoError(t, db.Create(&model.RelayForSubscriptionPolicy{PlanID: internal.Id, Code: "internal-mutation", Internal: true}).Error)
	require.NoError(t, db.Create(&model.RelayForPrepaidCard{PlanID: prepaid.Id, Code: "rfp_mutation", CodeHash: "mutation-hash"}).Error)

	for _, protected := range []model.SubscriptionPlan{internal, prepaid} {
		id := fmt.Sprint(protected.Id)
		update := callSubscriptionPlanHandler(t, http.MethodPut, "/api/admin/subscription/plans/"+id, `{"plan":{"title":"Changed","enabled":false}}`, id, AdminUpdateSubscriptionPlan)
		require.False(t, update.Success, protected.Title)
		status := callSubscriptionPlanHandler(t, http.MethodPatch, "/api/admin/subscription/plans/"+id+"/status", `{"enabled":false}`, id, AdminUpdateSubscriptionPlanStatus)
		require.False(t, status.Success, protected.Title)
		var persisted model.SubscriptionPlan
		require.NoError(t, db.First(&persisted, protected.Id).Error)
		require.Equal(t, protected.Title, persisted.Title)
		require.True(t, persisted.Enabled)
	}

	id := fmt.Sprint(ordinary.Id)
	update := callSubscriptionPlanHandler(t, http.MethodPut, "/api/admin/subscription/plans/"+id, `{"plan":{"title":"Changed ordinary","enabled":true}}`, id, AdminUpdateSubscriptionPlan)
	require.True(t, update.Success, update.Message)
	status := callSubscriptionPlanHandler(t, http.MethodPatch, "/api/admin/subscription/plans/"+id+"/status", `{"enabled":false}`, id, AdminUpdateSubscriptionPlanStatus)
	require.True(t, status.Success, status.Message)
	status = callSubscriptionPlanHandler(t, http.MethodPatch, "/api/admin/subscription/plans/"+id+"/status", `{"enabled":false}`, id, AdminUpdateSubscriptionPlanStatus)
	require.True(t, status.Success, "setting an already disabled ordinary plan must remain successful")
	var persisted model.SubscriptionPlan
	require.NoError(t, db.First(&persisted, ordinary.Id).Error)
	require.Equal(t, "Changed ordinary", persisted.Title)
	require.False(t, persisted.Enabled)
}

func TestAdminCreateSubscriptionPlanPersistsExplicitDisabled(t *testing.T) {
	db := setupSubscriptionPlanGuardTest(t)
	for _, test := range []struct {
		name, body string
		want       bool
	}{
		{"explicitly disabled", `{"plan":{"title":"Disabled draft","enabled":false}}`, false},
		{"default enabled", `{"plan":{"title":"Enabled default"}}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := callSubscriptionPlanHandler(t, http.MethodPost, "/api/admin/subscription/plans", test.body, "", AdminCreateSubscriptionPlan)
			require.True(t, result.Success, result.Message)
			var returned model.SubscriptionPlan
			require.NoError(t, json.Unmarshal(result.Data, &returned))
			require.Equal(t, test.want, returned.Enabled)
			var persisted model.SubscriptionPlan
			require.NoError(t, db.First(&persisted, returned.Id).Error)
			require.Equal(t, test.want, persisted.Enabled)
		})
	}
}
