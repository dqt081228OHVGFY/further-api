package common

import (
	"context"
	"errors"
	"sync"

	"github.com/dqt081228OHVGFY/further-api/constant"
)

var ErrPayloadMemoryCapacity = errors.New("网关请求处理资源正忙，请稍后重试。")
var payloadMemoryPool struct {
	sync.Mutex
	used int64
}

type PayloadMemoryReservation struct {
	weight   int64
	released bool
}

func ReservePayloadMemory(ctx context.Context, weight int64) (*PayloadMemoryReservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reservation := &PayloadMemoryReservation{}
	if err := reservation.Resize(weight); err != nil {
		return nil, err
	}
	return reservation, nil
}

func (reservation *PayloadMemoryReservation) Resize(weight int64) error {
	payloadMemoryPool.Lock()
	defer payloadMemoryPool.Unlock()
	if reservation.released || weight < 0 {
		return ErrPayloadMemoryCapacity
	}
	limitMB := constant.ExternalPayloadMemoryBudgetMB
	if limitMB <= 0 {
		limitMB = 2048
	}
	if int64(limitMB) > (1<<62)/(1<<20) {
		return ErrPayloadMemoryCapacity
	}
	limit := int64(limitMB) << 20
	delta := weight - reservation.weight
	if delta > 0 && (weight > limit || payloadMemoryPool.used > limit-delta) {
		return ErrPayloadMemoryCapacity
	}
	payloadMemoryPool.used += delta
	reservation.weight = weight
	return nil
}

func (reservation *PayloadMemoryReservation) Release() {
	if reservation == nil {
		return
	}
	payloadMemoryPool.Lock()
	defer payloadMemoryPool.Unlock()
	if reservation.released {
		return
	}
	reservation.released = true
	payloadMemoryPool.used -= reservation.weight
}

func PayloadMemoryReservedBytes() int64 {
	payloadMemoryPool.Lock()
	defer payloadMemoryPool.Unlock()
	return payloadMemoryPool.used
}
