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

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const furtherLDCCheckoutURL = "https://credit.linux.do/epay/pay/submit.php"
const furtherLDCQueryURL = "https://credit.linux.do/epay/api.php"

var furtherLDCHTTPClient = &http.Client{
	Timeout: 12 * time.Second,
	// Never forward a merchant secret to a redirected host.
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
}

func furtherLDCCallbackURLs(cfg model.FurtherLDCSettings) (string, string, error) {
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

func furtherLDCConfigured(cfg model.FurtherLDCSettings) bool {
	_, _, err := furtherLDCCallbackURLs(cfg)
	_, keyErr := model.ParseFurtherLDCPrivateKey(cfg.PrivateKey)
	return cfg.ClientID != "" && cfg.ClientSecret != "" && err == nil && keyErr == nil
}

func furtherLDCConfigView(cfg model.FurtherLDCSettings) gin.H {
	notify, back, _ := furtherLDCCallbackURLs(cfg)
	public, publicPEM := model.FurtherLDCPublicKeys(cfg.PrivateKey)
	return gin.H{"enabled": cfg.Enabled, "configured": furtherLDCConfigured(cfg), "client_id": cfg.ClientID, "secret_configured": cfg.ClientSecret != "", "private_key_configured": public != "", "public_key": public, "public_key_pem": publicPEM, "credits_per_unit": cfg.CreditsPerUnit, "min_topup": cfg.MinTopUp, "callback_base_url": cfg.CallbackBaseURL, "notify_url": notify, "return_url": back, "currency": "LDC", "wallet_unit": "USD"}
}

func FurtherGetLDCConfig(c *gin.Context) {
	cfg, err := model.GetFurtherLDCSettings()
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, furtherLDCConfigView(cfg))
}

func FurtherSaveLDCConfig(c *gin.Context) {
	var body struct {
		Enabled         bool   `json:"enabled"`
		ClientID        string `json:"client_id"`
		ClientSecret    string `json:"client_secret"`
		PrivateKey      string `json:"private_key"`
		CreditsPerUnit  string `json:"credits_per_unit"`
		MinTopUp        string `json:"min_topup"`
		CallbackBaseURL string `json:"callback_base_url"`
	}
	if !furtherBusinessJSON(c, &body) {
		return
	}
	cfg := model.FurtherLDCSettings{Enabled: body.Enabled, ClientID: body.ClientID, ClientSecret: body.ClientSecret, PrivateKey: body.PrivateKey, CreditsPerUnit: body.CreditsPerUnit, MinTopUp: body.MinTopUp, CallbackBaseURL: body.CallbackBaseURL}
	if cfg.Enabled {
		if _, _, err := furtherLDCCallbackURLs(cfg); err != nil {
			furtherBusinessError(c, err)
			return
		}
	}
	cfg, err := model.SaveFurtherLDCSettings(cfg)
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, 0, "further.ldc.configure", map[string]any{"enabled": cfg.Enabled, "credits_per_unit": cfg.CreditsPerUnit, "min_topup": cfg.MinTopUp})
	common.ApiSuccess(c, furtherLDCConfigView(cfg))
}

func FurtherLDCInfo(c *gin.Context) {
	cfg, err := model.GetFurtherLDCSettings()
	if err != nil {
		furtherBusinessError(c, err)
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
	configured := furtherLDCConfigured(cfg)
	common.ApiSuccess(c, gin.H{"enabled": cfg.Enabled && configured, "configured": configured, "currency": "LDC", "wallet_unit": "USD", "payment_method": "ldc", "credits_per_unit": cfg.CreditsPerUnit, "min_topup": cfg.MinTopUp, "max_topup": maximum, "amount_options": []string{"1", "10", "50", "100"}})
}

type furtherLDCPayRequest struct {
	Amount          json.Number `json:"amount"`
	PlanID          int         `json:"plan_id"`
	ExpectedCredits string      `json:"expected_credits"`
}

func FurtherLDCQuote(c *gin.Context) {
	var req furtherLDCPayRequest
	if !furtherBusinessJSON(c, &req) {
		return
	}
	cfg, err := model.GetFurtherLDCSettings()
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	if !furtherLDCConfigured(cfg) || req.PlanID < 0 {
		common.ApiErrorMsg(c, "LDC 支付配置未完成或参数无效")
		return
	}
	quote, err := model.QuoteFurtherLDC(c.GetInt("id"), req.Amount.String(), req.PlanID, cfg)
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, quote)
}

