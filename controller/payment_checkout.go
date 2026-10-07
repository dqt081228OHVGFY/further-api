package controller

import (
	"encoding/json"
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"strings"
)

type GatewayCheckoutRequest struct {
	Provider        string          `json:"provider"`
	Amount          json.RawMessage `json:"amount"`
	Method          string          `json:"method"`
	LegacyMethod    string          `json:"payment_method,omitempty"`
	ProductID       string          `json:"product_id,omitempty"`
	LegacyProductID string          `json:"productId,omitempty"`
	ReturnURL       string          `json:"return_url,omitempty"`
}

func createPaymentCheckout(c *gin.Context, provider string) {
	var input GatewayCheckoutRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		common.ApiErrorMsg(c, "支付参数无效")
		return
	}
	if provider == "" {
		provider = strings.TrimSpace(input.Provider)
	}
	amount := strings.TrimSpace(string(input.Amount))
	if strings.HasPrefix(amount, "\"") {
		var text string
		if json.Unmarshal(input.Amount, &text) != nil {
			common.ApiErrorMsg(c, "充值金额无效")
			return
		}
		amount = strings.TrimSpace(text)
	}
	if input.ProductID == "" {
		input.ProductID = input.LegacyProductID
	}
	if input.Method == "" {
		input.Method = input.LegacyMethod
	}
	result, err := service.CreateGatewayCheckoutContext(c.Request.Context(), c.GetInt("id"), provider, service.GatewayCheckoutInput{Amount: amount, Method: input.Method, ProductID: input.ProductID, ReturnURL: input.ReturnURL})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func CreatePaymentCheckout(c *gin.Context) { createPaymentCheckout(c, "") }
func RequestEpay(c *gin.Context)           { createPaymentCheckout(c, model.PaymentProviderEpay) }
func RequestStripe(c *gin.Context)         { createPaymentCheckout(c, model.PaymentProviderStripe) }
func RequestCreem(c *gin.Context)          { createPaymentCheckout(c, model.PaymentProviderCreem) }
func RequestWaffo(c *gin.Context)          { createPaymentCheckout(c, model.PaymentProviderWaffo) }
func RequestWaffoPancake(c *gin.Context)   { createPaymentCheckout(c, model.PaymentProviderWaffoPancake) }
