package middleware

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/go-redis/redis/v8"
)

var redisModelSuccessLeaseDuration = 90 * time.Second

const redisModelSuccessAdmissionScript = `
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1] - ARGV[2])
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', ARGV[1])
if redis.call('ZCARD', KEYS[1]) + redis.call('ZCARD', KEYS[2]) >= tonumber(ARGV[3]) then
  return 0
end
redis.call('ZADD', KEYS[2], ARGV[1] + ARGV[4], ARGV[5])
redis.call('PEXPIRE', KEYS[2], ARGV[4])
return 1
`

const redisModelSuccessRenewScript = `
if not redis.call('ZSCORE', KEYS[1], ARGV[1]) then return 0 end
redis.call('ZADD', KEYS[1], ARGV[2] + ARGV[3], ARGV[1])
redis.call('PEXPIRE', KEYS[1], ARGV[3])
return 1
`

const redisModelSuccessCompleteScript = `
redis.call('ZREM', KEYS[2], ARGV[1])
if tonumber(ARGV[2]) == 1 then
  redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[3] - ARGV[4])
  redis.call('ZADD', KEYS[1], ARGV[3], ARGV[1])
  redis.call('PEXPIRE', KEYS[1], ARGV[4])
end
return 1
`

type redisModelSuccessReservation struct {
	rdb            *redis.Client
	keys           []string
	id             string
	window         time.Duration
	lease          time.Duration
	once           sync.Once
	healthy        atomic.Bool
	requestContext context.Context
	cancelRequest  context.CancelFunc
	cancelRenewal  context.CancelFunc
	renewalDone    chan struct{}
}

func reserveRedisModelSuccess(ctx context.Context, rdb *redis.Client, userID string, maxCount int, durationSeconds int64) (*redisModelSuccessReservation, bool, error) {
	if maxCount <= 0 {
		return nil, true, nil
	}
	if rdb == nil || durationSeconds <= 0 || durationSeconds > 24*60*60 {
		return nil, false, fmt.Errorf("invalid model success rate limit configuration")
	}
	reservation := &redisModelSuccessReservation{
		rdb:    rdb,
		keys:   []string{"rateLimit:v2:model:{" + userID + "}:success", "rateLimit:v2:model:{" + userID + "}:active"},
		id:     common.GetUUID(),
		window: time.Duration(durationSeconds) * time.Second,
		lease:  redisModelSuccessLeaseDuration,
	}
	requestContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	admitted, err := rdb.Eval(requestContext, redisModelSuccessAdmissionScript, reservation.keys,
		time.Now().UnixMilli(), reservation.window.Milliseconds(), maxCount, reservation.lease.Milliseconds(), reservation.id).Int()
	if err != nil || admitted != 1 {
		return nil, false, err
	}
	reservation.healthy.Store(true)
	return reservation, true, nil
}

func (r *redisModelSuccessReservation) startRenewal(requestContext context.Context, cancelRequest context.CancelFunc) {
	r.requestContext, r.cancelRequest = requestContext, cancelRequest
	renewalContext, cancelRenewal := context.WithCancel(context.Background())
	r.cancelRenewal = cancelRenewal
	r.renewalDone = make(chan struct{})
	go func() {
		defer close(r.renewalDone)
		ticker := time.NewTicker(r.lease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-requestContext.Done():
				return
			case <-renewalContext.Done():
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(renewalContext, 5*time.Second)
				kept, err := r.rdb.Eval(ctx, redisModelSuccessRenewScript, []string{r.keys[1]}, r.id, time.Now().UnixMilli(), r.lease.Milliseconds()).Int()
				cancel()
				if renewalContext.Err() != nil {
					return
				}
				if err != nil || kept != 1 {
					// Losing distributed capacity must terminate the request, never
					// silently release a live SSE stream and admit more work.
					r.healthy.Store(false)
					cancelRequest()
					return
				}
			}
		}
	}()
}

func (r *redisModelSuccessReservation) complete(success bool) {
	if r == nil {
		return
	}
	r.once.Do(func() {
		if r.cancelRenewal != nil {
			r.cancelRenewal()
			<-r.renewalDone
		}
		completed := 0
		if success && r.healthy.Load() && (r.requestContext == nil || r.requestContext.Err() == nil) {
			completed = 1
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := r.rdb.Eval(ctx, redisModelSuccessCompleteScript, r.keys, r.id, completed, time.Now().UnixMilli(), r.window.Milliseconds()).Result()
		if err != nil {
			common.SysError("model success reservation cleanup failed")
		}
		if r.cancelRequest != nil {
			r.cancelRequest()
		}
	})
}
