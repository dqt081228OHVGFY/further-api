package common

import (
	"context"
	"errors"
	"os"
	"strconv"
	"sync"
	"time"
)

var ErrPasswordKDFBusy = errors.New("身份验证繁忙，请稍后重试。")

type passwordKDFLimiter struct {
	slots chan struct{}
	queue chan struct{}
	wait  time.Duration
}

var passwordKDFMu sync.Mutex
var passwordKDF *passwordKDFLimiter

func configuredPasswordKDFConcurrency(raw string) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 4 {
		return 2
	}
	return value
}

func currentPasswordKDFLimiter() *passwordKDFLimiter {
	passwordKDFMu.Lock()
	defer passwordKDFMu.Unlock()
	if passwordKDF == nil {
		concurrency := configuredPasswordKDFConcurrency(os.Getenv("ACCOUNT_PASSWORD_KDF_CONCURRENCY"))
		passwordKDF = &passwordKDFLimiter{slots: make(chan struct{}, concurrency), queue: make(chan struct{}, concurrency*2), wait: 2 * time.Second}
	}
	return passwordKDF
}

func acquirePasswordKDF(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limiter := currentPasswordKDFLimiter()
	release := func() { <-limiter.slots }
	select {
	case limiter.slots <- struct{}{}:
		if err := ctx.Err(); err != nil {
			release()
			return nil, err
		}
		return release, nil
	default:
	}
	select {
	case limiter.queue <- struct{}{}:
		defer func() { <-limiter.queue }()
	default:
		return nil, ErrPasswordKDFBusy
	}
	timer := time.NewTimer(limiter.wait)
	defer timer.Stop()
	select {
	case limiter.slots <- struct{}{}:
		if err := ctx.Err(); err != nil {
			release()
			return nil, err
		}
		return release, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, ErrPasswordKDFBusy
	}
}
