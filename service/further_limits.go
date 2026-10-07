package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
)

const furtherLeaseTTL = 2 * time.Minute
const furtherMemorySubjects = 10000
const furtherMemoryLeases = 100000

// The server clock determines both the minute window and lease expiry. No
// client-provided timestamp or process-local clock can enlarge a Redis window.
const furtherAcquireScript = `
local clock = redis.call('TIME')
local now = tonumber(clock[1])*1000 + math.floor(tonumber(clock[2])/1000)
local minute = math.floor(now/60000)
local count = 0
if tonumber(redis.call('HGET', KEYS[1], 'window')) == minute then
  count = tonumber(redis.call('HGET', KEYS[1], 'count')) or 0
end
local rpm = tonumber(ARGV[1])
local concurrent = tonumber(ARGV[2])
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', now)
if rpm > 0 and count >= rpm then
  return {0, 1, math.max(1, math.ceil((60000-now%60000)/1000))}
end
if concurrent > 0 and redis.call('ZCARD', KEYS[2]) >= concurrent then
  return {0, 2, 1}
end
if rpm > 0 then
  redis.call('HSET', KEYS[1], 'window', minute, 'count', count+1)
  redis.call('PEXPIRE', KEYS[1], 120000)
end
if concurrent > 0 then
  redis.call('ZADD', KEYS[2], now+tonumber(ARGV[4]), ARGV[3])
  redis.call('PEXPIRE', KEYS[2], tonumber(ARGV[4])*2)
end
return {1, 0, 0}
`

const furtherRenewScript = `
local clock = redis.call('TIME')
local now = tonumber(clock[1])*1000 + math.floor(tonumber(clock[2])/1000)
local expiry = tonumber(redis.call('ZSCORE', KEYS[1], ARGV[1]))
if not expiry or expiry <= now then return 0 end
redis.call('ZADD', KEYS[1], now+tonumber(ARGV[2]), ARGV[1])
redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[2])*2)
return 1
`

const furtherReleaseScript = `
redis.call('ZREM', KEYS[1], ARGV[1])
if redis.call('ZCARD', KEYS[1]) == 0 then redis.call('DEL', KEYS[1]) end
return 1
`

type FurtherLimitError struct {
	Reason     string
	RetryAfter int
	Cause      error
}

func (e *FurtherLimitError) Error() string {
	switch e.Reason {
	case "rpm":
		return "已达到当前策略的每分钟请求上限"
	case "concurrency":
		return "已达到当前策略的并发请求上限"
	default:
		return "请求限流服务暂不可用，请稍后重试"
	}
}

func (e *FurtherLimitError) Unwrap() error { return e.Cause }

type FurtherLimitLease struct {
	ctx     context.Context
	cancel  context.CancelFunc
	once    sync.Once
	stop    chan struct{}
	release func()
	renew   func() error
}

func (lease *FurtherLimitLease) Context() context.Context { return lease.ctx }

// Release is idempotent across normal completion, refund, retry and cancellation.
func (lease *FurtherLimitLease) Release() {
	if lease == nil {
		return
	}
	lease.once.Do(func() {
		close(lease.stop)
		if lease.release != nil {
			lease.release()
		}
		lease.cancel()
	})
}

func (lease *FurtherLimitLease) keepAlive() {
	ticker := time.NewTicker(furtherLeaseTTL / 3)
	defer ticker.Stop()
	for {
		select {
		case <-lease.stop:
			return
		case <-lease.ctx.Done():
			lease.Release()
			return
		case <-ticker.C:
			if !lease.renewOrCancel() {
				return
			}
		}
	}
}

func (lease *FurtherLimitLease) renewOrCancel() bool {
	if err := lease.renew(); err != nil {
		common.SysError("Further concurrency lease lost; cancelling upstream request")
		lease.Release()
		return false
	}
	return true
}

type furtherMemorySubject struct {
	minute int64
	count  int
	leases map[string]int64
}

type furtherMemoryStore struct {
	mu        sync.Mutex
	subjects  map[string]*furtherMemorySubject
	leases    int
	lastSweep int64
}

var furtherLocalLimits = furtherMemoryStore{subjects: make(map[string]*furtherMemorySubject)}

func (store *furtherMemoryStore) prune(now int64) {
	for key, subject := range store.subjects {
		for id, expiry := range subject.leases {
			if expiry <= now {
				delete(subject.leases, id)
				store.leases--
			}
		}
		if len(subject.leases) == 0 && subject.minute < now/60000 {
			delete(store.subjects, key)
		}
	}
	store.lastSweep = now
}

func (store *furtherMemoryStore) take(subjectKey, leaseID string, rpm, concurrent int, now int64) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if now-store.lastSweep >= 60000 || len(store.subjects) >= furtherMemorySubjects || store.leases >= furtherMemoryLeases {
		store.prune(now)
	}
	subject := store.subjects[subjectKey]
	if subject == nil {
		if len(store.subjects) >= furtherMemorySubjects {
			return &FurtherLimitError{Reason: "capacity"}
		}
		subject = &furtherMemorySubject{minute: now / 60000, leases: make(map[string]int64)}
		store.subjects[subjectKey] = subject
	}
	for id, expiry := range subject.leases {
		if expiry <= now {
			delete(subject.leases, id)
			store.leases--
		}
	}
	if subject.minute != now/60000 {
		subject.minute, subject.count = now/60000, 0
	}
	if rpm > 0 && subject.count >= rpm {
		return &FurtherLimitError{Reason: "rpm", RetryAfter: int((60000 - now%60000 + 999) / 1000)}
	}
	if concurrent > 0 && len(subject.leases) >= concurrent {
		return &FurtherLimitError{Reason: "concurrency", RetryAfter: 1}
	}
	if concurrent > 0 && store.leases >= furtherMemoryLeases {
		return &FurtherLimitError{Reason: "capacity"}
	}
	if rpm > 0 {
		subject.count++
	}
	if concurrent > 0 {
		subject.leases[leaseID] = now + furtherLeaseTTL.Milliseconds()
		store.leases++
	}
	return nil
}

