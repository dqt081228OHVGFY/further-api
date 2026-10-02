package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/model"

	pancake "github.com/waffo-com/waffo-pancake-sdk-go"
)

// WaffoPancakeWebhookEvent mirrors the SDK's WebhookEvent shape using plain
// strings so controllers don't have to import the SDK package.
type WaffoPancakeWebhookEvent struct {
	ID        string
	Timestamp string
	EventType string
	EventID   string
	StoreID   string
	Mode      string
	Data      WaffoPancakeWebhookData
}

type WaffoPancakeWebhookData struct {
	// OrderID = Pancake ORD_* (logs); OrderMerchantExternalID = our trade_no (lookup).
	OrderID                       string
	OrderMerchantExternalID       string
	BuyerEmail                    string
	Currency                      string
	Amount                        string
	TaxAmount                     string
	ProductName                   string
	MerchantProvidedBuyerIdentity string
}

// NormalizedEventType returns the event type or empty string for a nil event.
func (e *WaffoPancakeWebhookEvent) NormalizedEventType() string {
	if e == nil {
		return ""
	}
	return e.EventType
}

// WaffoPancakeBuyerIdentityFromUserID renders the canonical buyer identity
// for checkout. Webhook handlers compare against the value rendered here to
// reject identity mismatches, so both call sites must use this function.
func WaffoPancakeBuyerIdentityFromUserID(userID int) string {
	return fmt.Sprintf("new-api-user-%d", userID)
}

// VerifyConfiguredWaffoPancakeWebhook verifies the signature header. The SDK
// picks the matching test / prod public key from the payload's `mode` field.
func VerifyConfiguredWaffoPancakeWebhook(payload string, signatureHeader string) (*WaffoPancakeWebhookEvent, error) {
	evt, err := pancake.VerifyWebhookTyped[pancake.WebhookEventData](payload, signatureHeader, nil)
	if err != nil {
		return nil, err
	}
	identity := ""
	if evt.Data.MerchantProvidedBuyerIdentity != nil {
		identity = *evt.Data.MerchantProvidedBuyerIdentity
	}
	externalID := ""
	if evt.Data.OrderMerchantExternalID != nil {
		externalID = *evt.Data.OrderMerchantExternalID
	}
	return &WaffoPancakeWebhookEvent{
		ID:        evt.ID,
		Timestamp: evt.Timestamp,
		EventType: evt.EventType,
		EventID:   evt.EventID,
		StoreID:   evt.StoreID,
		Mode:      string(evt.Mode),
		Data: WaffoPancakeWebhookData{
			OrderID:                       evt.Data.OrderID,
			OrderMerchantExternalID:       externalID,
			BuyerEmail:                    evt.Data.BuyerEmail,
			Currency:                      evt.Data.Currency,
			Amount:                        evt.Data.Amount,
			TaxAmount:                     evt.Data.TaxAmount,
			ProductName:                   evt.Data.ProductName,
			MerchantProvidedBuyerIdentity: identity,
		},
	}, nil
}

// ResolveWaffoPancakeTradeNo maps a verified webhook event to a local TopUp
// trade_no via OrderMerchantExternalID, and rejects buyer-identity mismatches.
func ResolveWaffoPancakeTradeNo(event *WaffoPancakeWebhookEvent) (string, error) {
	if event == nil {
		return "", fmt.Errorf("missing webhook event")
	}
	tradeNo := strings.TrimSpace(event.Data.OrderMerchantExternalID)
	if tradeNo == "" {
		return "", fmt.Errorf("missing webhook orderMerchantExternalId")
	}
	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil || topUp.PaymentProvider != model.PaymentProviderWaffoPancake {
		return "", fmt.Errorf("waffo pancake order not found for tradeNo=%s", tradeNo)
	}
	expectedIdentity := WaffoPancakeBuyerIdentityFromUserID(topUp.UserId)
	actualIdentity := strings.TrimSpace(event.Data.MerchantProvidedBuyerIdentity)
	if actualIdentity != expectedIdentity {
		return "", fmt.Errorf(
			"waffo pancake buyer identity mismatch for tradeNo=%s: expected=%q actual=%q",
			tradeNo,
			expectedIdentity,
			actualIdentity,
		)
	}
	return tradeNo, nil
}

// ResolveWaffoPancakeSubscriptionTradeNo is the SubscriptionOrder counterpart
// of ResolveWaffoPancakeTradeNo.
func ResolveWaffoPancakeSubscriptionTradeNo(event *WaffoPancakeWebhookEvent) (string, error) {
	if event == nil {
		return "", fmt.Errorf("missing webhook event")
	}
	tradeNo := strings.TrimSpace(event.Data.OrderMerchantExternalID)
	if tradeNo == "" {
		return "", fmt.Errorf("missing webhook orderMerchantExternalId")
	}
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	if order == nil || order.PaymentProvider != model.PaymentProviderWaffoPancake {
		return "", fmt.Errorf("waffo pancake subscription order not found for tradeNo=%s", tradeNo)
	}
	expectedIdentity := WaffoPancakeBuyerIdentityFromUserID(order.UserId)
	actualIdentity := strings.TrimSpace(event.Data.MerchantProvidedBuyerIdentity)
	if actualIdentity != expectedIdentity {
		return "", fmt.Errorf(
			"waffo pancake buyer identity mismatch for subscription tradeNo=%s: expected=%q actual=%q",
			tradeNo,
			expectedIdentity,
			actualIdentity,
		)
	}
	return tradeNo, nil
}