// The native LDC protocol signs sorted non-empty form fields plus the merchant
// secret using Ed25519. No legacy payment provider participates in checkout.
func furtherLDCForm(order *model.FurtherLDCOrder, cfg model.FurtherLDCSettings) (map[string]string, error) {
	notify, back, err := furtherLDCCallbackURLs(cfg)
	if err != nil {
		return nil, err
	}
	key, err := model.ParseFurtherLDCPrivateKey(cfg.PrivateKey)
	if err != nil {
		return nil, err
	}
	name := "further-api 钱包充值"
	if order.Kind == "subscription" {
		name = "further-api 订阅 · " + order.PlanTitle
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

func furtherCreateLDCPayment(c *gin.Context, subscription bool) {
	var req furtherLDCPayRequest
	if !furtherBusinessJSON(c, &req) {
		return
	}
	if (subscription && (req.PlanID <= 0 || req.Amount != "")) || (!subscription && req.PlanID != 0) {
		common.ApiErrorMsg(c, "LDC 订单类型或参数无效")
		return
	}
	cfg, err := model.GetFurtherLDCSettings()
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	if !furtherLDCConfigured(cfg) {
		common.ApiErrorMsg(c, "LDC 支付配置未完成")
		return
	}
	order, err := model.CreateFurtherLDCOrder(c.GetInt("id"), req.Amount.String(), req.PlanID, req.ExpectedCredits, cfg)
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	params, err := furtherLDCForm(order, cfg)
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"trade_no": order.TradeNo, "kind": order.Kind, "status": order.Status, "credits": order.Credits, "currency": "LDC", "url": furtherLDCCheckoutURL, "method": "POST", "params": params})
}

func FurtherLDCPay(c *gin.Context)             { furtherCreateLDCPayment(c, false) }
func FurtherLDCSubscriptionPay(c *gin.Context) { furtherCreateLDCPayment(c, true) }

func FurtherLDCOrderStatus(c *gin.Context) {
	order, err := model.GetFurtherLDCOrder(c.GetInt("id"), c.Param("trade_no"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "LDC 订单不存在"})
		return
	}
	common.ApiSuccess(c, order)
}

func FurtherLDCOrders(c *gin.Context) {
	var orders []model.FurtherLDCOrder
	if err := model.DB.Where("user_id = ?", c.GetInt("id")).Order("id DESC").Limit(100).Find(&orders).Error; err != nil {
		furtherBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, orders)
}

type furtherLDCQueryResult struct {
	Code       int    `json:"code"`
	TradeNo    string `json:"trade_no"`
	OutTradeNo string `json:"out_trade_no"`
	Type       string `json:"type"`
	PID        string `json:"pid"`
	Money      string `json:"money"`
	Status     int    `json:"status"`
}

func furtherLDCQuery(ctx context.Context, order *model.FurtherLDCOrder, gatewayTradeNo string) (*furtherLDCQueryResult, error) {
	var credential model.FurtherLDCCredential
	if err := model.DB.First(&credential, order.CredentialID).Error; err != nil {
		return nil, errors.New("LDC 订单凭据不可用")
	}
	values := url.Values{"act": {"order"}, "pid": {credential.ClientID}, "key": {credential.ClientSecret}, "trade_no": {gatewayTradeNo}, "out_trade_no": {order.TradeNo}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, furtherLDCQueryURL+"?"+values.Encode(), nil)
	if err != nil {
		return nil, errors.New("LDC 查单请求无效")
	}
	response, err := furtherLDCHTTPClient.Do(req)
	if err != nil {
		// http errors may contain the URL (and secret); never return/log them.
		return nil, errors.New("LDC 查单暂时不可用")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("LDC 查单未成功")
	}
	var result furtherLDCQueryResult
	if err := json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&result); err != nil {
		return nil, errors.New("LDC 查单响应无效")
	}
	if result.Code != 1 || result.Status != 1 || result.PID != credential.ClientID || result.OutTradeNo != order.TradeNo || result.TradeNo != gatewayTradeNo || (result.Type != "epay" && result.Type != "ldcpay") {
		return nil, errors.New("LDC 查单尚未付款或订单信息不匹配")
	}
	return &result, nil
}

func FurtherLDCNotify(c *gin.Context) {
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
	order, err := model.GetFurtherLDCOrder(0, tradeNo)
	if err != nil {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	// A browser or unauthenticated caller can trigger reconciliation, but only
	// an authenticated HTTPS query to the fixed LDC gateway can authorize it.
	result, err := furtherLDCQuery(c.Request.Context(), order, platformTradeNo)
	if err == nil {
		err = model.CompleteFurtherLDCOrder(result.OutTradeNo, result.TradeNo, result.PID, result.Money)
	}
	if err != nil {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	c.String(http.StatusOK, "success")
}

// Browser redirects are informational. Settlement only occurs after the
// independent server query above, never from a client-side success parameter.
func FurtherLDCReturn(c *gin.Context) {
	c.Redirect(http.StatusFound, paymentReturnPath("/wallet?payment=ldc&pay=pending"))
}
