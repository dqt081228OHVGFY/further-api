package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func TestRedisSuccessCapacityIsAtomicAcrossConcurrentRequests(t *testing.T) {
	_, client := useRateLimitMiniRedis(t)
	var entered atomic.Int64
	var limited atomic.Int64
	ready := make(chan struct{}, 40)
	release := make(chan struct{})
	engine := gin.New()
	engine.POST("/", func(c *gin.Context) { c.Set("id", 94001) }, redisRateLimitHandler(60, 0, 3), func(c *gin.Context) {
		entered.Add(1)
		ready <- struct{}{}
		<-release
		c.Status(http.StatusOK)
	})
	var callers sync.WaitGroup
	for range 40 {
		callers.Add(1)
		go func() {
			defer callers.Done()
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", nil))
			if response.Code == http.StatusTooManyRequests {
				limited.Add(1)
				ready <- struct{}{}
			}
		}()
	}
	for range 40 {
		select {
		case <-ready:
		case <-time.After(3 * time.Second):
			close(release)
			callers.Wait()
			t.Fatal("admission did not finish")
		}
	}
	close(release)
	callers.Wait()
	require.EqualValues(t, 3, entered.Load())
	require.EqualValues(t, 37, limited.Load())
	require.EqualValues(t, 3, client.ZCard(context.Background(), "rateLimit:v2:model:{94001}:success").Val())
	require.Zero(t, client.ZCard(context.Background(), "rateLimit:v2:model:{94001}:active").Val())
}

func TestRedisSuccessReservationReleasesFailuresCancellationAndPanic(t *testing.T) {
	for _, outcome := range []string{"failed", "canceled", "panic"} {
		t.Run(outcome, func(t *testing.T) {
			_, client := useRateLimitMiniRedis(t)
			engine := gin.New()
			engine.Use(gin.Recovery())
			engine.POST("/:outcome", func(c *gin.Context) { c.Set("id", 94002) }, redisRateLimitHandler(60, 0, 1), func(c *gin.Context) {
				switch c.Param("outcome") {
				case "failed":
					c.Status(http.StatusBadGateway)
				case "canceled":
					<-c.Request.Context().Done()
				case "panic":
					panic("isolated recovery fixture")
				default:
					c.Status(http.StatusOK)
				}
			})
			ctx, cancel := context.WithCancel(context.Background())
			request := httptest.NewRequest(http.MethodPost, "/"+outcome, nil).WithContext(ctx)
			if outcome == "canceled" {
				time.AfterFunc(30*time.Millisecond, cancel)
			}
			first := httptest.NewRecorder()
			engine.ServeHTTP(first, request)
			cancel()
			for _, status := range []int{http.StatusOK, http.StatusTooManyRequests} {
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/success", nil))
				require.Equal(t, status, response.Code)
			}
			require.EqualValues(t, 1, client.ZCard(context.Background(), "rateLimit:v2:model:{94002}:success").Val())
			require.Zero(t, client.ZCard(context.Background(), "rateLimit:v2:model:{94002}:active").Val())
		})
	}
}

func TestRedisSuccessLeaseRenewsThroughoutLongRequestAndCancelsOnLoss(t *testing.T) {
	_, client := useRateLimitMiniRedis(t)
	previousLease := redisModelSuccessLeaseDuration
	redisModelSuccessLeaseDuration = 90 * time.Millisecond
	t.Cleanup(func() { redisModelSuccessLeaseDuration = previousLease })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reservation, allowed, err := reserveRedisModelSuccess(ctx, client, "94003", 1, 60)
	require.NoError(t, err)
	require.True(t, allowed)
	reservation.startRenewal(ctx, cancel)
	defer reservation.complete(false)
	// Live work extends beyond multiple nominal leases without releasing capacity.
	timer := time.NewTimer(300 * time.Millisecond)
	<-timer.C
	blocked, allowed, err := reserveRedisModelSuccess(context.Background(), client, "94003", 1, 60)
	require.NoError(t, err)
	require.False(t, allowed)
	require.Nil(t, blocked)
	require.NoError(t, ctx.Err())
	require.NoError(t, client.Del(context.Background(), reservation.keys[1]).Err())
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("lost distributed capacity did not terminate active request")
	}
	require.False(t, reservation.healthy.Load())
}

func TestRedisSuccessWindowExpiryAndOrphanCleanup(t *testing.T) {
	_, client := useRateLimitMiniRedis(t)
	now := time.Now().UnixMilli()
	keys := []string{"rateLimit:v2:model:{94004}:success", "rateLimit:v2:model:{94004}:active"}
	require.NoError(t, client.ZAdd(context.Background(), keys[0], &redis.Z{Score: float64(now - 61000), Member: "past-success"}).Err())
	require.NoError(t, client.ZAdd(context.Background(), keys[1], &redis.Z{Score: float64(now - 1), Member: "orphan"}).Err())
	reservation, admitted, err := reserveRedisModelSuccess(context.Background(), client, "94004", 1, 60)
	require.NoError(t, err)
	require.True(t, admitted)
	reservation.complete(true)
	_, admitted, err = reserveRedisModelSuccess(context.Background(), client, "94004", 1, 60)
	require.NoError(t, err)
	require.False(t, admitted)
}
