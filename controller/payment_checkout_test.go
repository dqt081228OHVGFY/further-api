package controller

import (
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestGatewayCheckoutHTTPBuildsConfiguredEpayWithoutChargingNetwork(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}))
	confirmPaymentComplianceForTest(t)
	address, id, key, price, minimum, methods := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, operation_setting.Price, operation_setting.MinTopUp, operation_setting.PayMethods
	t.Cleanup(func() {
		operation_setting.PayAddress = address
		operation_setting.EpayId = id
		operation_setting.EpayKey = key
		operation_setting.Price = price
		operation_setting.MinTopUp = minimum
		operation_setting.PayMethods = methods
	})
	operation_setting.PayAddress = "https://merchant.example.invalid"
	operation_setting.EpayId = "test-merchant"
	operation_setting.EpayKey = "test-signing-key"
	operation_setting.Price = 7
	operation_setting.MinTopUp = 1
	operation_setting.PayMethods = []map[string]string{{"type": "alipay", "name": "支付宝"}}
	user := model.User{Username: "checkout-http", AffCode: "checkout-http", Role: common.RoleCommonUser}
	require.NoError(t, db.Create(&user).Error)
	ctx, response := newAuthenticatedContext(t, http.MethodPost, "/api/user/pay", map[string]any{"amount": 2, "payment_method": "alipay"}, user.Id)
	RequestEpay(ctx)
	result := decodeAPIResponse(t, response)
	require.True(t, result.Success, result.Message)
	var checkout service.GatewayCheckoutResult
	require.NoError(t, common.Unmarshal(result.Data, &checkout))
	require.Equal(t, http.MethodPost, checkout.Method)
	require.Equal(t, "https://merchant.example.invalid/submit.php", checkout.URL)
	require.NotEmpty(t, checkout.Params["sign"])
	var stored model.TopUp
	require.NoError(t, db.Where("trade_no = ?", checkout.TradeNo).First(&stored).Error)
	require.Equal(t, user.Id, stored.UserId)
	require.Equal(t, model.PaymentProviderEpay, stored.PaymentProvider)
	require.Equal(t, 2*int(common.QuotaPerUnit), stored.CreditedQuota)
}
