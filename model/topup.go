package model

import (
	"errors"
	"fmt"
	"maps"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/logger"

	"gorm.io/gorm"
)

type TopUp struct {
	Id               int     `json:"id"`
	UserId           int     `json:"user_id" gorm:"index"`
	Amount           int64   `json:"amount"`
	WalletUnits      string  `json:"wallet_units,omitempty" gorm:"size:64"`
	CreditedQuota    int     `json:"credited_quota,omitempty"`
	Money            float64 `json:"money"`
	TradeNo          string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod    string  `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider  string  `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	PaymentCurrency  string  `json:"payment_currency,omitempty" gorm:"type:varchar(8);default:''"`
	AcceptsDiscounts bool    `json:"-" gorm:"not null;default:false"`
	CreateTime       int64   `json:"create_time"`
	CompleteTime     int64   `json:"complete_time"`
	Status           string  `json:"status"`
}

const (
	PaymentMethodStripe       = "stripe"
	PaymentMethodCreem        = "creem"
	PaymentMethodWaffo        = "waffo"
	PaymentMethodWaffoPancake = "waffo_pancake"
	PaymentMethodBalance      = "balance"
)

const (
	PaymentProviderEpay         = "epay"
	PaymentProviderStripe       = "stripe"
	PaymentProviderCreem        = "creem"
	PaymentProviderWaffo        = "waffo"
	PaymentProviderWaffoPancake = "waffo_pancake"
	PaymentProviderBalance      = "balance"
)

var (
	ErrPaymentMethodMismatch    = errors.New("payment method mismatch")
	ErrTopUpNotFound            = errors.New("topup not found")
	ErrTopUpStatusInvalid       = errors.New("topup status invalid")
	ErrInvalidTopUpQuota        = errors.New("invalid top-up quota")
	ErrTopUpQuotaLimitExceeded  = errors.New("top-up quota limit exceeded")
	ErrWalletQuotaLimitExceeded = errors.New("wallet quota limit exceeded")
)

func (topUp *TopUp) Insert() error {
	if topUp == nil {
		return ErrTopUpNotFound
	}
	if topUp.Status == common.TopUpStatusPending {
		switch topUp.PaymentProvider {
		case PaymentProviderEpay, PaymentProviderStripe, PaymentProviderCreem, PaymentProviderWaffo, PaymentProviderWaffoPancake:
			quota, units, err := topUpCreditSnapshot(topUp)
			if err != nil {
				return err
			}
			topUp.CreditedQuota, topUp.WalletUnits = quota, units
		}
	}
	return DB.Create(topUp).Error
}

func topUpQuotaMaxCurrent(creditedQuota int) (int, error) {
	if creditedQuota <= 0 || creditedQuota > common.MaxWalletQuota {
		return 0, ErrInvalidTopUpQuota
	}
	return common.MaxWalletQuota - creditedQuota, nil
}

// ValidateTopUpQuotaCapacity performs the user-facing pre-payment check. The
// settlement path repeats the same invariant with an atomic conditional
// update, because the wallet balance can change after checkout creation.
func ValidateTopUpQuotaCapacity(userId int, creditedQuota int) error {
	maxCurrentQuota, err := topUpQuotaMaxCurrent(creditedQuota)
	if err != nil {
		return err
	}

	var user User
	if err := DB.Select("quota").Where("id = ?", userId).First(&user).Error; err != nil {
		return err
	}
	if user.Quota > maxCurrentQuota {
		return ErrTopUpQuotaLimitExceeded
	}
	return nil
}

// creditTopUpQuota atomically enforces the wallet ceiling while adding quota.
// Keeping the predicate and increment in one UPDATE prevents two
// concurrent callbacks from both passing a separate read/check.
func creditTopUpQuota(tx *gorm.DB, userId int, creditedQuota int, updates map[string]any) error {
	maxCurrentQuota, err := topUpQuotaMaxCurrent(creditedQuota)
	if err != nil {
		return err
	}

	updateFields := make(map[string]any, len(updates)+1)
	maps.Copy(updateFields, updates)
	updateFields["quota"] = gorm.Expr("quota + ?", creditedQuota)

	result := tx.Model(&User{}).
		Where("id = ? AND quota <= ?", userId, maxCurrentQuota).
		Updates(updateFields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}

	var count int64
	if err := tx.Model(&User{}).Where("id = ?", userId).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return ErrTopUpQuotaLimitExceeded
}

// Update preserves the status-change entry point while success requires settlement.
func (topUp *TopUp) Update() error {
	if topUp == nil {
		return ErrTopUpNotFound
	}
	return transitionPendingTopUp(topUp.TradeNo, topUp.PaymentProvider, topUp.Status)
}

