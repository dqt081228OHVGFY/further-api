package model

import (
	"errors"
	"fmt"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// One durable reservation per gateway request binds the selected policy,
// preserves its multiplier across retries and serializes savings-cap usage.
type FurtherDiscountReservation struct {
	RequestID       string `gorm:"primaryKey;size:64"`
	UserID          int    `gorm:"index"`
	SubscriptionID  int    `gorm:"index"`
	Multiplier      float64
	BaseReserved    int64
	ChargedReserved int64
	SavingsReserved int64
	BaseSettled     int64
	ChargedSettled  int64
	SavingsSettled  int64
	Status          string `gorm:"size:16;index"`
	CreatedAt       int64
	UpdatedAt       int64
}

func furtherActiveUpgradeChainTx(tx *gorm.DB, userID, planID int, seen map[int]bool) (bool, error) {
	if planID == 0 {
		return true, nil
	}
	if seen[planID] || len(seen) >= 32 {
		return false, nil
	}
	nextSeen := make(map[int]bool, len(seen)+1)
	for id, value := range seen {
		nextSeen[id] = value
	}
	nextSeen[planID] = true
	now := common.GetTimestamp()
	var subs []UserSubscription
	if err := tx.Where("user_id = ? AND plan_id = ? AND status = ? AND start_time <= ? AND end_time > ?", userID, planID, "active", now, now).Find(&subs).Error; err != nil {
		return false, err
	}
	for _, sub := range subs {
		var policy FurtherUserSubscriptionPolicy
		err := tx.First(&policy, "subscription_id = ?", sub.Id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			var template FurtherSubscriptionPolicy
			err = tx.First(&template, "plan_id = ?", planID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return true, nil
			}
			if err != nil {
				return false, err
			}
			policy = furtherPolicySnapshot(template, sub.Id)
		} else if err != nil {
			return false, err
		}
		valid, err := furtherActiveUpgradeChainTx(tx, userID, policy.UpgradeFromPlanID, nextSeen)
		if err != nil {
			return false, err
		}
		if valid {
			return true, nil
		}
	}
	return false, nil
}

func furtherDiscountPolicyTx(tx *gorm.DB, sub *UserSubscription, plan *SubscriptionPlan) (*FurtherUserSubscriptionPolicy, bool, error) {
	if !tx.Migrator().HasTable(&FurtherSubscriptionPolicy{}) {
		return nil, true, nil
	}
	var policy FurtherUserSubscriptionPolicy
	err := lockForUpdate(tx).First(&policy, "subscription_id = ?", sub.Id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		var template FurtherSubscriptionPolicy
		err = tx.First(&template, "plan_id = ?", plan.Id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, true, nil
		}
		if err != nil {
			return nil, false, err
		}
		policy = furtherPolicySnapshot(template, sub.Id)
		if err := tx.Create(&policy).Error; err != nil {
			return nil, false, err
		}
	} else if err != nil {
		return nil, false, err
	}
	valid, err := furtherActiveUpgradeChainTx(tx, sub.UserId, policy.UpgradeFromPlanID, map[int]bool{sub.PlanId: true})
	return &policy, valid, err
}

func furtherDiscountAmounts(base int64, multiplier float64, policy *FurtherUserSubscriptionPolicy, ownReserved int64) (charged, savings int64) {
	if base <= 0 {
		return 0, 0
	}
	if multiplier <= 0 || multiplier > 1 {
		multiplier = 1
	}
	charged = decimal.NewFromInt(base).Mul(decimal.NewFromFloat(multiplier)).Ceil().IntPart()
	savings = max(int64(0), base-charged)
	if policy.DiscountSavingsLimit > 0 {
		savings = min(savings, max(int64(0), policy.DiscountSavingsLimit-policy.DiscountSavingsUsed-policy.DiscountSavingsReserved+ownReserved))
	}
	return base - savings, savings
}

func reserveFurtherDiscountTx(tx *gorm.DB, requestID string, sub *UserSubscription, policy *FurtherUserSubscriptionPolicy, modelName string, base, charged int64) error {
	if policy == nil {
		return nil
	}
	savings := base - charged
	if err := tx.Model(policy).Update("discount_savings_reserved", policy.DiscountSavingsReserved+savings).Error; err != nil {
		return err
	}
	now := common.GetTimestamp()
	return tx.Create(&FurtherDiscountReservation{RequestID: requestID, UserID: sub.UserId, SubscriptionID: sub.Id, Multiplier: policy.ModelMultiplier(modelName), BaseReserved: base, ChargedReserved: charged, SavingsReserved: savings, Status: "reserved", CreatedAt: now, UpdatedAt: now}).Error
}

func GetFurtherDiscountReservation(requestID string) (*FurtherDiscountReservation, error) {
	if !DB.Migrator().HasTable(&FurtherDiscountReservation{}) {
		return nil, nil
	}
	var record FurtherDiscountReservation
	err := DB.First(&record, "request_id = ?", requestID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &record, err
}

func withFurtherSubscriptionReservationTx(requestID string, action func(*gorm.DB) error) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var identity FurtherDiscountReservation
		if err := tx.Select("request_id", "user_id").First(&identity, "request_id = ?", requestID).Error; err != nil {
			return err
		}
		if err := lockSubscriptionOwner(tx, identity.UserID); err != nil {
			return err
		}
		return action(tx)
	})
}

