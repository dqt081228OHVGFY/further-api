package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/webhook"
)

type stripePaymentSession struct {
	Reference      string          `json:"client_reference_id"`
	Status         string          `json:"status"`
	PaymentStatus  string          `json:"payment_status"`
	Customer       json.RawMessage `json:"customer"`
	Currency       string          `json:"currency"`
	AmountTotal    *int64          `json:"amount_total"`
	AmountSubtotal *int64          `json:"amount_subtotal"`
}

func decodeStripePaymentSession(event stripe.Event) (stripePaymentSession, error) {
	var session stripePaymentSession
	if event.Data == nil {
		return session, errors.New("支付事件缺少订单信息")
	}
	raw := event.Data.Raw
	if len(raw) == 0 {
		var err error
		raw, err = json.Marshal(event.Data.Object)
		if err != nil {
			return session, err
		}
	}
	if err := json.Unmarshal(raw, &session); err != nil {
		return session, err
	}
	session.Reference = strings.TrimSpace(session.Reference)
	if session.Reference == "" {
		return session, model.ErrTopUpNotFound
	}
	return session, nil
}

func stripeReceiptCustomer(raw json.RawMessage) string {
	var id string
	if json.Unmarshal(raw, &id) == nil {
		return id
	}
	var expanded struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &expanded)
	return expanded.ID
}

func stripeSessionEvidence(session stripePaymentSession) *model.PaymentEvidence {
	amount := ""
	minor := session.AmountSubtotal
	if minor == nil {
		minor = session.AmountTotal
	}
	if minor != nil {
		amount = decimal.NewFromInt(*minor).Div(decimal.NewFromInt(100)).String()
	}
	return &model.PaymentEvidence{Amount: amount, Currency: session.Currency}
}

func processStripePayment(ctx context.Context, event stripe.Event, callerIP string) error {
	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted, stripe.EventTypeCheckoutSessionAsyncPaymentSucceeded,
		stripe.EventTypeCheckoutSessionExpired, stripe.EventTypeCheckoutSessionAsyncPaymentFailed:
	default:
		return nil
	}
	session, err := decodeStripePaymentSession(event)
	if err != nil {
		return err
	}
	LockOrder(session.Reference)
	defer UnlockOrder(session.Reference)
	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted, stripe.EventTypeCheckoutSessionAsyncPaymentSucceeded:
		if session.PaymentStatus != "paid" || session.Status != "complete" {
			return nil
		}
		receipt, _ := json.Marshal(map[string]any{"event_id": event.ID, "event_type": event.Type, "currency": session.Currency, "amount_total": session.AmountTotal})
		return model.CompleteGatewayPaymentContext(ctx, model.GatewayPaymentConfirmation{TradeNo: session.Reference, Provider: model.PaymentProviderStripe, CustomerID: stripeReceiptCustomer(session.Customer), Receipt: string(receipt), CallerIP: callerIP, Evidence: stripeSessionEvidence(session)})
	case stripe.EventTypeCheckoutSessionExpired:
		if session.Status != "expired" {
			return nil
		}
		err = model.TransitionGatewayPaymentContext(ctx, session.Reference, model.PaymentProviderStripe, common.TopUpStatusExpired)
	case stripe.EventTypeCheckoutSessionAsyncPaymentFailed:
		err = model.TransitionGatewayPaymentContext(ctx, session.Reference, model.PaymentProviderStripe, common.TopUpStatusFailed)
	}
	if paymentTerminalEventHandled(err) {
		return nil
	}
	return err
}

func StripeWebhook(c *gin.Context) {
	if !isStripeWebhookEnabled() {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	payload, err := readPaymentWebhook(c)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	event, err := webhook.ConstructEventWithOptions(payload, c.GetHeader("Stripe-Signature"), setting.StripeWebhookSecret, webhook.ConstructEventOptions{IgnoreAPIVersionMismatch: true})
	if err != nil {
		logger.LogWarn(c.Request.Context(), "Stripe 回调签名验证未通过")
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if err := processStripePayment(c.Request.Context(), event, c.ClientIP()); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 回调结算失败 event_id=%s event_type=%s error=%q", event.ID, event.Type, err.Error()))
		c.AbortWithStatus(paymentWebhookErrorStatus(err))
		return
	}
	c.Status(http.StatusOK)
}
