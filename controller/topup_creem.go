package controller

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const CreemSignatureHeader = "creem-signature"

type CreemWebhookEvent struct {
	Id        string               `json:"id"`
	EventType string               `json:"eventType"`
	CreatedAt int64                `json:"created_at"`
	Object    CreemCheckoutReceipt `json:"object"`
}

type CreemCheckoutReceipt struct {
	Id        string               `json:"id"`
	Object    string               `json:"object"`
	RequestId string               `json:"request_id"`
	Order     CreemOrderReceipt    `json:"order"`
	Product   CreemProductReceipt  `json:"product"`
	Customer  CreemCustomerReceipt `json:"customer"`
	Units     int                  `json:"units"`
	Status    string               `json:"status"`
	Metadata  map[string]string    `json:"metadata"`
	Mode      string               `json:"mode"`
}

type CreemOrderReceipt struct {
	Id          string `json:"id"`
	Object      string `json:"object"`
	Customer    string `json:"customer"`
	Product     string `json:"product"`
	Amount      int    `json:"amount"`
	Currency    string `json:"currency"`
	SubTotal    int    `json:"sub_total"`
	TaxAmount   int    `json:"tax_amount"`
	AmountDue   int    `json:"amount_due"`
	AmountPaid  int    `json:"amount_paid"`
	Status      string `json:"status"`
	Type        string `json:"type"`
	Transaction string `json:"transaction"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Mode        string `json:"mode"`
}

type CreemProductReceipt struct {
	Id                string  `json:"id"`
	Object            string  `json:"object"`
	Name              string  `json:"name"`
	Description       string  `json:"description"`
	Price             int     `json:"price"`
	Currency          string  `json:"currency"`
	BillingType       string  `json:"billing_type"`
	BillingPeriod     string  `json:"billing_period"`
	Status            string  `json:"status"`
	TaxMode           string  `json:"tax_mode"`
	TaxCategory       string  `json:"tax_category"`
	DefaultSuccessUrl *string `json:"default_success_url"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
	Mode              string  `json:"mode"`
}

type CreemCustomerReceipt struct {
	Id        string `json:"id"`
	Object    string `json:"object"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Country   string `json:"country"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Mode      string `json:"mode"`
}

func generateCreemSignature(payload, secret string) string {
	digest := hmac.New(sha256.New, []byte(secret))
	_, _ = digest.Write([]byte(payload))
	return hex.EncodeToString(digest.Sum(nil))
}

func verifyCreemSignature(payload, signature, secret string) bool {
	if secret == "" {
		return false
	}
	provided, err := hex.DecodeString(strings.TrimSpace(signature))
	if err != nil {
		return false
	}
	expected, _ := hex.DecodeString(generateCreemSignature(payload, secret))
	return hmac.Equal(provided, expected)
}

func creemPaymentEvidence(order CreemOrderReceipt) *model.PaymentEvidence {
	amount := order.SubTotal
	if amount == 0 {
		amount = order.Amount
	}
	if amount == 0 {
		amount = order.AmountPaid - order.TaxAmount
	}
	value := ""
	if amount != 0 {
		value = decimal.NewFromInt(int64(amount)).Div(decimal.NewFromInt(100)).String()
	}
	return &model.PaymentEvidence{Amount: value, Currency: order.Currency}
}

func processCreemPayment(ctx context.Context, event *CreemWebhookEvent, callerIP string) error {
	if event == nil {
		return errors.New("支付事件为空")
	}
	if event.EventType != "checkout.completed" || event.Object.Order.Status != "paid" {
		return nil
	}
	tradeNo := strings.TrimSpace(event.Object.RequestId)
	if tradeNo == "" {
		return model.ErrTopUpNotFound
	}
	receipt, _ := json.Marshal(map[string]any{"event_id": event.Id, "event_type": event.EventType, "order_id": event.Object.Order.Id, "currency": event.Object.Order.Currency, "amount_paid": event.Object.Order.AmountPaid})
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)
	err := model.CompleteGatewayPaymentContext(ctx, model.GatewayPaymentConfirmation{TradeNo: tradeNo, Provider: model.PaymentProviderCreem, CustomerEmail: event.Object.Customer.Email, Receipt: string(receipt), CallerIP: callerIP, Evidence: creemPaymentEvidence(event.Object.Order), SubscriptionOnly: event.Object.Order.Type != "onetime"})
	if event.Object.Order.Type != "onetime" && errors.Is(err, model.ErrSubscriptionOrderNotFound) {
		return nil
	}
	return err
}

func CreemWebhook(c *gin.Context) {
	if !isCreemWebhookEnabled() {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	payload, err := readPaymentWebhook(c)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if !verifyCreemSignature(string(payload), c.GetHeader(CreemSignatureHeader), setting.CreemWebhookSecret) {
		logger.LogWarn(c.Request.Context(), "Creem 回调签名验证未通过")
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var event CreemWebhookEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	handleCheckoutCompleted(c, &event)
}

func handleCheckoutCompleted(c *gin.Context, event *CreemWebhookEvent) {
	if err := processCreemPayment(c.Request.Context(), event, c.ClientIP()); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 回调结算失败 error=%q", err.Error()))
		c.AbortWithStatus(paymentWebhookErrorStatus(err))
		return
	}
	c.Status(http.StatusOK)
}
