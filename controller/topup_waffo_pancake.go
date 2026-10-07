package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

var errPancakeEnvironment = errors.New("支付回调环境不匹配")
var errPancakeReference = errors.New("支付回调账户或订单不匹配")

func pancakePaymentEvidence(event *service.WaffoPancakeWebhookEvent) *model.PaymentEvidence {
	amount := strings.TrimSpace(event.Data.Subtotal)
	if amount == "" {
		amount = strings.TrimSpace(event.Data.Amount)
	}
	if amount == "" && event.Data.Total != "" {
		paid, paidErr := decimal.NewFromString(event.Data.Total)
		tax, taxErr := decimal.NewFromString(event.Data.TaxAmount)
		if paidErr == nil && taxErr == nil && tax.Sign() >= 0 {
			amount = paid.Sub(tax).String()
		}
	}
	return &model.PaymentEvidence{Amount: amount, Currency: event.Data.Currency}
}

func processWaffoPancakePayment(ctx context.Context, event *service.WaffoPancakeWebhookEvent, expectedEnvironment, receipt, callerIP string) error {
	if event == nil {
		return errPancakeReference
	}
	if !strings.EqualFold(strings.TrimSpace(event.Mode), expectedEnvironment) {
		return errPancakeEnvironment
	}
	if event.NormalizedEventType() != "order.completed" {
		return nil
	}
	tradeNo := strings.TrimSpace(event.Data.OrderMerchantExternalID)
	// Stored order type determines dispatch, including historical trade numbers.
	reference, err := model.FindGatewayPaymentReferenceContext(ctx, tradeNo, model.PaymentProviderWaffoPancake)
	if err != nil {
		if errors.Is(err, model.ErrTopUpNotFound) || errors.Is(err, model.ErrPaymentMethodMismatch) {
			return errPancakeReference
		}
		return err
	}
	if reference.Subscription {
		_, err = service.ResolveWaffoPancakeSubscriptionTradeNoContext(ctx, event)
	} else {
		_, err = service.ResolveWaffoPancakeTradeNoContext(ctx, event)
	}
	if err != nil {
		return errPancakeReference
	}
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)
	return model.CompleteGatewayPaymentContext(ctx, model.GatewayPaymentConfirmation{TradeNo: tradeNo, Provider: model.PaymentProviderWaffoPancake, Receipt: receipt, CallerIP: callerIP, Evidence: pancakePaymentEvidence(event)})
}

func WaffoPancakeWebhook(c *gin.Context) {
	if !isWaffoPancakeWebhookEnabled() {
		c.String(http.StatusForbidden, "webhook disabled")
		return
	}
	expectedEnvironment := strings.TrimSpace(c.Param("env"))
	if expectedEnvironment != "test" && expectedEnvironment != "prod" {
		c.String(http.StatusNotFound, "unknown env")
		return
	}
	payload, err := readPaymentWebhook(c)
	if err != nil {
		c.String(http.StatusBadRequest, "bad request")
		return
	}
	event, err := service.VerifyConfiguredWaffoPancakeWebhook(string(payload), c.GetHeader("X-Waffo-Signature"))
	if err != nil {
		logger.LogWarn(c.Request.Context(), "Waffo Pancake 回调签名验证未通过")
		c.String(http.StatusUnauthorized, "invalid signature")
		return
	}
	if err := processWaffoPancakePayment(c.Request.Context(), event, expectedEnvironment, string(payload), c.ClientIP()); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 回调结算失败 event_id=%s error=%q", event.ID, err.Error()))
		if errors.Is(err, errPancakeEnvironment) || errors.Is(err, errPancakeReference) {
			c.String(http.StatusOK, "OK")
			return
		}
		c.String(http.StatusInternalServerError, "retry")
		return
	}
	c.String(http.StatusOK, "OK")
}
