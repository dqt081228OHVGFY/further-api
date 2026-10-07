package model

import (
	"context"
	"fmt"
	"maps"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var gatewayOptionMutationMu sync.Mutex

func UpdateOption(key, value string) error {
	return UpdateOptionContext(context.Background(), key, value)
}
func UpdateOptionContext(ctx context.Context, key, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if key == retiredChatOptionKey {
		return ErrRetiredChatOption
	}
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	if IsPasskeyDomainOption(key) {
		_, err := UpdatePasskeyDomainOptionsContext(ctx, map[string]string{key: value}, false, "")
		return err
	}
	if IsRequestPolicyOption(key) {
		return UpdateRequestPolicyOptionsContext(ctx, map[string]string{key: value})
	}
	if IsModelPricingOption(key) {
		return UpdateModelPricingOptionsContext(ctx, map[string]string{key: value})
	}
	return UpdateOptionsBulkContext(ctx, map[string]string{key: value})
}

func UpdateOptionsBulk(values map[string]string) error {
	return UpdateOptionsBulkContext(context.Background(), values)
}
func UpdateOptionsBulkContext(ctx context.Context, values map[string]string) error {
	return mutateGatewayOptionsContext(ctx, values, nil)
}

func mutateGatewayOptionsContext(ctx context.Context, values map[string]string, guard func(*gorm.DB) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(values) == 0 {
		return nil
	}
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	values = maps.Clone(values)
	for key := range values {
		if IsPasskeyDomainOption(key) {
			_, err := UpdatePasskeyDomainOptionsContext(ctx, values, false, "")
			return err
		}
	}
	pricing := 0
	for key := range values {
		if IsModelPricingOption(key) {
			pricing++
		}
	}
	if pricing > 0 {
		if pricing != len(values) {
			return fmt.Errorf("模型定价须通过定价配置接口单独提交")
		}
		return UpdateModelPricingOptionsContext(ctx, values)
	}
	gatewayOptionMutationMu.Lock()
	defer gatewayOptionMutationMu.Unlock()
	var policy *RequestPolicySnapshot
	for key := range values {
		if IsRequestPolicyOption(key) {
			requestPolicyOptionMutex.Lock()
			defer requestPolicyOptionMutex.Unlock()
			break
		}
	}
	plan, err := prepareGatewayOptionPlan(values, false)
	if err != nil {
		return err
	}
	for key := range plan.values {
		if IsRequestPolicyOption(key) {
			options := maps.Clone(CurrentRequestPolicy().Options)
			for key, value := range plan.values {
				if IsRequestPolicyOption(key) {
					options[key] = value
				}
			}
			policy, err = BuildRequestPolicy(options)
			if err != nil {
				return err
			}
			break
		}
	}
	if err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if guard != nil {
			if err := guard(tx); err != nil {
				return err
			}
		}
		return persistGatewayOptionsTx(tx, plan.values)
	}); err != nil {
		return err
	}
	if err := publishGatewayOptionPlan(plan); err != nil {
		return err
	}
	if policy != nil {
		requestPolicySnapshot.Store(policy)
	}
	return nil
}

func persistGatewayOptionsTx(tx *gorm.DB, values map[string]string) error {
	for _, key := range sortedGatewayOptionKeys(values) {
		value := values[key]
		row := Option{Key: key, Value: value}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		if err := tx.Model(&Option{}).Where(commonKeyCol+" = ?", key).Update("value", value).Error; err != nil {
			return err
		}
	}
	return nil
}
