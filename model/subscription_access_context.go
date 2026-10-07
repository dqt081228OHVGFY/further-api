package model

import (
	"context"
	"github.com/dqt081228OHVGFY/further-api/common"
)

func FurtherSubscriptionModelAllowedContext(ctx context.Context, userID int, modelName string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if modelName == "" || DB == nil {
		return true, nil
	}
	db := DB.WithContext(ctx)
	installed := furtherAccessInstalled.Load() == DB || db.Migrator().HasTable(&FurtherModelAccess{})
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !installed {
		return true, nil
	}
	var restrictions []Model
	if err := db.Model(&Model{}).Joins("JOIN further_model_accesses AS access ON access.model_id = models.id").Where("access.subscription_only = ? AND (models.model_name = ? OR models.name_rule <> ?)", true, modelName, NameRuleExact).Find(&restrictions).Error; err != nil {
		return false, err
	}
	matched := make(map[int]bool)
	for _, restriction := range restrictions {
		if restriction.MatchesName(modelName) {
			matched[restriction.Id] = true
		}
	}
	if len(matched) == 0 {
		return true, nil
	}
	entitlements, err := GetFurtherActiveEntitlementsContext(ctx, userID, modelName)
	if err != nil {
		return false, err
	}
	for _, entitlement := range entitlements {
		for _, rule := range entitlement.Policy.Rules() {
			if matched[rule.ModelID] {
				return true, nil
			}
			if rule.Model != "" && (&Model{ModelName: rule.Model, NameRule: rule.NameRule}).MatchesName(modelName) {
				return true, nil
			}
		}
	}
	return false, nil
}

func GetSubscriptionPlanByIdContext(ctx context.Context, id int) (*SubscriptionPlan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return getSubscriptionPlanByIdTx(DB.WithContext(ctx), id)
}

// A refund caller supplies its own bounded recovery context; it does not use
// the canceled customer request for corrective writes.
func DecreaseUserQuotaContext(ctx context.Context, id, quota int, syncDB bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if quota < 0 || quota > common.MaxWalletQuota {
		return ErrWalletQuotaLimitExceeded
	}
	if quota == 0 {
		return nil
	}
	input := ledgerReservationInput{id: id}
	if err := applyLedgerDeltaContext(ctx, input, -quota, !syncDB); err != nil {
		return err
	}
	if common.RedisEnabled && common.RDB != nil {
		_, _ = mutateLedgerCacheContext(ctx, input, -quota, false)
	}
	return nil
}

func DecreaseTokenQuotaContext(ctx context.Context, id int, key string, quota int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if quota < 0 || quota > common.MaxWalletQuota {
		return ErrWalletQuotaLimitExceeded
	}
	if quota == 0 {
		return nil
	}
	input := ledgerReservationInput{id: id, key: key, token: true}
	if err := applyLedgerDeltaContext(ctx, input, -quota, true); err != nil {
		return err
	}
	if common.RedisEnabled && common.RDB != nil {
		_, _ = mutateLedgerCacheContext(ctx, input, -quota, false)
	}
	return nil
}
