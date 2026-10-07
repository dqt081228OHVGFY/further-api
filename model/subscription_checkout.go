package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var (
	ErrSubscriptionOrderNotFound      = errors.New("subscription order not found")
	ErrSubscriptionOrderStatusInvalid = errors.New("subscription order status invalid")
	ErrSubscriptionLedgerConflict     = errors.New("subscription payment ledger conflict")
)

type SubscriptionPurchaseExpectation struct {
	PriceAmount *float64
	Currency    *string
}

func isInternalOrPrepaidSubscriptionPlanTx(tx *gorm.DB, planID int) (bool, error) {
	if tx == nil || planID <= 0 {
		return false, errors.New("套餐编号无效")
	}
	for _, query := range []struct {
		row    any
		filter string
		args   []any
	}{
		{&FurtherSubscriptionPolicy{}, "plan_id = ? AND internal = ?", []any{planID, true}},
		{&FurtherPrepaidCard{}, "plan_id = ?", []any{planID}},
	} {
		if !tx.Migrator().HasTable(query.row) {
			continue
		}
		var count int64
		if err := tx.Unscoped().Model(query.row).Where(query.filter, query.args...).Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

func requireSubscriptionSale(tx *gorm.DB, plan *SubscriptionPlan) error {
	protected, err := isInternalOrPrepaidSubscriptionPlanTx(tx, plan.Id)
	if err != nil {
		return err
	}
	if protected {
		return errors.New("该套餐仅用于内部发放，不能购买")
	}
	if !plan.Enabled {
		return errors.New("套餐未启用")
	}
	return nil
}

func (order *SubscriptionOrder) Insert() error {
	if order == nil || order.UserId <= 0 || order.PlanId <= 0 || strings.TrimSpace(order.TradeNo) == "" || len(order.TradeNo) > 255 || math.IsNaN(order.Money) || math.IsInf(order.Money, 0) || order.Money < 0 {
		return errors.New("订阅订单参数无效")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if order.Status == common.TopUpStatusPending {
			switch order.PaymentProvider {
			case PaymentProviderStripe, PaymentProviderCreem, PaymentProviderEpay, PaymentProviderWaffoPancake:
				var plan SubscriptionPlan
				if err := lockForUpdate(tx).First(&plan, order.PlanId).Error; err != nil {
					return err
				}
				if err := requireSubscriptionSale(tx, &plan); err != nil {
					return err
				}
			}
		}
		if order.CreateTime == 0 {
			order.CreateTime = furtherDBTimestampTx(tx)
		}
		return tx.Create(order).Error
	})
}

func (order *SubscriptionOrder) Update() error {
	if order == nil || order.Id <= 0 {
		return ErrSubscriptionOrderNotFound
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var current SubscriptionOrder
		if err := lockForUpdate(tx).First(&current, order.Id).Error; err != nil {
			return err
		}
		if current.Status == common.TopUpStatusSuccess && order.Status != current.Status {
			return ErrSubscriptionOrderStatusInvalid
		}
		return tx.Model(&current).Updates(map[string]any{"status": order.Status, "payment_method": order.PaymentMethod, "complete_time": order.CompleteTime, "provider_payload": order.ProviderPayload}).Error
	})
}

func GetSubscriptionOrderByTradeNo(tradeNo string) *SubscriptionOrder {
	if strings.TrimSpace(tradeNo) == "" {
		return nil
	}
	var order SubscriptionOrder
	if DB.Where("trade_no = ?", tradeNo).First(&order).Error != nil {
		return nil
	}
	return &order
}

func readSubscriptionOrderLocked(tx *gorm.DB, tradeNo, provider string) (*SubscriptionOrder, error) {
	var order SubscriptionOrder
	if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSubscriptionOrderNotFound
		}
		return nil, err
	}
	if provider != "" && order.PaymentProvider != provider {
		return nil, ErrPaymentMethodMismatch
	}
	return &order, nil
}

