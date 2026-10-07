package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81/webhook"
	"github.com/waffo-com/waffo-go/core"
	"gorm.io/gorm"
)

func callbackWalletFixture(t *testing.T, provider string) (*gorm.DB, *model.TopUp) {
	t.Helper()
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}))
	user := model.User{Username: "callback-owner", AffCode: "callback-owner"}
	require.NoError(t, db.Create(&user).Error)
	order, err := model.CreateGatewayTopUpContext(context.Background(), user.Id, model.GatewayTopUpCreate{PaymentProvider: provider, WalletUnits: "2", Money: 2, PaymentCurrency: "USD"})
	require.NoError(t, err)
	return db, order
}

func stripeCallbackRequest(t *testing.T, tradeNo, paymentStatus, currency string, minorAmount int, secret string) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"id": "evt_fixture", "object": "event", "type": "checkout.session.completed", "data": map[string]any{"object": map[string]any{"client_reference_id": tradeNo, "status": "complete", "payment_status": paymentStatus, "currency": currency, "amount_total": minorAmount, "customer": "cus_fixture"}}})
	require.NoError(t, err)
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: payload, Secret: secret})
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/stripe/webhook", strings.NewReader(string(payload)))
	c.Request.Header.Set("Stripe-Signature", signed.Header)
	StripeWebhook(c)
	return response
}

func TestStripeSignedCallbackWaitsForPaymentAndCreditsOnlyOnce(t *testing.T) {
	db, order := callbackWalletFixture(t, model.PaymentProviderStripe)
	oldSecret := setting.StripeWebhookSecret
	setting.StripeWebhookSecret = "fixture-secret"
	t.Cleanup(func() { setting.StripeWebhookSecret = oldSecret })
	require.Equal(t, http.StatusOK, stripeCallbackRequest(t, order.TradeNo, "unpaid", "usd", 200, "fixture-secret").Code)
	require.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(order.TradeNo).Status)
	require.Equal(t, http.StatusBadRequest, stripeCallbackRequest(t, order.TradeNo, "paid", "usd", 200, "wrong-secret").Code)
	require.Equal(t, http.StatusBadRequest, stripeCallbackRequest(t, order.TradeNo, "paid", "eur", 200, "fixture-secret").Code)
	require.Equal(t, http.StatusBadRequest, stripeCallbackRequest(t, order.TradeNo, "paid", "usd", 100, "fixture-secret").Code)
	for range 2 {
		require.Equal(t, http.StatusOK, stripeCallbackRequest(t, order.TradeNo, "paid", "usd", 200, "fixture-secret").Code)
	}
	var user model.User
	require.NoError(t, db.First(&user, order.UserId).Error)
	require.Equal(t, order.CreditedQuota, user.Quota)
	require.Equal(t, "cus_fixture", user.StripeCustomer)
}

func TestStripeSignedCallbackRequestsRetryAfterDatabaseFailure(t *testing.T) {
	db, order := callbackWalletFixture(t, model.PaymentProviderStripe)
	oldSecret := setting.StripeWebhookSecret
	setting.StripeWebhookSecret = "fixture-secret"
	t.Cleanup(func() { setting.StripeWebhookSecret = oldSecret })
	callback := "payment-fixture-failure"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			tx.AddError(errors.New("fixture database unavailable"))
		}
	}))
	require.Equal(t, http.StatusInternalServerError, stripeCallbackRequest(t, order.TradeNo, "paid", "usd", 200, "fixture-secret").Code)
	require.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(order.TradeNo).Status)
	require.NoError(t, db.Callback().Update().Remove(callback))
	require.Equal(t, http.StatusOK, stripeCallbackRequest(t, order.TradeNo, "paid", "usd", 200, "fixture-secret").Code)
}

