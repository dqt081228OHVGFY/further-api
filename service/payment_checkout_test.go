package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
	"github.com/stretchr/testify/require"
)

func TestStripeCheckoutUsesFrozenOrderReferenceAndConfiguredPrice(t *testing.T) {
	db := setupRiskAssessmentDatabase(t)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}))
	require.NoError(t, db.Create(&model.User{Id: 61, Username: "stripe-owner", Status: common.UserStatusEnabled}).Error)
	InitHttpClient()
	previousSecret, previousPrice, previousEndpoint, previousAddress := setting.StripeApiSecret, setting.StripePriceId, checkoutEndpoints.stripe, system_setting.ServerAddress
	previousUnit, previousMinimum := setting.StripeUnitPrice, setting.StripeMinTopUp
	t.Cleanup(func() {
		setting.StripeApiSecret, setting.StripePriceId, checkoutEndpoints.stripe, system_setting.ServerAddress = previousSecret, previousPrice, previousEndpoint, previousAddress
		setting.StripeUnitPrice, setting.StripeMinTopUp = previousUnit, previousMinimum
	})
	setting.StripeApiSecret, setting.StripePriceId, setting.StripeUnitPrice, setting.StripeMinTopUp, system_setting.ServerAddress = "fixture-secret", "price_fixture", 1, 1, "https://gateway.example"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		require.Equal(t, "price_fixture", r.PostForm.Get("line_items[0][price]"))
		require.Equal(t, "3", r.PostForm.Get("line_items[0][quantity]"))
		require.Equal(t, r.Header.Get("Idempotency-Key"), r.PostForm.Get("client_reference_id"))
		require.Equal(t, "Bearer fixture-secret", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"url":"https://checkout.stripe.example/session"}`)
	}))
	defer server.Close()
	checkoutEndpoints.stripe = server.URL
	result, err := CreateGatewayCheckoutContext(context.Background(), 61, model.PaymentProviderStripe, GatewayCheckoutInput{Amount: "3", Method: "stripe"})
	require.NoError(t, err)
	require.Equal(t, "https://checkout.stripe.example/session", result.URL)
	var order model.TopUp
	require.NoError(t, db.First(&order, "trade_no = ?", result.TradeNo).Error)
	require.Equal(t, "3", order.WalletUnits)
	require.Equal(t, 3*int(common.QuotaPerUnit), order.CreditedQuota)
}

func TestCreemCheckoutIgnoresUntrustedQuotaAndUsesConfiguredProduct(t *testing.T) {
	db := setupRiskAssessmentDatabase(t)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}))
	require.NoError(t, db.Create(&model.User{Id: 62, Username: "creem-owner", Status: common.UserStatusEnabled}).Error)
	InitHttpClient()
	previousKey, previousProducts, previousEndpoint := setting.CreemApiKey, setting.CreemProducts, checkoutEndpoints.creem
	t.Cleanup(func() {
		setting.CreemApiKey, setting.CreemProducts, checkoutEndpoints.creem = previousKey, previousProducts, previousEndpoint
	})
	setting.CreemApiKey, setting.CreemProducts = "creem-fixture", `[{"productId":"prod_fixture","price":9.95,"quota":10}]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, "prod_fixture", payload["product_id"])
		_, _ = io.WriteString(w, `{"checkout_url":"https://checkout.creem.example/session"}`)
	}))
	defer server.Close()
	checkoutEndpoints.creem = server.URL + "/v1/checkouts"
	result, err := CreateGatewayCheckoutContext(context.Background(), 62, model.PaymentProviderCreem, GatewayCheckoutInput{Amount: "9999", Method: "creem", ProductID: "prod_fixture"})
	require.NoError(t, err)
	var order model.TopUp
	require.NoError(t, db.First(&order, "trade_no = ?", result.TradeNo).Error)
	require.Equal(t, "10", order.WalletUnits)
	require.Equal(t, 9.95, order.Money)
}

func TestEpayReturnURLRejectedBeforeCreatingOrder(t *testing.T) {
	db := setupRiskAssessmentDatabase(t)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}))
	previousURL, previousID, previousKey, previousAddress := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, system_setting.ServerAddress
	t.Cleanup(func() {
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, system_setting.ServerAddress = previousURL, previousID, previousKey, previousAddress
	})
	operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, system_setting.ServerAddress = "https://merchant.example", "pid", "secret", "https://gateway.example"
	require.NoError(t, db.Create(&model.User{Id: 63, Username: "return-url-owner", Status: common.UserStatusEnabled}).Error)
	for _, target := range []string{"https://other.example/return", "//other.example/return"} {
		_, err := CreateGatewayCheckoutContext(context.Background(), 63, model.PaymentProviderEpay, GatewayCheckoutInput{Amount: "10", Method: "alipay", ReturnURL: target})
		require.ErrorContains(t, err, "返回地址")
	}
	var count int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&count).Error)
	require.Zero(t, count)
}
