package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting"
	"github.com/gin-gonic/gin"
	waffo "github.com/waffo-com/waffo-go"
	"github.com/waffo-com/waffo-go/config"
	"github.com/waffo-com/waffo-go/core"
)

func getWaffoSDK() (*waffo.Waffo, error) {
	environment := config.Production
	apiKey, privateKey, certificate := setting.WaffoApiKey, setting.WaffoPrivateKey, setting.WaffoPublicCert
	if setting.WaffoSandbox {
		environment = config.Sandbox
		apiKey, privateKey, certificate = setting.WaffoSandboxApiKey, setting.WaffoSandboxPrivateKey, setting.WaffoSandboxPublicCert
	}
	builder := config.NewConfigBuilder().APIKey(apiKey).PrivateKey(privateKey).WaffoPublicKey(certificate).Environment(environment)
	if merchant := setting.WaffoMerchantId; merchant != "" {
		builder = builder.MerchantID(merchant)
	}
	configuration, err := builder.Build()
	if err != nil {
		return nil, err
	}
	return waffo.New(configuration), nil
}

func processWaffoPayment(ctx context.Context, result *core.PaymentNotificationResult, callerIP string) error {
	if result == nil {
		return errors.New("支付事件缺少订单信息")
	}
	// Pending authorization and capture notifications leave the order payable.
	switch result.OrderStatus {
	case core.OrderStatusOrderClose:
		if result.MerchantOrderID == "" {
			return nil
		}
		err := model.TransitionGatewayPaymentContext(ctx, result.MerchantOrderID, model.PaymentProviderWaffo, common.TopUpStatusFailed)
		if paymentTerminalEventHandled(err) {
			return nil
		}
		return err
	case core.OrderStatusPaySuccess:
	default:
		return nil
	}
	if result.MerchantOrderID == "" {
		return model.ErrTopUpNotFound
	}
	LockOrder(result.MerchantOrderID)
	defer UnlockOrder(result.MerchantOrderID)
	receipt, _ := json.Marshal(map[string]string{"request_id": result.PaymentRequestID, "order_status": result.OrderStatus, "currency": result.OrderCurrency, "amount": result.OrderAmount})
	return model.CompleteGatewayPaymentContext(ctx, model.GatewayPaymentConfirmation{TradeNo: result.MerchantOrderID, Provider: model.PaymentProviderWaffo, CallerIP: callerIP, Receipt: string(receipt), Evidence: &model.PaymentEvidence{Amount: result.OrderAmount, Currency: result.OrderCurrency}})
}

func WaffoWebhook(c *gin.Context) {
	if !isWaffoWebhookEnabled() {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	payload, err := readPaymentWebhook(c)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	sdk, err := getWaffoSDK()
	if err != nil {
		logger.LogError(c.Request.Context(), "Waffo 回调验证器初始化失败")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	verifier := sdk.Webhook()
	if !verifier.VerifySignature(string(payload), c.GetHeader("X-SIGNATURE")) {
		logger.LogWarn(c.Request.Context(), "Waffo 回调签名验证未通过")
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	var event core.PaymentNotification
	if err := json.Unmarshal(payload, &event); err != nil {
		sendWaffoWebhookResponse(c, verifier, false, "invalid payload")
		return
	}
	if event.EventType != core.EventPayment {
		sendWaffoWebhookResponse(c, verifier, true, "")
		return
	}
	handleWaffoPayment(c, verifier, event.Result)
}

func handleWaffoPayment(c *gin.Context, verifier *core.WebhookHandler, result *core.PaymentNotificationResult) {
	if err := processWaffoPayment(c.Request.Context(), result, c.ClientIP()); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo 回调结算失败 error=%q", err.Error()))
		sendWaffoWebhookResponse(c, verifier, false, "settlement unavailable")
		return
	}
	sendWaffoWebhookResponse(c, verifier, true, "")
}

func sendWaffoWebhookResponse(c *gin.Context, verifier *core.WebhookHandler, success bool, message string) {
	var response, signature string
	if success {
		response, signature = verifier.BuildSuccessResponse()
	} else {
		response, signature = verifier.BuildFailedResponse(message)
	}
	c.Header("X-SIGNATURE", signature)
	c.Data(http.StatusOK, "application/json", []byte(response))
}
