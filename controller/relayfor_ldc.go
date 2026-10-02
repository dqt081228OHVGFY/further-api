package controller

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const relayForLDCCheckoutURL = "https://credit.linux.do/epay/pay/submit.php"
const relayForLDCQueryURL = "https://credit.linux.do/epay/api.php"

var relayForLDCHTTPClient = &http.Client{
	Timeout: 12 * time.Second,
	// Never forward a merchant secret to a redirected host.
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
}

func relayForLDCCallbackURLs(cfg model.RelayForLDCSettings) (string, string, error) {
	base := cfg.CallbackBaseURL
	if base == "" {
		base = strings.TrimRight(system_setting.ServerAddress, "/")
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", "", errors.New("请配置公开可访问的 HTTPS 站点地址或 LDC 回调基础地址")
	}
	notify, back := base+"/api/ldc/notify", base+"/api/ldc/return"
	if len(notify) > 100 || len(back) > 100 {
		return "", "", errors.New("LDC 回调地址不可超过 100 个字符")
	}
	return notify, back, nil
}

func relayForLDCConfigured(cfg model.RelayForLDCSettings) bool {
	_, _, err := relayForLDCCallbackURLs(cfg)
	_, keyErr := model.ParseRelayForLDCPrivateKey(cfg.PrivateKey)
	return cfg.ClientID != "" && cfg.ClientSecret != "" && err == nil && keyErr == nil
}

func relayForLDCConfigView(cfg model.RelayForLDCSettings) gin.H {
	notify, back, _ := relayForLDCCallbackURLs(cfg)
	public, publicPEM := model.RelayForLDCPublicKeys(cfg.PrivateKey)
	return gin.H{"enabled": cfg.Enabled, "configured": relayForLDCConfigured(cfg), "client_id": cfg.ClientID, "secret_configured": cfg.ClientSecret != "", "private_key_configured": public != "", "public_key": public, "public_key_pem": publicPEM, "credits_per_unit": cfg.CreditsPerUnit, "min_topup": cfg.MinTopUp, "callback_base_url": cfg.CallbackBaseURL, "notify_url": notify, "return_url": back, "currency": "LDC", "wallet_unit": "USD"}
}

func RelayForGetLDCConfig(c *gin.Context) {
	cfg, err := model.GetRelayForLDCSettings()
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, relayForLDCConfigView(cfg))
}

func RelayForSaveLDCConfig(c *gin.Context) {
	var body struct {
		Enabled         bool   `json:"enabled"`
		ClientID        string `json:"client_id"`
		ClientSecret    string `json:"client_secret"`
		PrivateKey      string `json:"private_key"`
		CreditsPerUnit  string `json:"credits_per_unit"`
		MinTopUp        string `json:"min_topup"`
		CallbackBaseURL string `json:"callback_base_url"`
	}
	if !relayForBusinessJSON(c, &body) {
		return
	}
	cfg := model.RelayForLDCSettings{Enabled: body.Enabled, ClientID: body.ClientID, ClientSecret: body.ClientSecret, PrivateKey: body.PrivateKey, CreditsPerUnit: body.CreditsPerUnit, MinTopUp: body.MinTopUp, CallbackBaseURL: body.CallbackBaseURL}
	if cfg.Enabled {
		if _, _, err := relayForLDCCallbackURLs(cfg); err != nil {
			relayForBusinessError(c, err)
			return
		}
	}
	cfg, err := model.SaveRelayForLDCSettings(cfg)
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, 0, "relayfor.ldc.configure", map[string]any{"enabled": cfg.Enabled, "credits_per_unit": cfg.CreditsPerUnit, "min_topup": cfg.MinTopUp})
	common.ApiSuccess(c, relayForLDCConfigView(cfg))
}

