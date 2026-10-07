package controller

import (
	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"sync"
)

func GetTopUpInfo(c *gin.Context) {
	cfg, err := model.GetFurtherLDCSettings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	methods := make([]map[string]string, 0)
	configured := make(map[string]bool)
	for _, entry := range service.GatewayCheckoutMethods() {
		configured[entry.Provider] = true
		methods = append(methods, map[string]string{"name": entry.Name, "type": entry.Method, "provider": entry.Provider, "currency": entry.Currency, "min_topup": strconv.Itoa(entry.MinAmount)})
	}
	ldc := cfg.Enabled && furtherLDCConfigured(cfg)
	if ldc {
		methods = append(methods, map[string]string{"name": "LINUX DO Credit", "type": "ldc", "provider": "ldc", "currency": "LDC", "min_topup": cfg.MinTopUp})
	}
	common.ApiSuccess(c, gin.H{"enable_ldc_topup": ldc, "enable_online_topup": configured[model.PaymentProviderEpay], "enable_stripe_topup": configured[model.PaymentProviderStripe], "enable_creem_topup": configured[model.PaymentProviderCreem], "enable_waffo_topup": configured[model.PaymentProviderWaffo], "enable_waffo_pancake_topup": configured[model.PaymentProviderWaffoPancake], "enable_redemption": operation_setting.IsPaymentComplianceConfirmed(), "pay_methods": methods, "creem_products": service.GatewayCheckoutProducts(), "currency": "LDC", "wallet_unit": "USD", "credits_per_unit": cfg.CreditsPerUnit, "min_topup": cfg.MinTopUp, "amount_options": []int{1, 10, 50, 100}, "discount": map[int]float64{}, "topup_link": ""})
}

func GetEpayClient() *epay.Client {
	if operation_setting.PayAddress == "" || operation_setting.EpayId == "" || operation_setting.EpayKey == "" {
		return nil
	}
	withUrl, err := epay.NewClient(&epay.Config{
		PartnerID: operation_setting.EpayId,
		Key:       operation_setting.EpayKey,
	}, operation_setting.PayAddress)
	if err != nil {
		return nil
	}
	return withUrl
}

// Per-order locks coalesce callbacks in one process. The settlement transaction
// owns the durable idempotency decision across gateway instances.
var paymentCallbackLocks = struct {
	sync.Mutex
	rows map[string]*refCountedMutex
}{rows: make(map[string]*refCountedMutex)}

type refCountedMutex struct {
	mu       sync.Mutex
	refCount int
}

func LockOrder(reference string) {
	paymentCallbackLocks.Lock()
	row := paymentCallbackLocks.rows[reference]
	if row == nil {
		row = &refCountedMutex{}
		paymentCallbackLocks.rows[reference] = row
	}
	row.refCount++
	paymentCallbackLocks.Unlock()
	row.mu.Lock()
}
func UnlockOrder(reference string) {
	paymentCallbackLocks.Lock()
	row := paymentCallbackLocks.rows[reference]
	paymentCallbackLocks.Unlock()
	if row == nil {
		return
	}
	row.mu.Unlock()
	paymentCallbackLocks.Lock()
	row.refCount--
	if row.refCount == 0 {
		delete(paymentCallbackLocks.rows, reference)
	}
	paymentCallbackLocks.Unlock()
}

func paymentCallbackParameters(c *gin.Context) (map[string]string, error) {
	values := c.Request.URL.Query()
	if c.Request.Method == http.MethodPost {
		if err := c.Request.ParseForm(); err != nil {
			return nil, err
		}
		values = c.Request.PostForm
	}
	result := make(map[string]string, len(values))
	for key := range values {
		result[key] = values.Get(key)
	}
	return result, nil
}

func EpayNotify(c *gin.Context) {
	response := "fail"
	defer func() { c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(response)) }()
	if !isEpayWebhookEnabled() {
		return
	}
	params, err := paymentCallbackParameters(c)
	if err != nil || len(params) == 0 {
		return
	}
	client := GetEpayClient()
	if client == nil {
		return
	}
	verified, err := client.Verify(params)
	if err != nil || !verified.VerifyStatus {
		return
	}
	if verified.TradeStatus == epay.StatusTradeSuccess {
		LockOrder(verified.ServiceTradeNo)
		defer UnlockOrder(verified.ServiceTradeNo)
		if err := model.CompleteGatewayPaymentContext(c.Request.Context(), model.GatewayPaymentConfirmation{TradeNo: verified.ServiceTradeNo, Provider: model.PaymentProviderEpay, Method: verified.Type, CallerIP: c.ClientIP(), Evidence: &model.PaymentEvidence{Amount: params["money"], Currency: "CNY"}}); err != nil {
			return
		}
	}
	response = "success"
}

func topUpDirectory(c *gin.Context, admin bool) {
	page := common.GetPageQuery(c)
	rows, total, err := model.ListTopUpDirectoryContext(c.Request.Context(), model.TopUpDirectoryInput{UserID: c.GetInt("id"), Admin: admin, Keyword: c.Query("keyword"), Offset: page.GetStartIdx(), Limit: page.GetPageSize()})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	page.SetTotal(int(total))
	page.SetItems(rows)
	common.ApiSuccess(c, page)
}
func GetUserTopUps(c *gin.Context) { topUpDirectory(c, false) }
func GetAllTopUps(c *gin.Context)  { topUpDirectory(c, true) }

type AdminCompleteTopupRequest struct {
	TradeNo string `json:"trade_no"`
}

func AdminCompleteTopUp(c *gin.Context) {
	var input AdminCompleteTopupRequest
	if err := c.ShouldBindJSON(&input); err != nil || input.TradeNo == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	LockOrder(input.TradeNo)
	defer UnlockOrder(input.TradeNo)
	if err := model.ManualCompleteTopUpContext(c.Request.Context(), input.TradeNo, c.ClientIP()); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