func GetTopUpById(id int) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("id = ?", id).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func GetTopUpByTradeNo(tradeNo string) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("trade_no = ?", tradeNo).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func UpdatePendingTopUpStatus(tradeNo string, expectedPaymentProvider string, targetStatus string) error {
	return transitionPendingTopUp(tradeNo, expectedPaymentProvider, targetStatus)
}

// RechargeEpay 原子完成易支付订单：订单行锁、状态校验、成功更新与用户额度增加
// 在同一个事务内完成，因此同一订单的并发/重复回调（包括多实例部署下）最多充值一次。
// alreadyDone=true 表示订单此前已完成，本次为幂等重复回调。
// 进程内的 LockOrder 只是优化，正确性由本函数的数据库行锁保证。
func RechargeEpay(tradeNo string, actualPaymentMethod string, callerIp string) (bool, error) {
	result, err := settleTopUp(topUpSettlementRequest{tradeNo: tradeNo, provider: PaymentProviderEpay, paymentMethod: actualPaymentMethod})
	if err != nil {
		return false, err
	}
	if result.alreadyDone {
		return true, nil
	}
	order := result.order
	RecordTopupLog(order.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%f", logger.LogQuota(result.quota), order.Money), callerIp, order.PaymentMethod, PaymentProviderEpay)
	return false, nil
}

func Recharge(referenceId string, customerId string, callerIp string) error {
	result, err := settleTopUp(topUpSettlementRequest{tradeNo: referenceId, provider: PaymentProviderStripe, customerID: customerId})
	if err != nil {
		return fmt.Errorf("充值失败，请稍后重试: %w", err)
	}
	if !result.alreadyDone {
		order := result.order
		RecordTopupLog(order.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%d", logger.FormatQuota(result.quota), order.Amount), callerIp, order.PaymentMethod, PaymentMethodStripe)
	}
	return nil
}

// topUpQueryWindowSeconds 限制充值记录查询的时间窗口（秒）。
const topUpQueryWindowSeconds int64 = 30 * 24 * 60 * 60

// topUpQueryCutoff 返回允许查询的最早 create_time（秒级 Unix 时间戳）。
func topUpQueryCutoff() int64 {
	return common.GetTimestamp() - topUpQueryWindowSeconds
}

// GetAllTopUps 获取全平台的充值记录（管理员使用，不限制时间窗口）

// searchTopUpCountHardLimit 搜索充值记录时 COUNT 的安全上限，
// 防止对超大表执行无界 COUNT 触发 DoS。
const searchTopUpCountHardLimit = 10000

// SearchUserTopUps 按订单号搜索某用户的充值记录

// SearchAllTopUps 按订单号搜索全平台充值记录（管理员使用，不限制时间窗口）

// ManualCompleteTopUp 管理员手动完成订单并给用户充值
func ManualCompleteTopUp(tradeNo string, callerIp string) error {
	result, err := settleTopUp(topUpSettlementRequest{tradeNo: tradeNo})
	if err != nil {
		return err
	}
	if !result.alreadyDone {
		order := result.order
		RecordTopupLog(order.UserId, fmt.Sprintf("管理员补单成功，充值金额: %v，支付金额：%f", logger.FormatQuota(result.quota), order.Money), callerIp, order.PaymentMethod, "admin")
	}
	return nil
}
func RechargeCreem(referenceId string, customerEmail string, customerName string, callerIp string) error {
	result, err := settleTopUp(topUpSettlementRequest{tradeNo: referenceId, provider: PaymentProviderCreem, customerEmail: customerEmail})
	if err != nil {
		return fmt.Errorf("充值失败，请稍后重试: %w", err)
	}
	if !result.alreadyDone {
		order := result.order
		RecordTopupLog(order.UserId, fmt.Sprintf("使用Creem充值成功，充值额度: %v，支付金额：%.2f", result.quota, order.Money), callerIp, order.PaymentMethod, PaymentMethodCreem)
	}
	return nil
}

func RechargeWaffo(tradeNo string, callerIp string) error {
	result, err := settleTopUp(topUpSettlementRequest{tradeNo: tradeNo, provider: PaymentProviderWaffo})
	if err != nil {
		return fmt.Errorf("充值失败，请稍后重试: %w", err)
	}
	if !result.alreadyDone {
		order := result.order
		RecordTopupLog(order.UserId, fmt.Sprintf("Waffo充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(result.quota), order.Money), callerIp, order.PaymentMethod, PaymentMethodWaffo)
	}
	return nil
}

func RechargeWaffoPancake(tradeNo string) error {
	result, err := settleTopUp(topUpSettlementRequest{tradeNo: tradeNo, provider: PaymentProviderWaffoPancake})
	if err != nil {
		return fmt.Errorf("充值失败，请稍后重试: %w", err)
	}
	if !result.alreadyDone {
		order := result.order
		RecordLog(order.UserId, LogTypeTopup, fmt.Sprintf("Waffo Pancake充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(result.quota), order.Money))
	}
	return nil
}
