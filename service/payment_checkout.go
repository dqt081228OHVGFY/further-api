package service

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/shopspring/decimal"
)

type GatewayCheckoutInput struct {
	Amount    string `json:"amount"`
	Method    string `json:"method"`
	ProductID string `json:"product_id,omitempty"`
	ReturnURL string `json:"return_url,omitempty"`
}
type GatewayCheckoutResult struct {
	TradeNo     string            `json:"trade_no"`
	URL         string            `json:"url"`
	Method      string            `json:"method"`
	Params      map[string]string `json:"params,omitempty"`
	WalletUnits string            `json:"wallet_units"`
	Money       float64           `json:"money"`
	Currency    string            `json:"currency"`
}
type GatewayCheckoutMethod struct {
	Provider  string `json:"provider"`
	Method    string `json:"method"`
	Name      string `json:"name"`
	Currency  string `json:"currency"`
	MinAmount int    `json:"min_amount"`
}

type GatewayCheckoutProduct struct {
	ProductID string  `json:"productId"`
	Price     float64 `json:"price"`
	Quota     float64 `json:"quota"`
	Currency  string  `json:"currency"`
}

func GatewayCheckoutProducts() []GatewayCheckoutProduct {
	result := make([]GatewayCheckoutProduct, 0)
	available := false
	for _, method := range GatewayCheckoutMethods() {
		available = available || method.Provider == model.PaymentProviderCreem
	}
	if !available {
		return result
	}
	for _, product := range gatewayCreemProducts() {
		currency := strings.ToUpper(product.Currency)
		if currency == "" {
			currency = "USD"
		}
		result = append(result, GatewayCheckoutProduct{ProductID: product.ProductID, Price: product.Price, Quota: product.Quota, Currency: currency})
	}
	return result
}

var ErrCheckoutUnavailable = errors.New("此支付渠道尚未配置或暂未启用。")
var checkoutEndpoints = struct{ stripe, creem string }{
	stripe: "https://api.stripe.com/v1/checkout/sessions", creem: "https://api.creem.io/v1/checkouts",
}

func GatewayCheckoutMethods() []GatewayCheckoutMethod {
	result := make([]GatewayCheckoutMethod, 0)
	if !operation_setting.IsPaymentComplianceConfirmed() {
		return result
	}
	if operation_setting.PayAddress != "" && operation_setting.EpayId != "" && operation_setting.EpayKey != "" && validCheckoutRate(operation_setting.Price) {
		for _, method := range operation_setting.PayMethods {
			result = append(result, GatewayCheckoutMethod{Provider: model.PaymentProviderEpay, Method: method["type"], Name: method["name"], Currency: "CNY", MinAmount: operation_setting.MinTopUp})
		}
	}
	if setting.StripeApiSecret != "" && setting.StripePriceId != "" && validCheckoutRate(setting.StripeUnitPrice) {
		result = append(result, GatewayCheckoutMethod{Provider: model.PaymentProviderStripe, Method: "stripe", Name: "银行卡", Currency: "USD", MinAmount: setting.StripeMinTopUp})
	}
	if setting.CreemApiKey != "" && len(gatewayCreemProducts()) > 0 {
		result = append(result, GatewayCheckoutMethod{Provider: model.PaymentProviderCreem, Method: "creem", Name: "银行卡", Currency: "USD", MinAmount: 1})
	}
	result = append(result, gatewayWaffoMethods()...)
	return result
}

