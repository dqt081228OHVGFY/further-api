package model

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRelayForStatsTest(t *testing.T) {
	t.Helper()
	previousDB, previousLog := DB, LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "stats.db")), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLog
		common.SetDatabaseTypes(previousMainType, previousLogType)
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &Channel{}, &Log{}, &UserSubscription{}, &SubscriptionPlan{}, &RelayForUIPreferences{}))
}

func TestRelayForLogFiltersAggregateAllMatchesAndProtectUserMetadata(t *testing.T) {
	setupRelayForStatsTest(t)
	logs := []Log{
		{Id: 1, UserId: 1, CreatedAt: 1000, Type: LogTypeConsume, ModelName: "m1", TokenName: "first", Quota: 20, PromptTokens: 10, CompletionTokens: 5, ChannelId: 9, RequestId: "r1", Other: `{"cache_tokens":3,"cache_creation_tokens":4,"admin_info":{"private":"admin-secret"},"root_info":{"key":"private-key"}}`},
		{Id: 2, UserId: 1, CreatedAt: 1001, Type: LogTypeError, ModelName: "m2", RequestId: "r2", Other: `{"status_code":429,"error_code":"rate_limited"}`},
		{Id: 3, UserId: 2, CreatedAt: 1002, Type: LogTypeConsume, Quota: 5000, PromptTokens: 900, RequestId: "other-user"},
		{Id: 4, UserId: 1, CreatedAt: 1003, Type: LogTypeConsume, ModelName: "m3", Quota: 30, PromptTokens: 20, CompletionTokens: 10, RequestId: "r4", Other: `{"cache_tokens":4,"cache_creation_tokens":2}`},
	}
	require.NoError(t, DB.Create(&logs).Error)
	q := RelayForLogQuery{UserID: 1, Page: 1, PageSize: 1, SortBy: "quota", SortOrder: "desc", StatusCode: 200}
	page, err := GetRelayForLogs(q, false)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, 4, page.Items[0].ID)
	assert.EqualValues(t, 2, page.Total)
	assert.EqualValues(t, 2, page.TotalPages)
	assert.EqualValues(t, 50, page.Summary.Quota)
	assert.EqualValues(t, 30, page.Summary.PromptTokens)
	assert.EqualValues(t, 7, page.Summary.CachedTokens)
	assert.EqualValues(t, 6, page.Summary.CacheWriteTokens)
	q.StatusCode, q.ExcludeStatusCode = 0, 200
	page, err = GetRelayForLogs(q, false)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, 429, page.Items[0].StatusCode)
	assert.EqualValues(t, 1, page.Summary.Errors)
	q.StatusCode, q.ExcludeStatusCode, q.PageSize = 0, 0, 100
	page, err = GetRelayForLogs(q, false)
	require.NoError(t, err)
	encoded, err := json.Marshal(page)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private-key")
	assert.NotContains(t, string(encoded), "admin-secret")
	assert.NotContains(t, string(encoded), "channel_id")
	assert.NotContains(t, string(encoded), "other-user")
}

func TestRelayForAdminMinuteWindowUsesOnlyRecentPersistedLogs(t *testing.T) {
	setupRelayForStatsTest(t)
	now := time.Date(2026, 10, 1, 6, 0, 0, 0, time.UTC)
	logs := []Log{
		{UserId: 1, CreatedAt: now.Unix() - 30, Type: LogTypeConsume, Quota: 20, PromptTokens: 10, CompletionTokens: 5, RequestId: "recent"},
		{UserId: 2, CreatedAt: now.Unix() - 20, Type: LogTypeError, RequestId: "recent-error"},
		{UserId: 1, CreatedAt: now.Unix() - 61, Type: LogTypeConsume, Quota: 1000, PromptTokens: 999, RequestId: "old"},
		{UserId: 1, CreatedAt: now.Unix() + 1, Type: LogTypeConsume, Quota: 2000, PromptTokens: 999, RequestId: "future"},
	}
	require.NoError(t, DB.Create(&logs).Error)
	result, err := GetRelayForAdminOverview(7, now)
	require.NoError(t, err)
	minute := result["recent_minute"].(map[string]any)
	assert.EqualValues(t, 2, minute["requests"])
	assert.EqualValues(t, 15, minute["tokens"])
	assert.EqualValues(t, 20, minute["quota"])
	assert.EqualValues(t, 1, minute["errors"])
}