func RelayForLDCInfo(c *gin.Context) {
	cfg, err := model.GetRelayForLDCSettings()
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	maximum := "0"
	if common.QuotaPerUnit > 0 {
		limit := decimal.NewFromInt(common.MaxWalletQuota).Div(decimal.NewFromFloat(common.QuotaPerUnit))
		if rate, err := decimal.NewFromString(cfg.CreditsPerUnit); err == nil && rate.GreaterThan(decimal.Zero) {
			limit = decimal.Min(limit, decimal.NewFromInt(1000000000).Div(rate))
		}
		maximum = limit.Truncate(2).String()
	}
	configured := relayForLDCConfigured(cfg)
	common.ApiSuccess(c, gin.H{"enabled": cfg.Enabled && configured, "configured": configured, "currency": "LDC", "wallet_unit": "USD", "payment_method": "ldc", "credits_per_unit": cfg.CreditsPerUnit, "min_topup": cfg.MinTopUp, "max_topup": maximum, "amount_options": []string{"1", "10", "50", "100"}})
}

type relayForLDCPayRequest struct {
	Amount          json.Number `json:"amount"`
	PlanID          int         `json:"plan_id"`
	ExpectedCredits string      `json:"expected_credits"`
}

func RelayForLDCQuote(c *gin.Context) {
	var req relayForLDCPayRequest
	if !relayForBusinessJSON(c, &req) {
		return
	}
	cfg, err := model.GetRelayForLDCSettings()
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	if !relayForLDCConfigured(cfg) || req.PlanID < 0 {
		common.ApiErrorMsg(c, "LDC 支付配置未完成或参数无效")
		return
	}
	quote, err := model.QuoteRelayForLDC(c.GetInt("id"), req.Amount.String(), req.PlanID, cfg)
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, quote)
}

// The native LDC protocol signs sorted non-empty form fields plus the merchant
// secret using Ed25519. No legacy payment provider participates in checkout.
func relayForLDCForm(order *model.RelayForLDCOrder, cfg model.RelayForLDCSettings) (map[string]string, error) {
	notify, back, err := relayForLDCCallbackURLs(cfg)
	if err != nil {
		return nil, err
	}
	key, err := model.ParseRelayForLDCPrivateKey(cfg.PrivateKey)
	if err != nil {
		return nil, err
	}
	name := "RelayFor 钱包充值"
	if order.Kind == "subscription" {
		name = "RelayFor 订阅 · " + order.PlanTitle
	}
	if len([]rune(name)) > 64 {
		name = string([]rune(name)[:64])
	}
	params := map[string]string{"client_id": cfg.ClientID, "type": "ldcpay", "out_trade_no": order.TradeNo, "money": order.Credits, "order_name": name, "notify_url": notify, "return_url": back}
	keys := make([]string, 0, len(params))
	for field, value := range params {
		if field != "sign" && value != "" {
			keys = append(keys, field)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, field := range keys {
		parts = append(parts, field+"="+params[field])
	}
	params["sign"] = base64.StdEncoding.EncodeToString(ed25519.Sign(key, []byte(strings.Join(parts, "&")+cfg.ClientSecret)))
	return params, nil
}

func relayForCreateLDCPayment(c *gin.Context, subscription bool) {
	var req relayForLDCPayRequest
	if !relayForBusinessJSON(c, &req) {
		return
	}
	if (subscription && (req.PlanID <= 0 || req.Amount != "")) || (!subscription && req.PlanID != 0) {
		common.ApiErrorMsg(c, "LDC 订单类型或参数无效")
		return
	}
	cfg, err := model.GetRelayForLDCSettings()
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	if !relayForLDCConfigured(cfg) {
		common.ApiErrorMsg(c, "LDC 支付配置未完成")
		return
	}
	order, err := model.CreateRelayForLDCOrder(c.GetInt("id"), req.Amount.String(), req.PlanID, req.ExpectedCredits, cfg)
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	params, err := relayForLDCForm(order, cfg)
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"trade_no": order.TradeNo, "kind": order.Kind, "status": order.Status, "credits": order.Credits, "currency": "LDC", "url": relayForLDCCheckoutURL, "method": "POST", "params": params})
}

