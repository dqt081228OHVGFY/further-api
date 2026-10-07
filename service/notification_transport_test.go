package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
	"github.com/stretchr/testify/require"
)

func notificationHTTPFixture(t *testing.T) {
	t.Helper()
	InitHttpClient()
	priorWorker := system_setting.WorkerUrl
	priorFetch := *system_setting.GetFetchSetting()
	system_setting.WorkerUrl = ""
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	t.Cleanup(func() { system_setting.WorkerUrl = priorWorker; *system_setting.GetFetchSetting() = priorFetch })
}

func TestNotificationLimitAdmitsExactlyConfiguredConcurrentCount(t *testing.T) {
	priorRedis, priorLimit := common.RedisEnabled, constant.NotifyLimitCount
	common.RedisEnabled, constant.NotifyLimitCount = false, 7
	t.Cleanup(func() { common.RedisEnabled, constant.NotifyLimitCount = priorRedis, priorLimit })
	key := notificationLimitKey(90123, "atomic-notify-test")
	notificationWindows.Lock()
	delete(notificationWindows.items, key)
	notificationWindows.Unlock()
	var allowed atomic.Int64
	var callers sync.WaitGroup
	for range 100 {
		callers.Add(1)
		go func() {
			defer callers.Done()
			ok, err := CheckNotificationLimitContext(context.Background(), 90123, "atomic-notify-test")
			if err == nil && ok {
				allowed.Add(1)
			}
		}()
	}
	callers.Wait()
	require.EqualValues(t, 7, allowed.Load())
}

func TestWebhookRendersAllValuesOnceAndSignsFinalPayload(t *testing.T) {
	notificationHTTPFixture(t)
	var received WebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &received))
		require.Equal(t, generateSignature("secret", raw), r.Header.Get("X-Webhook-Signature"))
		require.Equal(t, "further-api/1.0", r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	err := SendWebhookNotify(server.URL, "secret", dto.NewNotify("test", "Title", "{{value}} has {{value}} quota", []any{"owner", 50}))
	require.NoError(t, err)
	require.Equal(t, "owner has 50 quota", received.Content)
	require.NotContains(t, received.Content, "%!")
}

func TestCanceledNotificationNeverContactsDestination(t *testing.T) {
	notificationHTTPFixture(t)
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1) }))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, SendWebhookNotifyContext(ctx, server.URL, "", dto.NewNotify("test", "Title", "body", nil)), context.Canceled)
	require.Zero(t, calls.Load())
}

func TestGotifyPreservesConfiguredQueryAndNotificationPriority(t *testing.T) {
	notificationHTTPFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/custom/message", r.URL.Path)
		require.Equal(t, "tenant", r.URL.Query().Get("scope"))
		require.Equal(t, "token with space", r.URL.Query().Get("token"))
		body, _ := io.ReadAll(r.Body)
		require.True(t, strings.Contains(string(body), `"priority":5`))
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	require.NoError(t, sendGotifyNotify(server.URL+"/custom?scope=tenant", "token with space", 99, dto.NewNotify("test", "Title", "body", nil)))
}