func TestRelayForHealthUsesAllLogsAndFoldsRetries(t *testing.T) {
	setupRelayForStatsTest(t)
	require.NoError(t, DB.AutoMigrate(&Model{}))
	require.NoError(t, DB.Create(&Channel{Id: 9, Name: "upstream", Models: "m1", Status: common.ChannelStatusEnabled}).Error)
	now := time.Date(2026, 9, 30, 4, 0, 0, 0, time.UTC)
	logs := []Log{
		{Id: 1, CreatedAt: now.Unix() - 2, Type: LogTypeError, ModelName: "m1", ChannelId: 9, RequestId: "retried", Quota: 900, PromptTokens: 900},
		{Id: 2, CreatedAt: now.Unix() - 1, Type: LogTypeConsume, ModelName: "m1", ChannelId: 9, RequestId: "retried", Quota: 40, PromptTokens: 100, CompletionTokens: 180, IsStream: true, UseTime: 2, Other: `{"cache_tokens":25,"frt":200,"root_info":{"secret":"private"}}`},
		{Id: 3, CreatedAt: now.Unix() - 1, Type: LogTypeError, ModelName: "m1", ChannelId: 9, RequestId: "failed"},
		{Id: 4, CreatedAt: now.Unix() - 86400, Type: LogTypeConsume, ModelName: "m1", ChannelId: 9, RequestId: "yesterday", Quota: 50000},
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)
	result, err := GetRelayForModelHealth(now)
	require.NoError(t, err)
	summary := result["summary"].(map[string]any)
	assert.Equal(t, RelayForHealthWindow{Requests: 2, Successful: 1, SuccessRate: 50}, summary["today"])
	assert.EqualValues(t, 40, summary["charge_quota"])
	entries := result["models"].([]*RelayForHealthEntry)
	require.Len(t, entries, 1)
	assert.EqualValues(t, 3, entries[0].Recent60.Requests)
	assert.EqualValues(t, 2, entries[0].Recent60.Successful)
	assert.EqualValues(t, 25, entries[0].CacheHitRate)
	channels := result["channels"].([]*RelayForHealthEntry)
	require.Len(t, channels, 1)
	assert.EqualValues(t, 50, channels[0].Today.SuccessRate)
	assert.EqualValues(t, 200, channels[0].AverageTTFTMS)
	assert.InDelta(t, 100, channels[0].AverageTokensPerSecond, 0.01)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private")
	assert.NotContains(t, string(encoded), "root_info")
	recent := make([]Log, 61)
	for index := range recent {
		kind := LogTypeConsume
		if index == 60 {
			kind = LogTypeError
		}
		recent[index] = Log{Id: 100 + index, CreatedAt: now.Unix() - int64(61-index), Type: kind, ModelName: "boundary", RequestId: "boundary-" + strconv.Itoa(index), Quota: 1}
	}
	require.NoError(t, LOG_DB.Create(&recent).Error)
	capped, err := GetRelayForModelHealth(now)
	require.NoError(t, err)
	for _, entry := range capped["models"].([]*RelayForHealthEntry) {
		if entry.Model != "boundary" {
			continue
		}
		assert.EqualValues(t, 61, entry.Today.Requests)
		assert.EqualValues(t, 60, entry.Recent60.Requests)
		assert.EqualValues(t, 59, entry.Recent60.Successful)
	}
}

