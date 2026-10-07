package model

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"time"
)

func CreateAuthFlowContext(ctx context.Context, input AuthFlowCreate) (string, *AuthFlow, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	return createAuthFlowWithTx(DB.WithContext(ctx), input)
}

func GetAuthFlowContext(ctx context.Context, token string, match AuthFlowMatch) (*AuthFlow, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if token == "" || match.Purpose == "" {
		return nil, ErrAuthFlowInvalid
	}
	var flow AuthFlow
	if err := applyAuthFlowMatch(DB.WithContext(ctx), token, match).First(&flow).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAuthFlowInvalid
		}
		return nil, err
	}
	if flow.ConsumedAt != nil {
		return nil, ErrAuthFlowConsumed
	}
	if !flow.ExpiresAt.After(time.Now()) {
		return nil, ErrAuthFlowExpired
	}
	return &flow, nil
}

func ConsumeAuthFlowContext(ctx context.Context, token string, match AuthFlowMatch) (*AuthFlow, error) {
	return ConsumeAuthFlowWithActionContext(ctx, token, match, nil)
}

func ConsumeAuthFlowWithActionContext(ctx context.Context, token string, match AuthFlowMatch, action func(*gorm.DB, *AuthFlow) error) (*AuthFlow, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if token == "" || match.Purpose == "" {
		return nil, ErrAuthFlowInvalid
	}
	var flow AuthFlow
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		claimed := applyAuthFlowMatch(tx.Model(&AuthFlow{}), token, match).Where("consumed_at IS NULL AND expires_at > ?", now).Update("consumed_at", now)
		if claimed.Error != nil {
			return claimed.Error
		}
		if err := applyAuthFlowMatch(tx, token, match).First(&flow).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAuthFlowInvalid
			}
			return err
		}
		if claimed.RowsAffected != 1 {
			if flow.ConsumedAt != nil {
				return ErrAuthFlowConsumed
			}
			if !flow.ExpiresAt.After(now) {
				return ErrAuthFlowExpired
			}
			return ErrAuthFlowInvalid
		}
		if action != nil {
			return action(tx, &flow)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Join(err, ctx.Err())
	}
	return &flow, nil
}