func upsertSubscriptionTopUpTx(tx *gorm.DB, order *SubscriptionOrder, previousMethod string) error {
	if tx == nil || order == nil {
		return ErrSubscriptionLedgerConflict
	}
	var entry TopUp
	err := lockForUpdate(tx).Where("trade_no = ?", order.TradeNo).First(&entry).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	now := furtherDBTimestampTx(tx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		entry = TopUp{UserId: order.UserId, Money: order.Money, TradeNo: order.TradeNo, PaymentMethod: order.PaymentMethod, PaymentProvider: order.PaymentProvider, CreateTime: order.CreateTime, CompleteTime: now, Status: common.TopUpStatusSuccess}
		return tx.Create(&entry).Error
	}
	if entry.UserId != order.UserId || entry.Amount != 0 || entry.CreditedQuota != 0 || entry.WalletUnits != "" || !decimal.NewFromFloat(entry.Money).Equal(decimal.NewFromFloat(order.Money)) {
		return ErrSubscriptionLedgerConflict
	}
	if (entry.PaymentProvider != "" && entry.PaymentProvider != order.PaymentProvider) || (entry.PaymentMethod != "" && entry.PaymentMethod != order.PaymentMethod && entry.PaymentMethod != previousMethod) {
		return ErrPaymentMethodMismatch
	}
	if entry.Status != common.TopUpStatusPending && entry.Status != common.TopUpStatusSuccess {
		return ErrSubscriptionLedgerConflict
	}
	created := entry.CreateTime
	if created == 0 {
		created = order.CreateTime
	}
	return tx.Model(&entry).Updates(map[string]any{"money": order.Money, "payment_method": order.PaymentMethod, "payment_provider": order.PaymentProvider, "create_time": created, "complete_time": now, "status": common.TopUpStatusSuccess}).Error
}

func CompleteSubscriptionOrderContext(ctx context.Context, tradeNo, payload, expectedProvider, actualMethod string) error {
	return CompleteSubscriptionOrderWithEvidenceContext(ctx, tradeNo, payload, expectedProvider, actualMethod, nil)
}

func CompleteSubscriptionOrderWithEvidenceContext(ctx context.Context, tradeNo, payload, expectedProvider, actualMethod string, evidence *PaymentEvidence) error {
	if strings.TrimSpace(tradeNo) == "" {
		return ErrSubscriptionOrderNotFound
	}
	var receipt *SubscriptionOrder
	var title, upgraded string
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		order, err := readSubscriptionOrderLocked(tx, tradeNo, expectedProvider)
		if err != nil {
			return err
		}
		if order.Status == common.TopUpStatusSuccess {
			return nil
		}
		if order.Status != common.TopUpStatusPending {
			return ErrSubscriptionOrderStatusInvalid
		}
		if err := validatePaymentEvidence(order.Money, "", false, evidence); err != nil {
			return err
		}
		plan, err := getSubscriptionPlanByIdTx(tx, order.PlanId)
		if err != nil {
			return err
		}
		previousMethod := order.PaymentMethod
		if actualMethod != "" {
			order.PaymentMethod = actualMethod
		}
		if err := upsertSubscriptionTopUpTx(tx, order, previousMethod); err != nil {
			return err
		}
		entitlement, err := CreateUserSubscriptionFromPlanTx(tx, order.UserId, plan, "order")
		if err != nil {
			return err
		}
		changes := map[string]any{"status": common.TopUpStatusSuccess, "complete_time": furtherDBTimestampTx(tx), "payment_method": order.PaymentMethod}
		if payload != "" {
			changes["provider_payload"] = payload
		}
		if err := tx.Model(order).Updates(changes).Error; err != nil {
			return err
		}
		receipt, title = order, plan.Title
		if entitlement.PrevUserGroup != "" {
			upgraded = entitlement.UpgradeGroup
		}
		return nil
	})
	if err != nil || receipt == nil {
		return err
	}
	if upgraded != "" {
		refreshSubscriptionUserGroupCache(receipt.UserId, "payment settlement")
	}
	RecordLog(receipt.UserId, LogTypeTopup, fmt.Sprintf("订阅购买成功，套餐: %s，支付金额: %.2f，支付方式: %s", title, receipt.Money, receipt.PaymentMethod))
	return nil
}

func ExpireSubscriptionOrder(tradeNo, expectedProvider string) error {
	return TransitionSubscriptionOrderContext(context.Background(), tradeNo, expectedProvider, common.TopUpStatusExpired)
}