func (store *furtherMemoryStore) renew(subjectKey, leaseID string, now int64) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	subject := store.subjects[subjectKey]
	if subject == nil || subject.leases[leaseID] <= now {
		return errors.New("concurrency lease expired")
	}
	subject.leases[leaseID] = now + furtherLeaseTTL.Milliseconds()
	return nil
}

func (store *furtherMemoryStore) release(subjectKey, leaseID string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if subject := store.subjects[subjectKey]; subject != nil {
		if _, exists := subject.leases[leaseID]; exists {
			delete(subject.leases, leaseID)
			store.leases--
		}
		if len(subject.leases) == 0 && subject.count == 0 {
			delete(store.subjects, subjectKey)
		}
	}
}

// subject must contain internal numeric resource IDs, never an API key or a
// user-supplied secret. Redis failure never switches a multi-node deployment to
// independent local counters; the bounded local store is for Redis-disabled use.
func AcquireFurtherLimits(ctx context.Context, subject string, rpm, concurrent int) (*FurtherLimitLease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if subject == "" || len(subject) > 96 || rpm < 0 || rpm > 1000000 || concurrent < 0 || concurrent > 100000 {
		return nil, errors.New("invalid Further limit configuration")
	}
	if rpm == 0 && concurrent == 0 {
		return &FurtherLimitLease{ctx: ctx, cancel: func() {}, stop: make(chan struct{})}, nil
	}
	leaseContext, cancel := context.WithCancel(ctx)
	lease := &FurtherLimitLease{ctx: leaseContext, cancel: cancel, stop: make(chan struct{})}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		cancel()
		return nil, err
	}
	leaseID := hex.EncodeToString(random[:])
	if common.RedisEnabled {
		if common.RDB == nil {
			cancel()
			return nil, &FurtherLimitError{Reason: "unavailable"}
		}
		prefix := "{further:limits:" + subject + "}"
		ctx, stop := context.WithTimeout(ctx, 2*time.Second)
		values, err := common.RDB.Eval(ctx, furtherAcquireScript, []string{prefix + ":rpm", prefix + ":leases"}, rpm, concurrent, leaseID, furtherLeaseTTL.Milliseconds()).Slice()
		stop()
		if err != nil || len(values) != 3 {
			cancel()
			return nil, &FurtherLimitError{Reason: "unavailable", Cause: err}
		}
		allowed, valid := values[0].(int64)
		if !valid {
			cancel()
			return nil, &FurtherLimitError{Reason: "unavailable"}
		}
		if allowed != 1 {
			cancel()
			reason, _ := values[1].(int64)
			retry, _ := values[2].(int64)
			kind := "rpm"
			if reason == 2 {
				kind = "concurrency"
			}
			return nil, &FurtherLimitError{Reason: kind, RetryAfter: int(retry)}
		}
		if concurrent > 0 {
			client := common.RDB
			lease.release = func() {
				ctx, done := context.WithTimeout(context.Background(), 2*time.Second)
				defer done()
				if err := client.Eval(ctx, furtherReleaseScript, []string{prefix + ":leases"}, leaseID).Err(); err != nil {
					common.SysError("Further concurrency release deferred to lease expiry")
				}
			}
			lease.renew = func() error {
				ctx, done := context.WithTimeout(leaseContext, 2*time.Second)
				defer done()
				value, err := client.Eval(ctx, furtherRenewScript, []string{prefix + ":leases"}, leaseID, furtherLeaseTTL.Milliseconds()).Int64()
				if err != nil {
					return err
				}
				if value != 1 {
					return errors.New("concurrency lease lost")
				}
				return nil
			}
		}
	} else {
		store := &furtherLocalLimits
		if err := store.take(subject, leaseID, rpm, concurrent, time.Now().UnixMilli()); err != nil {
			cancel()
			return nil, err
		}
		if concurrent > 0 {
			lease.release = func() { store.release(subject, leaseID) }
			lease.renew = func() error { return store.renew(subject, leaseID, time.Now().UnixMilli()) }
		}
	}
	if concurrent > 0 {
		go lease.keepAlive()
	}
	return lease, nil
}

// Only the subscription actually selected by the native funding transaction
// supplies this policy. Native further-api subscriptions with no Further policy
// remain unrestricted by this additional limiter.
func AcquireFurtherSubscriptionLimits(ctx context.Context, userID, subscriptionID int) (*FurtherLimitLease, error) {
	if model.DB == nil || !model.DB.Migrator().HasTable(&model.FurtherSubscriptionPolicy{}) {
		return AcquireFurtherLimits(ctx, fmt.Sprintf("user:%d:subscription:%d", userID, subscriptionID), 0, 0)
	}
	entitlements, err := model.GetFurtherActiveEntitlements(userID, "")
	if err != nil {
		return nil, err
	}
	rpm, concurrent := 0, 0
	for _, entitlement := range entitlements {
		if entitlement.Subscription.Id == subscriptionID {
			rpm, concurrent = entitlement.Policy.RequestsPerMinute, entitlement.Policy.ConcurrentRequests
			break
		}
	}
	return AcquireFurtherLimits(ctx, fmt.Sprintf("user:%d:subscription:%d", userID, subscriptionID), rpm, concurrent)
}