func validCheckoutRate(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

type gatewayCreemProduct struct {
	ProductID string  `json:"productId"`
	Price     float64 `json:"price"`
	Quota     float64 `json:"quota"`
	Currency  string  `json:"currency"`
}

func gatewayCreemProducts() []gatewayCreemProduct {
	var products []gatewayCreemProduct
	if json.Unmarshal([]byte(setting.CreemProducts), &products) != nil {
		return nil
	}
	valid := products[:0]
	for _, product := range products {
		if product.ProductID != "" && product.Price > 0 && product.Quota > 0 {
			valid = append(valid, product)
		}
	}
	return valid
}

func CreateGatewayCheckoutContext(parent context.Context, userID int, provider string, input GatewayCheckoutInput) (GatewayCheckoutResult, error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return GatewayCheckoutResult{}, err
	}
	methodAvailable := false
	for _, method := range GatewayCheckoutMethods() {
		if method.Provider == provider && (input.Method == "" || method.Method == input.Method) {
			methodAvailable = true
			break
		}
	}
	if !methodAvailable {
		return GatewayCheckoutResult{}, ErrCheckoutUnavailable
	}
	units, err := decimal.NewFromString(strings.TrimSpace(input.Amount))
	if err != nil || !units.IsPositive() || units.Exponent() < -6 {
		return GatewayCheckoutResult{}, errors.New("充值额度必须为有效正数，最多保留 6 位小数。")
	}
	money, minimum, currency := decimal.Zero, 1, "USD"
	switch provider {
	case model.PaymentProviderEpay:
		currency = "CNY"
		if !operation_setting.ContainsPayMethod(input.Method) {
			return GatewayCheckoutResult{}, ErrCheckoutUnavailable
		}
		money, minimum = units.Mul(decimal.NewFromFloat(operation_setting.Price)), operation_setting.MinTopUp
	case model.PaymentProviderStripe:
		if !units.Equal(units.Truncate(0)) {
			return GatewayCheckoutResult{}, errors.New("银行卡充值额度必须为整数。")
		}
		money, minimum = units.Mul(decimal.NewFromFloat(setting.StripeUnitPrice)), setting.StripeMinTopUp
	case model.PaymentProviderCreem:
		found := false
		for _, product := range gatewayCreemProducts() {
			if product.ProductID == input.ProductID {
				units, money, found = decimal.NewFromFloat(product.Quota), decimal.NewFromFloat(product.Price), true
				if product.Currency != "" {
					currency = strings.ToUpper(product.Currency)
				}
				break
			}
		}
		if !found {
			return GatewayCheckoutResult{}, errors.New("充值产品未配置。")
		}
	case model.PaymentProviderWaffo:
		if setting.WaffoCurrency != "" {
			currency = strings.ToUpper(setting.WaffoCurrency)
		}
		money, minimum = units.Mul(decimal.NewFromFloat(setting.WaffoUnitPrice)), setting.WaffoMinTopUp
	case model.PaymentProviderWaffoPancake:
		money, minimum = units.Mul(decimal.NewFromFloat(setting.WaffoPancakeUnitPrice)), setting.WaffoPancakeMinTopUp
	default:
		return GatewayCheckoutResult{}, ErrCheckoutUnavailable
	}
	if units.LessThan(decimal.NewFromInt(int64(max(1, minimum)))) {
		return GatewayCheckoutResult{}, fmt.Errorf("充值额度不能低于 %d。", max(1, minimum))
	}
	if !money.IsPositive() {
		return GatewayCheckoutResult{}, errors.New("支付单价未正确配置。")
	}
	returnURL := PaymentReturnURL("/wallet")
	if input.ReturnURL != "" {
		approved, err := validateCheckoutReturnURL(input.ReturnURL)
		if err != nil {
			return GatewayCheckoutResult{}, err
		}
		returnURL = approved
	}
	order, err := model.CreateGatewayTopUpContext(ctx, userID, model.GatewayTopUpCreate{PaymentProvider: provider, PaymentMethod: input.Method, WalletUnits: units.String(), Money: money.Round(2).InexactFloat64(), PaymentCurrency: currency, AcceptsDiscounts: provider == model.PaymentProviderStripe && setting.StripePromotionCodesEnabled})
	if err != nil {
		return GatewayCheckoutResult{}, err
	}
	result := GatewayCheckoutResult{TradeNo: order.TradeNo, Method: http.MethodGet, WalletUnits: order.WalletUnits, Money: order.Money, Currency: currency}
	switch provider {
	case model.PaymentProviderEpay:
		params := map[string]string{"pid": operation_setting.EpayId, "type": input.Method, "out_trade_no": order.TradeNo, "notify_url": strings.TrimRight(GetCallbackAddress(), "/") + "/api/user/epay/notify", "return_url": returnURL, "name": "further-api 钱包充值", "money": money.Round(2).StringFixed(2)}
		keys := make([]string, 0, len(params))
		for key := range params {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var fields []string
		for _, key := range keys {
			fields = append(fields, key+"="+params[key])
		}
		hash := md5.Sum([]byte(strings.Join(fields, "&") + operation_setting.EpayKey))
		params["sign"], params["sign_type"] = hex.EncodeToString(hash[:]), "MD5"
		result.URL, result.Method, result.Params = strings.TrimRight(operation_setting.PayAddress, "/")+"/submit.php", http.MethodPost, params
		return result, nil
	case model.PaymentProviderStripe:
		body := url.Values{"mode": {"payment"}, "client_reference_id": {order.TradeNo}, "success_url": {returnURL}, "cancel_url": {returnURL}, "line_items[0][price]": {setting.StripePriceId}, "line_items[0][quantity]": {strconv.FormatInt(units.IntPart(), 10)}}
		if setting.StripePromotionCodesEnabled {
			body.Set("allow_promotion_codes", "true")
		}
		raw, err := checkoutHTTPRequest(ctx, checkoutEndpoints.stripe, "Bearer "+setting.StripeApiSecret, "application/x-www-form-urlencoded", []byte(body.Encode()), order.TradeNo)
		if err != nil {
			return result, err
		}
		result.URL, _ = raw["url"].(string)
	case model.PaymentProviderCreem:
		endpoint := checkoutEndpoints.creem
		if setting.CreemTestMode && endpoint == "https://api.creem.io/v1/checkouts" {
			endpoint = "https://test-api.creem.io/v1/checkouts"
		}
		payload, _ := json.Marshal(map[string]any{"product_id": input.ProductID, "request_id": order.TradeNo, "success_url": returnURL, "metadata": map[string]string{"trade_no": order.TradeNo, "user_id": strconv.Itoa(userID)}})
		raw, err := checkoutHTTPRequest(ctx, endpoint, setting.CreemApiKey, "application/json", payload, order.TradeNo)
		if err != nil {
			return result, err
		}
		result.URL, _ = raw["checkout_url"].(string)
	default:
		return createGatewayWaffoCheckout(ctx, userID, order, returnURL, input)
	}
	if result.URL == "" {
		return result, errors.New("支付服务未返回结算地址。")
	}
	return result, nil
}

func validateCheckoutReturnURL(input string) (string, error) {
	base, baseErr := url.Parse(PaymentReturnURL("/"))
	candidate, err := url.Parse(input)
	if err != nil || baseErr != nil || candidate.User != nil {
		return "", errors.New("付款返回地址无效。")
	}
	if !candidate.IsAbs() {
		candidate = base.ResolveReference(candidate)
	}
	if candidate.Scheme != base.Scheme || !strings.EqualFold(candidate.Host, base.Host) {
		return "", errors.New("付款返回地址必须属于当前站点。")
	}
	return candidate.String(), nil
}

func checkoutHTTPRequest(ctx context.Context, endpoint, authorization, contentType string, payload []byte, reference string) (map[string]any, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("User-Agent", "further-api/1.0")
	request.Header.Set("Idempotency-Key", reference)
	if strings.Contains(endpoint, "/v1/checkouts") {
		request.Header.Set("x-api-key", authorization)
	} else {
		request.Header.Set("Authorization", authorization)
	}
	client := GetHttpClient()
	if client == nil {
		return nil, errors.New("支付 HTTP 服务尚未初始化。")
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response == nil || response.Body == nil {
		return nil, errors.New("支付服务未返回有效响应。")
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("支付服务返回状态 %d。", response.StatusCode)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}