func TransitionSubscriptionOrderContext(ctx context.Context, tradeNo, expectedProvider, status string) error {
	if strings.TrimSpace(tradeNo) == "" {
		return ErrSubscriptionOrderNotFound
	}
	if status != common.TopUpStatusExpired && status != common.TopUpStatusFailed {
		return ErrSubscriptionOrderStatusInvalid
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		order, err := readSubscriptionOrderLocked(tx, tradeNo, expectedProvider)
		if err != nil || order.Status != common.TopUpStatusPending {
			return err
		}
		return tx.Model(order).Updates(map[string]any{"status": status, "complete_time": furtherDBTimestampTx(tx)}).Error
	})
}

func calcSubscriptionBalanceQuota(amount float64) (int, error) {
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 {
		return 0, errors.New("套餐价格无效")
	}
	if amount == 0 {
		return 0, nil
	}
	if common.QuotaPerUnit <= 0 {
		return 0, errors.New("额度单位配置错误")
	}
	return common.WalletQuotaFromDecimalStrict(decimal.NewFromFloat(amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Ceil())
}

func validateSubscriptionQuote(plan *SubscriptionPlan, quote []SubscriptionPurchaseExpectation) error {
	if len(quote) > 1 {
		return errors.New("套餐报价参数无效")
	}
	if len(quote) == 0 {
		return nil
	}
	if price := quote[0].PriceAmount; price != nil {
		if math.IsNaN(*price) || math.IsInf(*price, 0) || !decimal.NewFromFloat(*price).Equal(decimal.NewFromFloat(plan.PriceAmount)) {
			return errors.New("套餐价格已更新，请刷新后重新确认")
		}
	}
	currency := strings.TrimSpace(plan.Currency)
	if currency == "" {
		currency = "USD"
	}
	if quote[0].Currency != nil && !strings.EqualFold(strings.TrimSpace(*quote[0].Currency), currency) {
		return errors.New("套餐币种已更新，请刷新后重新确认")
	}
	return nil
}

func PurchaseSubscriptionWithBalance(userID, planID int, quote ...SubscriptionPurchaseExpectation) error {
	if userID <= 0 || planID <= 0 {
		return errors.New("请选择有效的用户与套餐")
	}
	var plan SubscriptionPlan
	var charged int
	var groupChanged bool
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).First(&user, userID).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).First(&plan, planID).Error; err != nil {
			return err
		}
		plan.NormalizeDefaults()
		if err := requireSubscriptionSale(tx, &plan); err != nil {
			return err
		}
		if !*plan.AllowBalancePay {
			return errors.New("该套餐不允许使用余额兑换")
		}
		if err := validateSubscriptionQuote(&plan, quote); err != nil {
			return err
		}
		var err error
		charged, err = calcSubscriptionBalanceQuota(plan.PriceAmount)
		if err != nil {
			return err
		}
		if charged > 0 {
			result := tx.Model(&User{}).Where("id = ? AND quota >= ?", userID, charged).Update("quota", gorm.Expr("quota - ?", charged))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errors.New("余额不足")
			}
		}
		entitlement, err := CreateUserSubscriptionFromPlanTx(tx, userID, &plan, PaymentMethodBalance)
		if err != nil {
			return err
		}
		groupChanged = entitlement.PrevUserGroup != ""
		now := furtherDBTimestampTx(tx)
		order := SubscriptionOrder{UserId: userID, PlanId: planID, Money: plan.PriceAmount, TradeNo: "further-subscription-" + common.GetRandomString(24), PaymentMethod: PaymentMethodBalance, PaymentProvider: PaymentProviderBalance, Status: common.TopUpStatusSuccess, CreateTime: now, CompleteTime: now, ProviderPayload: fmt.Sprintf("charged_quota=%d", charged)}
		return tx.Create(&order).Error
	})
	if err != nil {
		return err
	}
	if charged > 0 {
		if err := cacheDecrUserQuota(userID, int64(charged)); err != nil {
			common.SysError("failed to refresh subscription purchase allowance: " + err.Error())
		}
	}
	if groupChanged {
		refreshSubscriptionUserGroupCache(userID, "wallet checkout")
	}
	RecordLog(userID, LogTypeTopup, fmt.Sprintf("使用余额购买订阅成功，套餐: %s，支付金额: %s，扣除额度: %d", plan.Title, decimal.NewFromFloat(plan.PriceAmount).String(), charged))
	return nil
}

func CompleteSubscriptionOrder(tradeNo, payload, provider, method string) error {
	return CompleteSubscriptionOrderContext(context.Background(), tradeNo, payload, provider, method)
}
