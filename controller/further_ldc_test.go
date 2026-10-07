package controller

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupLDCControllerTest(t *testing.T) (model.FurtherLDCSettings, model.User) {
	t.Helper()
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldRedis, oldType := common.RedisEnabled, common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	connection.SetMaxOpenConns(1)
	model.DB, model.LOG_DB, common.RedisEnabled = db, db, false
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled = oldDB, oldLogDB, oldRedis
		common.SetMainDatabaseType(oldType)
		require.NoError(t, connection.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}, &model.AuditLog{}, &model.FurtherLDCCredential{}, &model.FurtherLDCSettings{}, &model.FurtherLDCOrder{}, &model.TopUp{}, &model.SubscriptionPlan{}, &model.UserSubscription{}))
	cfg, err := model.SaveFurtherLDCSettings(model.FurtherLDCSettings{Enabled: true, ClientID: "test-merchant", ClientSecret: "fake-ldc-secret", CreditsPerUnit: "10", MinTopUp: "0.01", CallbackBaseURL: "https://further.example"})
	require.NoError(t, err)
	user := model.User{Username: "ldc-controller", AffCode: "ldc-controller", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, db.Create(&user).Error)
	return cfg, user
}

func callLDCHandler(method, target, body string, userID int, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	handler(ctx)
	return response
}

type ldcRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn ldcRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestFurtherLDCNativeSigningAndSecretRedaction(t *testing.T) {
	cfg, user := setupLDCControllerTest(t)
	order, err := model.CreateFurtherLDCOrder(user.Id, "1.25", 0, "12.50", cfg)
	require.NoError(t, err)
	params, err := furtherLDCForm(order, cfg)
	require.NoError(t, err)
	require.Equal(t, "ldcpay", params["type"])
	require.NotContains(t, params, "pid")
	require.NotContains(t, params, "client_secret")
	canonical := "client_id=test-merchant&money=12.50&notify_url=https://further.example/api/ldc/notify&order_name=further-api 钱包充值&out_trade_no=" + order.TradeNo + "&return_url=https://further.example/api/ldc/return&type=ldcpay" + cfg.ClientSecret
	key, err := model.ParseFurtherLDCPrivateKey(cfg.PrivateKey)
	require.NoError(t, err)
	signature, err := base64.StdEncoding.DecodeString(params["sign"])
	require.NoError(t, err)
	require.True(t, ed25519.Verify(key.Public().(ed25519.PublicKey), []byte(canonical), signature))
	require.False(t, ed25519.Verify(key.Public().(ed25519.PublicKey), []byte(strings.Replace(canonical, "12.50", "0.01", 1)), signature))
	for _, handler := range []gin.HandlerFunc{FurtherGetLDCConfig, FurtherLDCInfo, FurtherLDCOrders} {
		response := callLDCHandler("GET", "/", "", user.Id, handler)
		require.Equal(t, http.StatusOK, response.Code)
		require.NotContains(t, response.Body.String(), cfg.ClientSecret)
		require.NotContains(t, response.Body.String(), cfg.PrivateKey)
	}
}

