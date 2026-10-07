package model

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrNoActiveSubscription = errors.New("no active subscription")
	ErrSubscriptionQuota    = errors.New("subscription quota insufficient")
)

func lockSubscriptionOwner(tx *gorm.DB, userID int) error {
	var user User
	return lockForUpdate(tx).Select("id").First(&user, userID).Error
}

func replaySubscriptionReservation(tx *gorm.DB, record *SubscriptionPreConsumeRecord, userID int) (*SubscriptionPreConsumeResult, error) {
	if record.UserId != userID {
		return nil, errors.New("subscription reservation belongs to another user")
	}
	if record.Status != "consumed" {
		return nil, errors.New("subscription request already settled or refunded")
	}
	var subscription UserSubscription
	if err := tx.First(&subscription, record.UserSubscriptionId).Error; err != nil {
		return nil, err
	}
	return &SubscriptionPreConsumeResult{UserSubscriptionId: subscription.Id, PreConsumed: record.PreConsumed, AmountTotal: subscription.AmountTotal, AmountUsedBefore: subscription.AmountUsed, AmountUsedAfter: subscription.AmountUsed}, nil
}

func PreConsumeUserSubscription(requestID string, userID int, modelName string, quotaType int, amount int64) (*SubscriptionPreConsumeResult, error) {
	if userID <= 0 || strings.TrimSpace(requestID) == "" || len(requestID) > 64 || amount <= 0 {
		return nil, errors.New("订阅额度预扣参数无效")
	}
	var result *SubscriptionPreConsumeResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockSubscriptionOwner(tx, userID); err != nil {
			return err
		}
		var existing SubscriptionPreConsumeRecord
		err := tx.Where("request_id = ?", requestID).First(&existing).Error
		if err == nil {
			result, err = replaySubscriptionReservation(tx, &existing, userID)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		order, err := furtherSubscriptionOrderTx(tx, userID)
		if err != nil {
			return err
		}
		now := furtherDBTimestampTx(tx)
		var subscriptions []UserSubscription
		if err := lockForUpdate(tx).Where("user_id = ? AND status = ? AND start_time <= ? AND end_time > ?", userID, "active", now, now).Order(order).Find(&subscriptions).Error; err != nil {
			return err
		}
		if len(subscriptions) == 0 {
			return ErrNoActiveSubscription
		}
		for index := range subscriptions {
			subscription := &subscriptions[index]
			plan, err := getSubscriptionPlanByIdTx(tx, subscription.PlanId)
			if err != nil {
				return err
			}
			if err := maybeResetUserSubscriptionWithPlanTx(tx, subscription, plan, now); err != nil {
				return err
			}
			policy, eligible, err := furtherDiscountPolicyTx(tx, subscription, plan)
			if err != nil {
				return err
			}
			if !eligible {
				continue
			}
			charged := amount
			if policy != nil {
				charged, _ = furtherDiscountAmounts(amount, policy.ModelMultiplier(modelName), policy, 0)
			}
			if subscription.AmountTotal > 0 && subscription.AmountTotal-subscription.AmountUsed < charged {
				continue
			}
			after, err := checkedSubscriptionUsage(subscription.AmountUsed, charged)
			if err != nil {
				return err
			}
			record := SubscriptionPreConsumeRecord{RequestId: requestID, UserId: userID, UserSubscriptionId: subscription.Id, PreConsumed: charged, Status: "consumed"}
			inserted := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "request_id"}}, DoNothing: true}).Create(&record)
			if inserted.Error != nil {
				return inserted.Error
			}
			if inserted.RowsAffected == 0 {
				if err := tx.Where("request_id = ?", requestID).First(&existing).Error; err != nil {
					return err
				}
				result, err = replaySubscriptionReservation(tx, &existing, userID)
				return err
			}
			if err := reserveFurtherDiscountTx(tx, requestID, subscription, policy, modelName, amount, charged); err != nil {
				return err
			}
			if err := tx.Model(&UserSubscription{}).Where("id = ?", subscription.Id).Updates(map[string]any{"amount_used": after, "updated_at": now}).Error; err != nil {
				return err
			}
			result = &SubscriptionPreConsumeResult{UserSubscriptionId: subscription.Id, PreConsumed: charged, AmountTotal: subscription.AmountTotal, AmountUsedBefore: subscription.AmountUsed, AmountUsedAfter: after}
			return nil
		}
		return fmt.Errorf("%w, need=%d", ErrSubscriptionQuota, amount)
	})
	return result, err
}

func checkedSubscriptionUsage(current, delta int64) (int64, error) {
	value := big.NewInt(current)
	value.Add(value, big.NewInt(delta))
	if value.Sign() < 0 {
		return 0, nil
	}
	if !value.IsInt64() {
		return 0, errors.New("订阅用量超出可用范围")
	}
	return value.Int64(), nil
}

func RefundSubscriptionPreConsume(requestID string) error {
	if strings.TrimSpace(requestID) == "" {
		return errors.New("订阅请求编号无效")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var record SubscriptionPreConsumeRecord
		if err := tx.Where("request_id = ?", requestID).First(&record).Error; err != nil {
			return err
		}
		if err := lockSubscriptionOwner(tx, record.UserId); err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&record).Error; err != nil {
			return err
		}
		if record.Status == "refunded" || record.Status == "settled" {
			return nil
		}
		if err := refundFurtherDiscountTx(tx, requestID); err != nil {
			return err
		}
		if record.PreConsumed > 0 {
			var subscription UserSubscription
			if err := lockForUpdate(tx).First(&subscription, record.UserSubscriptionId).Error; err != nil {
				return err
			}
			after, err := checkedSubscriptionUsage(subscription.AmountUsed, -record.PreConsumed)
			if err != nil {
				return err
			}
			if err := tx.Model(&subscription).Updates(map[string]any{"amount_used": after, "updated_at": common.GetTimestamp()}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&record).Updates(map[string]any{"status": "refunded", "updated_at": common.GetTimestamp()}).Error
	})
}

func PostConsumeUserSubscriptionDelta(id int, delta int64) error {
	if id <= 0 {
		return errors.New("订阅编号无效")
	}
	if delta == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var subscription UserSubscription
		if err := lockForUpdate(tx).First(&subscription, id).Error; err != nil {
			return err
		}
		after, err := checkedSubscriptionUsage(subscription.AmountUsed, delta)
		if err != nil {
			return err
		}
		if subscription.AmountTotal > 0 && after > subscription.AmountTotal {
			return fmt.Errorf("subscription used exceeds total, used=%d total=%d", after, subscription.AmountTotal)
		}
		return tx.Model(&subscription).Updates(map[string]any{"amount_used": after, "updated_at": common.GetTimestamp()}).Error
	})
}