func RelayForLDCPay(c *gin.Context)             { relayForCreateLDCPayment(c, false) }
func RelayForLDCSubscriptionPay(c *gin.Context) { relayForCreateLDCPayment(c, true) }

func RelayForLDCOrderStatus(c *gin.Context) {
	order, err := model.GetRelayForLDCOrder(c.GetInt("id"), c.Param("trade_no"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "LDC 订单不存在"})
		return
	}
	common.ApiSuccess(c, order)
}

func RelayForLDCOrders(c *gin.Context) {
	var orders []model.RelayForLDCOrder
	if err := model.DB.Where("user_id = ?", c.GetInt("id")).Order("id DESC").Limit(100).Find(&orders).Error; err != nil {
		relayForBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, orders)
}

type relayForLDCQueryResult struct {
	Code       int    `json:"code"`
	TradeNo    string `json:"trade_no"`
	OutTradeNo string `json:"out_trade_no"`
	Type       string `json:"type"`
	PID        string `json:"pid"`
	Money      string `json:"money"`
	Status     int    `json:"status"`
}

func relayForLDCQuery(ctx context.Context, order *model.RelayForLDCOrder, gatewayTradeNo string) (*relayForLDCQueryResult, error) {
	var credential model.RelayForLDCCredential
	if err := model.DB.First(&credential, order.CredentialID).Error; err != nil {
		return nil, errors.New("LDC 订单凭据不可用")
	}
	values := url.Values{"act": {"order"}, "pid": {credential.ClientID}, "key": {credential.ClientSecret}, "trade_no": {gatewayTradeNo}, "out_trade_no": {order.TradeNo}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, relayForLDCQueryURL+"?"+values.Encode(), nil)
	if err != nil {
		return nil, errors.New("LDC 查单请求无效")
	}
	response, err := relayForLDCHTTPClient.Do(req)
	if err != nil {
		// http errors may contain the URL (and secret); never return/log them.
		return nil, errors.New("LDC 查单暂时不可用")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("LDC 查单未成功")
	}
	var result relayForLDCQueryResult
	if err := json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&result); err != nil {
		return nil, errors.New("LDC 查单响应无效")
	}
	if result.Code != 1 || result.Status != 1 || result.PID != credential.ClientID || result.OutTradeNo != order.TradeNo || result.TradeNo != gatewayTradeNo || (result.Type != "epay" && result.Type != "ldcpay") {
		return nil, errors.New("LDC 查单尚未付款或订单信息不匹配")
	}
	return &result, nil
}

func RelayForLDCNotify(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	if err := c.Request.ParseForm(); err != nil {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	values := c.Request.Form
	for _, key := range []string{"out_trade_no", "trade_no"} {
		if len(values[key]) != 1 || values.Get(key) == "" || len(values.Get(key)) > 128 {
			c.String(http.StatusBadRequest, "fail")
			return
		}
	}
	tradeNo, platformTradeNo := values.Get("out_trade_no"), values.Get("trade_no")
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)
	order, err := model.GetRelayForLDCOrder(0, tradeNo)
	if err != nil {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	// A browser or unauthenticated caller can trigger reconciliation, but only
	// an authenticated HTTPS query to the fixed LDC gateway can authorize it.
	result, err := relayForLDCQuery(c.Request.Context(), order, platformTradeNo)
	if err == nil {
		err = model.CompleteRelayForLDCOrder(result.OutTradeNo, result.TradeNo, result.PID, result.Money)
	}
	if err != nil {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	c.String(http.StatusOK, "success")
}

// Browser redirects are informational. Settlement only occurs after the
// independent server query above, never from a client-side success parameter.
func RelayForLDCReturn(c *gin.Context) {
	c.Redirect(http.StatusFound, paymentReturnPath("/wallet?payment=ldc&pay=pending"))
}