func TestCreemSignedCallbackCreditsOnlyItsConfiguredGatewayOrder(t *testing.T) {
	db, order := callbackWalletFixture(t, model.PaymentProviderCreem)
	oldSecret := setting.CreemWebhookSecret
	setting.CreemWebhookSecret = "fixture-creem"
	t.Cleanup(func() { setting.CreemWebhookSecret = oldSecret })
	event := CreemWebhookEvent{Id: "evt_creem", EventType: "checkout.completed", Object: CreemCheckoutReceipt{RequestId: order.TradeNo, Order: CreemOrderReceipt{Status: "paid", Type: "onetime", Amount: 200, AmountPaid: 220, TaxAmount: 20, Currency: "USD"}, Customer: CreemCustomerReceipt{Email: "buyer@example.invalid"}}}
	call := func(secret string) int {
		payload, err := json.Marshal(event)
		require.NoError(t, err)
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/user/creem/webhook", strings.NewReader(string(payload)))
		c.Request.Header.Set(CreemSignatureHeader, generateCreemSignature(string(payload), secret))
		CreemWebhook(c)
		return response.Code
	}
	require.Equal(t, http.StatusUnauthorized, call("wrong-secret"))
	require.Equal(t, http.StatusOK, call("fixture-creem"))
	require.Equal(t, http.StatusOK, call("fixture-creem"))
	var user model.User
	require.NoError(t, db.First(&user, order.UserId).Error)
	require.Equal(t, order.CreditedQuota, user.Quota)
	require.Equal(t, "buyer@example.invalid", user.Email)
}

func TestWaffoPendingNotificationsRemainPayableUntilSuccess(t *testing.T) {
	db, order := callbackWalletFixture(t, model.PaymentProviderWaffo)
	for _, status := range []string{core.OrderStatusPayInProgress, core.OrderStatusAuthorizationRequired, core.OrderStatusAuthedWaitingCapture, "CAPTURE_IN_PROGRESS"} {
		require.NoError(t, processWaffoPayment(context.Background(), &core.PaymentNotificationResult{MerchantOrderID: order.TradeNo, OrderStatus: status}, ""))
		require.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(order.TradeNo).Status)
	}
	paid := &core.PaymentNotificationResult{MerchantOrderID: order.TradeNo, OrderStatus: core.OrderStatusPaySuccess, OrderAmount: "2.00", OrderCurrency: "USD"}
	for range 2 {
		require.NoError(t, processWaffoPayment(context.Background(), paid, ""))
	}
	require.NoError(t, processWaffoPayment(context.Background(), &core.PaymentNotificationResult{MerchantOrderID: order.TradeNo, OrderStatus: core.OrderStatusOrderClose}, ""))
	var user model.User
	require.NoError(t, db.First(&user, order.UserId).Error)
	require.Equal(t, order.CreditedQuota, user.Quota)
	require.Equal(t, common.TopUpStatusSuccess, model.GetTopUpByTradeNo(order.TradeNo).Status)
}

func TestPancakeVerifiedReceiptUsesStoredOrderAndRejectsWrongOwner(t *testing.T) {
	db, order := callbackWalletFixture(t, model.PaymentProviderWaffoPancake)
	event := &service.WaffoPancakeWebhookEvent{ID: "evt_pancake", EventType: "order.completed", Mode: "test", Data: service.WaffoPancakeWebhookData{OrderMerchantExternalID: order.TradeNo, MerchantProvidedBuyerIdentity: service.WaffoPancakeBuyerIdentityFromUserID(order.UserId), Amount: "2", Currency: "USD"}}
	require.ErrorIs(t, processWaffoPancakePayment(context.Background(), event, "prod", "", ""), errPancakeEnvironment)
	event.Data.MerchantProvidedBuyerIdentity = "another-account"
	require.ErrorIs(t, processWaffoPancakePayment(context.Background(), event, "test", "", ""), errPancakeReference)
	require.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(order.TradeNo).Status)
	event.Data.MerchantProvidedBuyerIdentity = service.WaffoPancakeBuyerIdentityFromUserID(order.UserId)
	for range 2 {
		require.NoError(t, processWaffoPancakePayment(context.Background(), event, "test", "", ""))
	}
	var user model.User
	require.NoError(t, db.First(&user, order.UserId).Error)
	require.Equal(t, order.CreditedQuota, user.Quota)
}

func TestPaymentWebhookOversizedBodyIsRejectedBeforeParsing(t *testing.T) {
	oldSecret := setting.StripeWebhookSecret
	setting.StripeWebhookSecret = "fixture-secret"
	t.Cleanup(func() { setting.StripeWebhookSecret = oldSecret })
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/stripe/webhook", strings.NewReader(strings.Repeat("x", int(paymentWebhookBodyLimit)+1)))
	StripeWebhook(c)
	require.Equal(t, http.StatusBadRequest, response.Code)
}
