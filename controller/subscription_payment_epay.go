package controller

import (
	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/gin-gonic/gin"
	"net/http"
)

func SubscriptionEpayNotify(c *gin.Context) {
	response := "fail"
	defer func() { c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(response)) }()
	params, err := paymentCallbackParameters(c)
	if err != nil || len(params) == 0 {
		return
	}
	client := GetEpayClient()
	if client == nil {
		return
	}
	verified, err := client.Verify(params)
	if err != nil || !verified.VerifyStatus || verified.TradeStatus != epay.StatusTradeSuccess {
		return
	}
	LockOrder(verified.ServiceTradeNo)
	defer UnlockOrder(verified.ServiceTradeNo)
	if err := model.CompleteSubscriptionOrderWithEvidenceContext(c.Request.Context(), verified.ServiceTradeNo, common.GetJsonString(gin.H{"trade_no": verified.ServiceTradeNo, "amount": params["money"], "method": verified.Type}), model.PaymentProviderEpay, verified.Type, &model.PaymentEvidence{Amount: params["money"], Currency: "CNY"}); err != nil {
		return
	}
	response = "success"
}

func SubscriptionEpayReturn(c *gin.Context) {
	outcome := "fail"
	defer func() { c.Redirect(http.StatusFound, paymentReturnPath("/wallet?pay="+outcome)) }()
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
	if verified.TradeStatus != epay.StatusTradeSuccess {
		outcome = "pending"
		return
	}
	LockOrder(verified.ServiceTradeNo)
	defer UnlockOrder(verified.ServiceTradeNo)
	if err := model.CompleteSubscriptionOrderWithEvidenceContext(c.Request.Context(), verified.ServiceTradeNo, common.GetJsonString(gin.H{"trade_no": verified.ServiceTradeNo, "amount": params["money"], "method": verified.Type}), model.PaymentProviderEpay, verified.Type, &model.PaymentEvidence{Amount: params["money"], Currency: "CNY"}); err != nil {
		return
	}
	outcome = "success"
}
