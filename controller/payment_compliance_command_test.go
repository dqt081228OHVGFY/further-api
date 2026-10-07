package controller

import (
	"errors"
	"maps"
	"net/http"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPaymentComplianceRequiresCurrentRootBrowserSessionAndAtomicSave(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	previousSettings := *operation_setting.GetPaymentSetting()
	common.OptionMapRWMutex.RLock()
	previousOptions := maps.Clone(common.OptionMap)
	common.OptionMapRWMutex.RUnlock()
	t.Cleanup(func() {
		*operation_setting.GetPaymentSetting() = previousSettings
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	response := securityEnrollmentRequest(http.MethodPost, "/api/option/payment_compliance", `{"confirmed":true}`, "", identity, ConfirmPaymentCompliance)
	require.Equal(t, http.StatusForbidden, response.Code)
	require.Contains(t, response.Body.String(), "ROOT_REQUIRED")
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("role", common.RoleRootUser).Error)
	response = securityEnrollmentRequest(http.MethodPost, "/api/option/payment_compliance", `{"confirmed":false}`, "", identity, ConfirmPaymentCompliance)
	require.Contains(t, response.Body.String(), `"success":false`)
	var count int64
	require.NoError(t, model.DB.Model(&model.Option{}).Count(&count).Error)
	require.Zero(t, count)
	failure := errors.New("confirmation option unavailable")
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("payment_terms_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "options" {
			tx.AddError(failure)
		}
	}))
	response = securityEnrollmentRequest(http.MethodPost, "/api/option/payment_compliance", `{"confirmed":true}`, "", identity, ConfirmPaymentCompliance)
	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.NoError(t, model.DB.Model(&model.Option{}).Count(&count).Error)
	require.Zero(t, count)
	require.Equal(t, previousSettings, *operation_setting.GetPaymentSetting())
	require.NoError(t, model.DB.Callback().Update().Remove("payment_terms_failure"))
	response = securityEnrollmentRequest(http.MethodPost, "/api/option/payment_compliance", `{"confirmed":true}`, "", identity, ConfirmPaymentCompliance)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"success":true`)
	require.True(t, operation_setting.IsPaymentComplianceConfirmed())
	require.NoError(t, model.DB.Model(&model.Option{}).Count(&count).Error)
	require.EqualValues(t, 5, count)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("auth_version", identity.UserAuthVersion+1).Error)
	response = securityEnrollmentRequest(http.MethodPost, "/api/option/payment_compliance", `{"confirmed":true}`, "", identity, ConfirmPaymentCompliance)
	require.Equal(t, http.StatusUnauthorized, response.Code)
}
