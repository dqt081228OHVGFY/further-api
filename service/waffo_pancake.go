package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/model"
	pancake "github.com/waffo-com/waffo-pancake-sdk-go"
)

type WaffoPancakeWebhookEvent struct {
	ID, Timestamp, EventType, EventID, StoreID, Mode string
	Data                                             WaffoPancakeWebhookData
}

type WaffoPancakeWebhookData struct {
	OrderID, OrderMerchantExternalID, BuyerEmail, Currency, Amount, TaxAmount, Subtotal, Total, ProductName, MerchantProvidedBuyerIdentity string
}

func (event *WaffoPancakeWebhookEvent) NormalizedEventType() string {
	if event == nil {
		return ""
	}
	return strings.TrimSpace(event.EventType)
}

func WaffoPancakeBuyerIdentityFromUserID(id int) string {
	return fmt.Sprintf("further-api-user-%d", id)
}

func matchesWaffoBuyerIdentity(identity string, id int) bool {
	if id <= 0 {
		return false
	}
	return identity == WaffoPancakeBuyerIdentityFromUserID(id) || identity == fmt.Sprintf("new-api-user-%d", id)
}

func pancakeOptionalText(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func VerifyConfiguredWaffoPancakeWebhook(payload, signature string) (*WaffoPancakeWebhookEvent, error) {
	if len(payload) > 2<<20 || strings.TrimSpace(payload) == "" || strings.TrimSpace(signature) == "" {
		return nil, errors.New("支付回调请求无效。")
	}
	verified, err := pancake.VerifyWebhookTyped[pancake.WebhookEventData](payload, signature, nil)
	if err != nil {
		return nil, err
	}
	data := verified.Data
	event := &WaffoPancakeWebhookEvent{ID: verified.ID, Timestamp: verified.Timestamp, EventType: verified.EventType, EventID: verified.EventID, StoreID: verified.StoreID, Mode: string(verified.Mode)}
	event.Data = WaffoPancakeWebhookData{OrderID: data.OrderID, OrderMerchantExternalID: pancakeOptionalText(data.OrderMerchantExternalID),
		BuyerEmail: data.BuyerEmail, Currency: data.Currency, Amount: data.Amount, TaxAmount: data.TaxAmount,
		Subtotal: pancakeOptionalText(data.Subtotal), Total: pancakeOptionalText(data.Total), ProductName: data.ProductName,
		MerchantProvidedBuyerIdentity: pancakeOptionalText(data.MerchantProvidedBuyerIdentity)}
	return event, nil
}

func ResolveWaffoPancakeTradeNo(event *WaffoPancakeWebhookEvent) (string, error) {
	return ResolveWaffoPancakeTradeNoContext(context.Background(), event)
}
func ResolveWaffoPancakeTradeNoContext(ctx context.Context, event *WaffoPancakeWebhookEvent) (string, error) {
	return resolvePancakeOrderOwner(ctx, event, false)
}
func ResolveWaffoPancakeSubscriptionTradeNo(event *WaffoPancakeWebhookEvent) (string, error) {
	return ResolveWaffoPancakeSubscriptionTradeNoContext(context.Background(), event)
}
func ResolveWaffoPancakeSubscriptionTradeNoContext(ctx context.Context, event *WaffoPancakeWebhookEvent) (string, error) {
	return resolvePancakeOrderOwner(ctx, event, true)
}

func resolvePancakeOrderOwner(ctx context.Context, event *WaffoPancakeWebhookEvent, subscription bool) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if event == nil || model.DB == nil {
		return "", errors.New("支付回调订单信息不可用。")
	}
	reference := strings.TrimSpace(event.Data.OrderMerchantExternalID)
	if reference == "" || len(reference) > 128 {
		return "", errors.New("支付回调缺少有效订单关联。")
	}
	identity := strings.TrimSpace(event.Data.MerchantProvidedBuyerIdentity)
	query := model.DB.WithContext(ctx)
	if subscription {
		query = query.Model(&model.SubscriptionOrder{})
	} else {
		query = query.Model(&model.TopUp{})
	}
	var owner struct {
		UserID   int    `gorm:"column:user_id"`
		Provider string `gorm:"column:payment_provider"`
	}
	if err := query.Select("user_id", "payment_provider").Where("trade_no = ?", reference).Take(&owner).Error; err != nil {
		return "", errors.New("支付回调订单不存在。")
	}
	if owner.Provider != model.PaymentProviderWaffoPancake || !matchesWaffoBuyerIdentity(identity, owner.UserID) {
		return "", errors.New("支付回调订单所属账户不匹配。")
	}
	return reference, nil
}
