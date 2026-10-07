package model

import (
	"context"
	"fmt"
	"github.com/dqt081228OHVGFY/further-api/logger"
)

type TopUpSettlementInput struct {
	TradeNo, Provider, PaymentMethod, CustomerID, CustomerEmail string
	Evidence                                                    *PaymentEvidence
}
type TopUpSettlementResult struct {
	Order         TopUp
	CreditedQuota int
	AlreadyDone   bool
}

func SettleTopUpContext(ctx context.Context, input TopUpSettlementInput) (*TopUpSettlementResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result, err := settleTopUpContext(ctx, topUpSettlementRequest{tradeNo: input.TradeNo, provider: input.Provider, paymentMethod: input.PaymentMethod, customerID: input.CustomerID, customerEmail: input.CustomerEmail, evidence: input.Evidence})
	if err != nil {
		return nil, err
	}
	return &TopUpSettlementResult{Order: result.order, CreditedQuota: result.quota, AlreadyDone: result.alreadyDone}, nil
}

func RechargeEpayContext(ctx context.Context, tradeNo, method, ip string) (bool, error) {
	result, err := SettleTopUpContext(ctx, TopUpSettlementInput{TradeNo: tradeNo, Provider: PaymentProviderEpay, PaymentMethod: method})
	if err != nil {
		return false, err
	}
	if !result.AlreadyDone {
		RecordTopupLog(result.Order.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%f", logger.LogQuota(result.CreditedQuota), result.Order.Money), ip, result.Order.PaymentMethod, PaymentProviderEpay)
	}
	return result.AlreadyDone, nil
}

func ManualCompleteTopUpContext(ctx context.Context, tradeNo, ip string) error {
	result, err := SettleTopUpContext(ctx, TopUpSettlementInput{TradeNo: tradeNo})
	if err != nil {
		return err
	}
	if !result.AlreadyDone {
		RecordTopupLog(result.Order.UserId, fmt.Sprintf("管理员补单成功，充值金额: %v，支付金额：%f", logger.FormatQuota(result.CreditedQuota), result.Order.Money), ip, result.Order.PaymentMethod, "admin")
	}
	return nil
}
