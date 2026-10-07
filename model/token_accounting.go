package model

import (
	"context"
	"errors"
)

func IncreaseTokenQuota(id int, key string, quota int) error {
	return IncreaseTokenQuotaContext(context.Background(), id, key, quota)
}
func DecreaseTokenQuota(id int, key string, quota int) error {
	return DecreaseTokenQuotaContext(context.Background(), id, key, quota)
}
func increaseTokenQuota(id, quota int) error {
	if quota < 0 {
		return errors.New("入账额度不能为负数")
	}
	return applyLedgerDeltaContext(context.Background(), ledgerReservationInput{id: id, token: true}, quota, false)
}
func decreaseTokenQuota(id, quota int) error {
	if quota < 0 {
		return errors.New("扣减额度不能为负数")
	}
	return applyLedgerDeltaContext(context.Background(), ledgerReservationInput{id: id, token: true}, -quota, false)
}

func TryReserveUserQuota(id, quota int) (bool, error) {
	return TryReserveUserQuotaContext(context.Background(), id, quota)
}
func TryReserveTokenQuota(id int, key string, quota int, unlimited bool) (bool, error) {
	return TryReserveTokenQuotaContext(context.Background(), id, key, quota, unlimited)
}