// ResizeFurtherSubscriptionReservation updates subscription quota, the
// original further-api reservation and savings budget in a single transaction.
func ResizeFurtherSubscriptionReservation(requestID string, targetBase int64) (int64, error) {
	if targetBase < 0 || targetBase > int64(common.MaxWalletQuota) {
		return 0, errors.New("invalid reservation amount")
	}
	var charged int64
	err := withFurtherSubscriptionReservationTx(requestID, func(tx *gorm.DB) error {
		var record FurtherDiscountReservation
		if err := lockForUpdate(tx).First(&record, "request_id = ?", requestID).Error; err != nil {
			return err
		}
		if record.Status != "reserved" {
			return errors.New("subscription reservation is no longer pending")
		}
		var sub UserSubscription
		if err := lockForUpdate(tx).First(&sub, record.SubscriptionID).Error; err != nil {
			return err
		}
		var policy FurtherUserSubscriptionPolicy
		if err := lockForUpdate(tx).First(&policy, "subscription_id = ?", sub.Id).Error; err != nil {
			return err
		}
		newCharge, newSavings := furtherDiscountAmounts(targetBase, record.Multiplier, &policy, record.SavingsReserved)
		delta := newCharge - record.ChargedReserved
		if delta > 0 && sub.AmountTotal > 0 && sub.AmountTotal-sub.AmountUsed < delta {
			return errors.New("subscription quota insufficient")
		}
		if err := tx.Model(&sub).Update("amount_used", max(int64(0), sub.AmountUsed+delta)).Error; err != nil {
			return err
		}
		if err := tx.Model(&policy).Update("discount_savings_reserved", max(int64(0), policy.DiscountSavingsReserved-record.SavingsReserved+newSavings)).Error; err != nil {
			return err
		}
		if err := tx.Model(&SubscriptionPreConsumeRecord{}).Where("request_id = ? AND status = ?", requestID, "consumed").Update("pre_consumed", newCharge).Error; err != nil {
			return err
		}
		if err := tx.Model(&record).Updates(map[string]any{"base_reserved": targetBase, "charged_reserved": newCharge, "savings_reserved": newSavings, "updated_at": common.GetTimestamp()}).Error; err != nil {
			return err
		}
		charged = newCharge
		return nil
	})
	return charged, err
}

func SettleFurtherSubscriptionReservation(requestID string, actualBase int64) (int64, error) {
	if actualBase < 0 || actualBase > int64(common.MaxWalletQuota) {
		return 0, errors.New("invalid settlement amount")
	}
	var charged int64
	err := withFurtherSubscriptionReservationTx(requestID, func(tx *gorm.DB) error {
		var record FurtherDiscountReservation
		if err := lockForUpdate(tx).First(&record, "request_id = ?", requestID).Error; err != nil {
			return err
		}
		if record.Status == "settled" {
			if record.BaseSettled != actualBase {
				return errors.New("settlement amount conflicts with completed request")
			}
			charged = record.ChargedSettled
			return nil
		}
		if record.Status != "reserved" {
			return errors.New("refunded request cannot be settled")
		}
		var sub UserSubscription
		if err := lockForUpdate(tx).First(&sub, record.SubscriptionID).Error; err != nil {
			return err
		}
		var policy FurtherUserSubscriptionPolicy
		if err := lockForUpdate(tx).First(&policy, "subscription_id = ?", sub.Id).Error; err != nil {
			return err
		}
		newCharge, newSavings := furtherDiscountAmounts(actualBase, record.Multiplier, &policy, record.SavingsReserved)
		delta := newCharge - record.ChargedReserved
		if delta > 0 && sub.AmountTotal > 0 && sub.AmountTotal-sub.AmountUsed < delta {
			return fmt.Errorf("subscription quota insufficient, need=%d", delta)
		}
		if err := tx.Model(&sub).Update("amount_used", max(int64(0), sub.AmountUsed+delta)).Error; err != nil {
			return err
		}
		if err := tx.Model(&policy).Updates(map[string]any{"discount_savings_reserved": max(int64(0), policy.DiscountSavingsReserved-record.SavingsReserved), "discount_savings_used": policy.DiscountSavingsUsed + newSavings}).Error; err != nil {
			return err
		}
		if err := tx.Model(&SubscriptionPreConsumeRecord{}).Where("request_id = ?", requestID).Update("status", "settled").Error; err != nil {
			return err
		}
		if err := tx.Model(&record).Updates(map[string]any{"status": "settled", "base_settled": actualBase, "charged_settled": newCharge, "savings_settled": newSavings, "savings_reserved": 0, "updated_at": common.GetTimestamp()}).Error; err != nil {
			return err
		}
		charged = newCharge
		return nil
	})
	return charged, err
}

func refundFurtherDiscountTx(tx *gorm.DB, requestID string) error {
	if !tx.Migrator().HasTable(&FurtherDiscountReservation{}) {
		return nil
	}
	var record FurtherDiscountReservation
	err := lockForUpdate(tx).First(&record, "request_id = ?", requestID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if record.Status == "refunded" || record.Status == "settled" {
		return nil
	}
	var policy FurtherUserSubscriptionPolicy
	if err := lockForUpdate(tx).First(&policy, "subscription_id = ?", record.SubscriptionID).Error; err != nil {
		return err
	}
	if err := tx.Model(&policy).Update("discount_savings_reserved", max(int64(0), policy.DiscountSavingsReserved-record.SavingsReserved)).Error; err != nil {
		return err
	}
	return tx.Model(&record).Updates(map[string]any{"status": "refunded", "savings_reserved": 0, "updated_at": common.GetTimestamp()}).Error
}
