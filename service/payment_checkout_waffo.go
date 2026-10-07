package service

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting"
	waffo "github.com/waffo-com/waffo-go"
	"github.com/waffo-com/waffo-go/config"
	"github.com/waffo-com/waffo-go/types/order"
	pancake "github.com/waffo-com/waffo-pancake-sdk-go"
)

func gatewayWaffoMethods() []GatewayCheckoutMethod {
	var result []GatewayCheckoutMethod
	key, private, cert := setting.WaffoApiKey, setting.WaffoPrivateKey, setting.WaffoPublicCert
	if setting.WaffoSandbox {
		key, private, cert = setting.WaffoSandboxApiKey, setting.WaffoSandboxPrivateKey, setting.WaffoSandboxPublicCert
	}
	if setting.WaffoEnabled && key != "" && private != "" && cert != "" && validCheckoutRate(setting.WaffoUnitPrice) {
		currency := setting.WaffoCurrency
		if currency == "" {
			currency = "USD"
		}
		for _, method := range setting.GetWaffoPayMethods() {
			result = append(result, GatewayCheckoutMethod{Provider: model.PaymentProviderWaffo, Method: method.PayMethodType, Name: method.Name, Currency: currency, MinAmount: setting.WaffoMinTopUp})
		}
	}
	if setting.WaffoPancakeMerchantID != "" && setting.WaffoPancakePrivateKey != "" && setting.WaffoPancakeProductID != "" && validCheckoutRate(setting.WaffoPancakeUnitPrice) {
		result = append(result, GatewayCheckoutMethod{Provider: model.PaymentProviderWaffoPancake, Method: "waffo_pancake", Name: "银行卡", Currency: "USD", MinAmount: setting.WaffoPancakeMinTopUp})
	}
	return result
}

func buildGatewayWaffoOrder(userID int, topup *model.TopUp, callback, back string, input GatewayCheckoutInput) *order.CreateOrderParams {
	currency := setting.WaffoCurrency
	if currency == "" {
		currency = "USD"
	}
	methodName := ""
	for _, method := range setting.GetWaffoPayMethods() {
		if method.PayMethodType == input.Method {
			methodName = method.PayMethodName
			break
		}
	}
	return &order.CreateOrderParams{
		PaymentRequestID: topup.TradeNo, MerchantOrderID: topup.TradeNo,
		OrderCurrency: currency, OrderAmount: strconv.FormatFloat(topup.Money, 'f', 2, 64),
		OrderDescription: "further-api 钱包充值", NotifyURL: callback,
		SuccessRedirectURL: back, FailedRedirectURL: back, CancelRedirectURL: back,
		UserInfo:    &order.UserInfo{UserID: strconv.Itoa(userID), UserTerminal: "WEB"},
		PaymentInfo: &order.PaymentInfo{ProductName: "further-api", PayMethodType: input.Method, PayMethodName: methodName},
	}
}

func createGatewayWaffoCheckout(ctx context.Context, userID int, topup *model.TopUp, back string, input GatewayCheckoutInput) (GatewayCheckoutResult, error) {
	result := GatewayCheckoutResult{TradeNo: topup.TradeNo, Method: "GET", WalletUnits: topup.WalletUnits, Money: topup.Money, Currency: topup.PaymentCurrency}
	if topup.PaymentProvider == model.PaymentProviderWaffoPancake {
		client, err := pancake.New(pancake.Config{MerchantID: setting.WaffoPancakeMerchantID, PrivateKey: setting.WaffoPancakePrivateKey, HTTPClient: GetHttpClient()})
		if err != nil {
			return result, err
		}
		response, err := client.Checkout.Authenticated.Create(ctx, pancake.AuthenticatedCheckoutParams{
			CreateCheckoutSessionParams: pancake.CreateCheckoutSessionParams{
				ProductID: setting.WaffoPancakeProductID, Currency: "USD",
				PriceSnapshot: &pancake.PriceInfo{Amount: strconv.FormatFloat(topup.Money, 'f', 2, 64), TaxCategory: pancake.TaxCategorySaaS},
				SuccessURL:    &back, OrderMerchantExternalID: &topup.TradeNo,
				Metadata: map[string]string{"gateway": "further-api", "trade_no": topup.TradeNo},
			},
			BuyerIdentity: WaffoPancakeBuyerIdentityFromUserID(userID),
		})
		if err != nil {
			return result, err
		}
		if response == nil || response.CheckoutURL == "" {
			return result, errors.New("支付服务未返回结算地址。")
		}
		result.URL = response.CheckoutURL
		return result, nil
	}
	env := config.Production
	key, private, cert := setting.WaffoApiKey, setting.WaffoPrivateKey, setting.WaffoPublicCert
	if setting.WaffoSandbox {
		env, key, private, cert = config.Sandbox, setting.WaffoSandboxApiKey, setting.WaffoSandboxPrivateKey, setting.WaffoSandboxPublicCert
	}
	builder := config.NewConfigBuilder().APIKey(key).PrivateKey(private).WaffoPublicKey(cert).Environment(env)
	if setting.WaffoMerchantId != "" {
		builder = builder.MerchantID(setting.WaffoMerchantId)
	}
	configuration, err := builder.Build()
	if err != nil {
		return result, err
	}
	callback := setting.WaffoNotifyUrl
	if callback == "" {
		callback = strings.TrimRight(GetCallbackAddress(), "/") + "/api/user/waffo/notify"
	}
	request := buildGatewayWaffoOrder(userID, topup, callback, back, input)
	response, err := waffo.New(configuration).Order().Create(ctx, request, nil)
	if err != nil {
		return result, err
	}
	if response == nil || !response.IsSuccess() || response.Data == nil {
		return result, errors.New("支付服务未接受结算订单。")
	}
	result.URL = response.Data.FetchRedirectURL()
	if result.URL == "" {
		return result, errors.New("支付服务未返回结算地址。")
	}
	return result, nil
}