func TestRelayForPublicModelHealthOnlyIncludesAllowedModelsAndPublicFields(t *testing.T) {
	window := RelayForHealthWindow{Requests: 3, Successful: 2, SuccessRate: 200.0 / 3}
	data := map[string]any{
		"generated_at": "2026-10-01T01:00:00Z", "timezone": "Asia/Shanghai",
		"models": []*RelayForHealthEntry{
			{Model: "visible", DisplayName: "Visible model", Recent60: window, Today: window, TodayChargeQuota: 987, CachedTokens: 321},
			{Model: "private", Today: window},
		},
		"channels": []*RelayForHealthEntry{{ChannelID: 99, Name: "private upstream"}},
		"summary":  map[string]any{"charge_quota": 987},
	}
	public := RelayForPublicModelHealth(data, map[string]bool{"visible": true})
	items := public["items"].([]RelayForPublicModelHealthEntry)
	require.Len(t, items, 1)
	assert.Equal(t, "visible", items[0].Model)
	assert.Equal(t, window, items[0].Today)
	encoded, err := json.Marshal(public)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private")
	assert.NotContains(t, string(encoded), "channel")
	assert.NotContains(t, string(encoded), "charge")
	assert.NotContains(t, string(encoded), "cached_tokens")
	assert.NotContains(t, string(encoded), "summary")
	empty := RelayForPublicModelHealth(data, nil)
	encoded, err = json.Marshal(empty)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"items":[]`)
}

func TestRelayForPreferencesPersistPartialUpdatesWithoutChangingUserSettings(t *testing.T) {
	setupRelayForStatsTest(t)
	user := User{Id: 1, Username: "tester", Password: "password-hash", Setting: `{"record_ip_log":true}`}
	require.NoError(t, DB.Create(&user).Error)
	prefs, err := GetRelayForPreferences(1)
	require.NoError(t, err)
	assert.False(t, prefs.Configured)
	assert.Equal(t, "light", prefs.Theme)
	prefs, err = PatchRelayForPreferences(1, map[string]any{"theme": "dark", "reduced_motion": true})
	require.NoError(t, err)
	assert.True(t, prefs.Configured)
	prefs, err = PatchRelayForPreferences(1, map[string]any{"page_size": 50, "reduced_motion": false})
	require.NoError(t, err)
	assert.Equal(t, "dark", prefs.Theme)
	assert.Equal(t, 50, prefs.PageSize)
	assert.False(t, prefs.ReducedMotion)
	other, err := GetRelayForPreferences(2)
	require.NoError(t, err)
	assert.False(t, other.Configured)
	var stored User
	require.NoError(t, DB.First(&stored, 1).Error)
	assert.Equal(t, user.Setting, stored.Setting)
}

func TestRelayForOverviewUsesUTC8DailyBoundaryAndRealDatabaseRows(t *testing.T) {
	setupRelayForStatsTest(t)
	now := time.Date(2026, 9, 24, 2, 0, 0, 0, time.UTC)
	start := RelayForTodayStart(now)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "alice", Password: "secret-hash", Quota: 1234, UsedQuota: 45, RequestCount: 12}).Error)
	require.NoError(t, DB.Create(&Token{UserId: 1, Key: "secret-api-key", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 100}).Error)
	require.NoError(t, DB.Create(&[]Log{{UserId: 1, CreatedAt: start - 1, Type: LogTypeConsume, Quota: 99, PromptTokens: 100}, {UserId: 1, CreatedAt: start, Type: LogTypeConsume, Quota: 7, PromptTokens: 3, CompletionTokens: 2}, {UserId: 1, CreatedAt: start + 1, Type: LogTypeError, Other: `{"status_code":503}`}}).Error)
	data, err := GetRelayForUserOverview(1, now)
	require.NoError(t, err)
	today := data["today"].(map[string]any)
	assert.EqualValues(t, 2, today["requests"])
	assert.EqualValues(t, 7, today["quota"])
	assert.EqualValues(t, 1, data["token_count"])
	encoded, _ := json.Marshal(data)
	assert.NotContains(t, string(encoded), "secret-hash")
	assert.NotContains(t, string(encoded), "secret-api-key")
	admin, err := GetRelayForAdminOverview(7, now)
	require.NoError(t, err)
	assert.EqualValues(t, 3, admin["logs"])
	trend := admin["trend"].([]RelayForTrend)
	require.Len(t, trend, 7)
	assert.Equal(t, "2026-09-24", trend[6].Date)
	assert.EqualValues(t, 50, trend[6].ErrorRate)
}

func TestRelayForPersistedTraceTimelineDoesNotExposePrivateScopes(t *testing.T) {
	setupRelayForStatsTest(t)
	logs := []Log{
		{UserId: 1, CreatedAt: 1000, Type: LogTypeError, RequestId: "trace1", Other: `{"status_code":429}`},
		{UserId: 1, CreatedAt: 1001, Type: LogTypeConsume, RequestId: "trace1", Other: `{"request_path":"/v1/chat/completions","admin_info":{"request_policy":[{"attempt":2,"elapsed_ms":123,"channel_id":3,"decision":{"action":"success","reason":"request_completed"}}],"secret":"admin-secret"},"root_info":{"payload":"private-body"}}`},
		{UserId: 1, CreatedAt: 1002, Type: LogTypeError, RequestId: "trace2", Other: `{"status_code":503}`},
	}
	require.NoError(t, DB.Create(&logs).Error)
	trace, events, err := GetRelayForTrace("trace1")
	require.NoError(t, err)
	assert.Equal(t, "succeeded", trace.Status)
	assert.Equal(t, 1, trace.RetryCount)
	require.Len(t, events, 1)
	assert.EqualValues(t, 3, events[0].ChannelID)
	encoded, _ := json.Marshal(events)
	assert.NotContains(t, string(encoded), "private-body")
	assert.NotContains(t, string(encoded), "admin-secret")
	page, err := GetRelayForTraces(RelayForLogQuery{Page: 1, PageSize: 25}, "succeeded")
	require.NoError(t, err)
	assert.EqualValues(t, 1, page["total"])
	items := page["items"].([]RelayForTrace)
	require.Len(t, items, 1)
	assert.Equal(t, "trace1", items[0].RequestID)
	page, err = GetRelayForTraces(RelayForLogQuery{Page: 1, PageSize: 25}, "running")
	require.NoError(t, err)
	assert.EqualValues(t, 0, page["total"])
}

func TestRelayForLegacyErrorDoesNotInventHTTPStatus(t *testing.T) {
	setupRelayForStatsTest(t)
	require.NoError(t, DB.Create(&Log{UserId: 1, Type: LogTypeError, RequestId: "legacy", Other: "legacy non-json metadata"}).Error)
	q := RelayForLogQuery{UserID: 1, Page: 1, PageSize: 25}
	page, err := GetRelayForLogs(q, false)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.True(t, page.Items[0].Failed)
	assert.True(t, page.Items[0].StatusEstimated)
	assert.Zero(t, page.Items[0].StatusCode)
	q.StatusCode = 500
	page, err = GetRelayForLogs(q, false)
	require.NoError(t, err)
	assert.Zero(t, page.Total)
}
