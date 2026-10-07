package common

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func isolatePasswordKDF(t *testing.T) *passwordKDFLimiter {
	t.Helper()
	passwordKDFMu.Lock()
	old := passwordKDF
	passwordKDF = nil
	passwordKDFMu.Unlock()
	t.Setenv("ACCOUNT_PASSWORD_KDF_CONCURRENCY", "2")
	t.Setenv("ACCOUNT_PASSWORD_HASH_ALGORITHM", "argon2id")
	limiter := currentPasswordKDFLimiter()
	t.Cleanup(func() { passwordKDFMu.Lock(); passwordKDF = old; passwordKDFMu.Unlock() })
	return limiter
}

func TestPasswordKDFLimitsActualExpensiveCallsAndSeparatesBusyFromMismatch(t *testing.T) {
	limiter := isolatePasswordKDF(t)
	argonHash, err := HashAccountPassword("valid-password")
	require.NoError(t, err)
	bcryptHash, err := Password2Hash("valid-password")
	require.NoError(t, err)
	original := deriveAccountPasswordKey
	var active, maximum, executions atomic.Int32
	entered := make(chan struct{}, 2)
	releaseWork := make(chan struct{})
	releaseCrypto := sync.OnceFunc(func() { close(releaseWork) })
	defer releaseCrypto()
	deriveAccountPasswordKey = func(password, salt []byte, rounds, memory uint32, threads uint8, size uint32) []byte {
		current := active.Add(1)
		defer active.Add(-1)
		for previous := maximum.Load(); current > previous && !maximum.CompareAndSwap(previous, current); previous = maximum.Load() {
		}
		if executions.Add(1) <= 2 {
			entered <- struct{}{}
			<-releaseWork
		}
		return original(password, salt, rounds, memory, threads, size)
	}
	t.Cleanup(func() { deriveAccountPasswordKey = original })
	type result struct {
		hash string
		err  error
	}
	results := make(chan result, 6)
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	for _, ctx := range []context.Context{firstCtx, context.Background()} {
		go func(ctx context.Context) {
			hash, err := HashAccountPasswordContext(ctx, "valid-password")
			results <- result{hash, err}
		}(ctx)
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("the two actual derivations did not start")
		}
	}
	// A canceled waiter frees its queue reservation without entering Argon2.
	waitCtx, cancelWait := context.WithCancel(context.Background())
	canceled := make(chan error, 1)
	go func() { _, err := HashAccountPasswordContext(waitCtx, "valid-password"); canceled <- err }()
	require.Eventually(t, func() bool { return len(limiter.queue) == 1 }, time.Second, time.Millisecond)
	cancelWait()
	require.ErrorIs(t, <-canceled, context.Canceled)
	require.EqualValues(t, 2, executions.Load())
	for range 4 {
		go func() {
			hash, err := HashAccountPasswordContext(context.Background(), "valid-password")
			results <- result{hash, err}
		}()
	}
	require.Eventually(t, func() bool { return len(limiter.queue) == 4 }, time.Second, time.Millisecond)
	for _, encoded := range []string{argonHash, bcryptHash} {
		valid, err := ValidatePasswordAndHashContext(context.Background(), "valid-password", encoded)
		require.False(t, valid)
		require.ErrorIs(t, err, ErrPasswordKDFBusy, "capacity must never be reported as a password mismatch")
	}
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ApiError(ginContext, ErrPasswordKDFBusy)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), "AUTH_BUSY")
	require.Equal(t, "1", recorder.Header().Get("Retry-After"))
	cancelFirst()
	releaseCrypto()
	cancellations := 0
	for range 6 {
		result := <-results
		if errors.Is(result.err, context.Canceled) {
			cancellations++
			require.Empty(t, result.hash)
		} else {
			require.NoError(t, result.err)
			require.NotEmpty(t, result.hash)
		}
	}
	require.Equal(t, 1, cancellations)
	require.EqualValues(t, 2, maximum.Load())
	require.EqualValues(t, 6, executions.Load(), "canceled/busy waiters must not run the KDF")
	require.Empty(t, limiter.slots)
	require.Empty(t, limiter.queue)
	valid, err := ValidatePasswordAndHashContext(context.Background(), "valid-password", argonHash)
	require.NoError(t, err)
	require.True(t, valid)
}

func TestPasswordKDFBoundedWaitAndLegacyReadThenArgonWriteDoNotDeadlock(t *testing.T) {
	limiter := isolatePasswordKDF(t)
	legacy, err := Password2HashContext(context.Background(), "migration-password")
	require.NoError(t, err)
	valid, err := ValidatePasswordAndHashContext(context.Background(), "migration-password", legacy)
	require.NoError(t, err)
	require.True(t, valid)
	upgraded, err := HashAccountPasswordContext(context.Background(), "migration-password")
	require.NoError(t, err)
	valid, err = ValidatePasswordAndHashContext(context.Background(), "migration-password", upgraded)
	require.NoError(t, err)
	require.True(t, valid)
	first, err := acquirePasswordKDF(context.Background())
	require.NoError(t, err)
	defer first()
	second, err := acquirePasswordKDF(context.Background())
	require.NoError(t, err)
	defer second()
	limiter.wait = 20 * time.Millisecond
	_, err = HashAccountPasswordContext(context.Background(), "migration-password")
	require.ErrorIs(t, err, ErrPasswordKDFBusy)
	require.Empty(t, limiter.queue)
	for _, test := range []struct {
		value string
		want  int
	}{{"", 2}, {"invalid", 2}, {"0", 2}, {"1", 1}, {"4", 4}, {"5", 2}} {
		require.Equal(t, test.want, configuredPasswordKDFConcurrency(test.value))
	}
}