func TestFurtherLDCQuoteAndPayRequireConfirmedAmount(t *testing.T) {
	_, user := setupLDCControllerTest(t)
	response := callLDCHandler("POST", "/", `{"amount":"1.25"}`, user.Id, FurtherLDCQuote)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"credits":"12.50"`)
	for _, body := range []string{`{"amount":"1.25"}`, `{"amount":"1.25","expected_credits":"0.01"}`, `{"amount":"1.25","plan_id":2,"expected_credits":"12.50"}`} {
		response = callLDCHandler("POST", "/", body, user.Id, FurtherLDCPay)
		require.Contains(t, response.Body.String(), `"success":false`)
	}
	response = callLDCHandler("POST", "/", `{"amount":"1.25","expected_credits":"12.50","user_id":999}`, user.Id, FurtherLDCPay)
	require.Contains(t, response.Body.String(), `"success":true`)
	require.Contains(t, response.Body.String(), furtherLDCCheckoutURL)
	var order model.FurtherLDCOrder
	require.NoError(t, model.DB.First(&order).Error)
	require.Equal(t, user.Id, order.UserID)
	var ledger int64
	require.NoError(t, model.DB.Model(&model.TopUp{}).Count(&ledger).Error)
	require.Zero(t, ledger, "opening checkout never credits the wallet")
}

func TestFurtherLDCNotificationRequiresIndependentGatewayQuery(t *testing.T) {
	cfg, user := setupLDCControllerTest(t)
	order, err := model.CreateFurtherLDCOrder(user.Id, "1", 0, "10.00", cfg)
	require.NoError(t, err)
	oldClient := furtherLDCHTTPClient
	t.Cleanup(func() { furtherLDCHTTPClient = oldClient })
	result := furtherLDCQueryResult{Code: 1, TradeNo: "8001", OutTradeNo: order.TradeNo, PID: cfg.ClientID, Money: "10.00", Type: "ldcpay", Status: 0}
	requests := 0
	furtherLDCHTTPClient = &http.Client{Transport: ldcRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		require.Equal(t, "credit.linux.do", request.URL.Host)
		require.Equal(t, "https", request.URL.Scheme)
		require.Equal(t, "/epay/api.php", request.URL.Path)
		require.Equal(t, cfg.ClientSecret, request.URL.Query().Get("key"))
		require.Equal(t, "8001", request.URL.Query().Get("trade_no"))
		body, _ := json.Marshal(result)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}
	callback := "/api/ldc/notify?" + url.Values{"out_trade_no": {order.TradeNo}, "trade_no": {"8001"}, "trade_status": {"TRADE_SUCCESS"}, "money": {"10.00"}, "sign": {"forged"}}.Encode()
	response := callLDCHandler("GET", callback, "", 0, FurtherLDCNotify)
	require.Equal(t, "fail", response.Body.String(), "a forged callback cannot authorize credit")
	require.NoError(t, model.DB.First(&user, user.Id).Error)
	require.Zero(t, user.Quota)
	result.Status, result.Money = 1, "0.01"
	response = callLDCHandler("GET", callback, "", 0, FurtherLDCNotify)
	require.Equal(t, "fail", response.Body.String())
	result.Money, result.PID = "10.00", "other-merchant"
	response = callLDCHandler("GET", callback, "", 0, FurtherLDCNotify)
	require.Equal(t, "fail", response.Body.String())
	result.PID = cfg.ClientID
	for range 2 {
		response = callLDCHandler("GET", callback, "", 0, FurtherLDCNotify)
		require.Equal(t, http.StatusOK, response.Code)
		require.Equal(t, "success", response.Body.String())
	}
	require.Equal(t, 5, requests)
	require.NoError(t, model.DB.First(&user, user.Id).Error)
	require.Equal(t, order.Quota, user.Quota)
}

func TestFurtherLDCReturnAndDuplicateParametersCannotCredit(t *testing.T) {
	cfg, user := setupLDCControllerTest(t)
	order, err := model.CreateFurtherLDCOrder(user.Id, "1", 0, "10.00", cfg)
	require.NoError(t, err)
	response := callLDCHandler("GET", "/api/ldc/return?trade_status=TRADE_SUCCESS&out_trade_no="+order.TradeNo, "", 0, FurtherLDCReturn)
	require.Equal(t, http.StatusFound, response.Code)
	require.NoError(t, model.DB.First(&user, user.Id).Error)
	require.Zero(t, user.Quota)
	response = callLDCHandler("GET", "/api/ldc/notify?trade_no=1&trade_no=2&out_trade_no="+order.TradeNo, "", 0, FurtherLDCNotify)
	require.Equal(t, http.StatusBadRequest, response.Code)
}

func TestFurtherLDCPlanManagementDoesNotDependOnRetiredCompliance(t *testing.T) {
	_, user := setupLDCControllerTest(t)
	old := operation_setting.GetPaymentSetting().ComplianceConfirmed
	operation_setting.GetPaymentSetting().ComplianceConfirmed = false
	t.Cleanup(func() { operation_setting.GetPaymentSetting().ComplianceConfirmed = old })
	response := callLDCHandler("POST", "/api/subscription/admin/plans", `{"plan":{"title":"LDC plan","price_amount":2,"duration_unit":"month","duration_value":1,"enabled":true,"total_amount":1000}}`, user.Id, AdminCreateSubscriptionPlan)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"success":true`)
}
