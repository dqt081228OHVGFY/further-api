package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RelayForUIPreferences is separate from New API's billing/security settings.
type RelayForUIPreferences struct {
	UserID             int    `json:"-" gorm:"primaryKey"`
	Configured         bool   `json:"configured"`
	HelpDocsVisible    bool   `json:"help_docs_visible"`
	Theme              string `json:"theme" gorm:"size:16"`
	Timezone           string `json:"timezone" gorm:"size:64"`
	DefaultPage        string `json:"default_page" gorm:"size:32"`
	PageSize           int    `json:"page_size"`
	ReducedMotion      bool   `json:"reduced_motion"`
	HideLockedModels   bool   `json:"hide_locked_models"`
	SidebarCollapsed   bool   `json:"sidebar_collapsed"`
	HourCycle          string `json:"hour_cycle" gorm:"size:2"`
	ShowSeconds        bool   `json:"show_seconds"`
	HideSuccessfulLogs bool   `json:"hide_successful_logs"`
}

func defaultRelayForPreferences(userID int) RelayForUIPreferences {
	return RelayForUIPreferences{UserID: userID, HelpDocsVisible: true, Theme: "light", Timezone: "Asia/Shanghai", DefaultPage: "overview", PageSize: 25, HourCycle: "24"}
}

func GetRelayForPreferences(userID int) (RelayForUIPreferences, error) {
	prefs := defaultRelayForPreferences(userID)
	err := DB.First(&prefs, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return prefs, nil
	}
	prefs.DefaultPage = RelayForActivityPageAlias(prefs.DefaultPage)
	return prefs, err
}

func PatchRelayForPreferences(userID int, updates map[string]any) (RelayForUIPreferences, error) {
	prefs := defaultRelayForPreferences(userID)
	prefs.Configured = true
	updates["configured"] = true
	// Insert defaults first, then apply the patch atomically. Explicit update
	// columns keep simultaneous patches from erasing unrelated preferences.
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&prefs).Error; err != nil {
			return err
		}
		return tx.Model(&RelayForUIPreferences{}).Where("user_id = ?", userID).Updates(updates).Error
	})
	if err != nil {
		return prefs, err
	}
	return GetRelayForPreferences(userID)
}

type RelayForLogQuery struct {
	UserID                        int
	Page, PageSize                int
	Keyword                       string
	StatusCode, ExcludeStatusCode int
	StartTimestamp, EndTimestamp  int64
	SortBy, SortOrder             string
}

type RelayForUsage struct {
	Requests         int64 `json:"requests"`
	Quota            int64 `json:"quota"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	CachedTokens     int64 `json:"cached_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	ActiveUsers      int64 `json:"active_users"`
	Errors           int64 `json:"errors"`
}

type RelayForLogItem struct {
	ID               int    `json:"id"`
	UserID           int    `json:"user_id"`
	CreatedAt        int64  `json:"created_at"`
	Username         string `json:"username"`
	TokenID          int    `json:"token_id"`
	TokenName        string `json:"token_name"`
	Model            string `json:"model"`
	Quota            int    `json:"quota"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	CachedTokens     int64  `json:"cached_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	DurationMS       int64  `json:"duration_ms"`
	StatusCode       int    `json:"status_code"`
	Failed           bool   `json:"failed"`
	StatusEstimated  bool   `json:"status_estimated,omitempty"`
	ErrorCode        string `json:"error_code,omitempty"`
	RequestID        string `json:"request_id"`
	ChannelID        int    `json:"channel_id,omitempty"`
	ChannelName      string `json:"channel_name,omitempty"`
	Stream           bool   `json:"stream"`
}

type RelayForLogPage struct {
	Items      []RelayForLogItem `json:"items"`
	Total      int64             `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"page_size"`
	TotalPages int64             `json:"total_pages"`
	Summary    RelayForUsage     `json:"summary"`
}

// Paths are compile-time literals. No user query text is interpolated in SQL.
func relayForJSONNumber(key string) string {
	switch common.LogDatabaseType() {
	case common.DatabaseTypePostgreSQL:
		return fmt.Sprintf("COALESCE((NULLIF(other, '')::jsonb ->> '%s')::bigint, 0)", key)
	case common.DatabaseTypeMySQL:
		return fmt.Sprintf("COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(IF(JSON_VALID(other), other, '{}'), '$.%s')) AS SIGNED), 0)", key)
	case common.DatabaseTypeClickHouse:
		return fmt.Sprintf("toInt64OrZero(JSONExtractRaw(other, '%s'))", key)
	default:
		return fmt.Sprintf("COALESCE(CAST(json_extract(CASE WHEN json_valid(other) THEN other ELSE '{}' END, '$.%s') AS INTEGER), 0)", key)
	}
}

func relayForStatusSQL() string {
	return "CASE WHEN type = 2 THEN 200 ELSE " + relayForJSONNumber("status_code") + " END"
}

func relayForLogsQuery(q RelayForLogQuery) *gorm.DB {
	tx := LOG_DB.Model(&Log{}).Where("type IN ?", []int{LogTypeConsume, LogTypeError})
	if q.UserID > 0 {
		tx = tx.Where("user_id = ?", q.UserID)
	}
	if q.StartTimestamp > 0 {
		tx = tx.Where("created_at >= ?", q.StartTimestamp)
	}
	if q.EndTimestamp > 0 {
		tx = tx.Where("created_at <= ?", q.EndTimestamp)
	}
	if q.Keyword != "" {
		// Escape SQL wildcard characters; keyword is always a bound parameter.
		keyword := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(q.Keyword) + "%"
		condition := "(model_name LIKE ? ESCAPE '!' OR token_name LIKE ? ESCAPE '!' OR request_id LIKE ? ESCAPE '!' OR username LIKE ? ESCAPE '!')"
		if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
			keyword = "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q.Keyword) + "%"
			condition = "(model_name LIKE ? OR token_name LIKE ? OR request_id LIKE ? OR username LIKE ?)"
		}
		tx = tx.Where(condition, keyword, keyword, keyword, keyword)
	}
	if q.StatusCode > 0 {
		tx = tx.Where("("+relayForStatusSQL()+") = ?", q.StatusCode)
	}
	if q.ExcludeStatusCode > 0 {
		tx = tx.Where("("+relayForStatusSQL()+") <> ?", q.ExcludeStatusCode)
	}
	return tx
}

func relayForUsageSelect() string {
	return "COUNT(*) AS requests, COALESCE(SUM(quota),0) AS quota, COALESCE(SUM(prompt_tokens),0) AS prompt_tokens, COALESCE(SUM(completion_tokens),0) AS completion_tokens, COALESCE(SUM(" + relayForJSONNumber("cache_tokens") + "),0) AS cached_tokens, COALESCE(SUM(" + relayForJSONNumber("cache_creation_tokens") + "),0) AS cache_write_tokens, COUNT(DISTINCT user_id) AS active_users, COALESCE(SUM(CASE WHEN type = 5 THEN 1 ELSE 0 END),0) AS errors"
}

func GetRelayForUsage(q RelayForLogQuery) (RelayForUsage, error) {
	var usage RelayForUsage
	err := relayForLogsQuery(q).Select(relayForUsageSelect()).Scan(&usage).Error
	return usage, err
}

func relayForLogItem(log Log, admin bool) RelayForLogItem {
	other := gjson.Parse(log.Other)
	status := 200
	if log.Type == LogTypeError {
		status = int(other.Get("status_code").Int())
		if status < 100 {
			status = 0
		}
	}
	item := RelayForLogItem{ID: log.Id, UserID: log.UserId, CreatedAt: log.CreatedAt, Username: log.Username,
		TokenID: log.TokenId, TokenName: log.TokenName, Model: log.ModelName, Quota: log.Quota,
		PromptTokens: log.PromptTokens, CompletionTokens: log.CompletionTokens,
		CachedTokens: max(0, other.Get("cache_tokens").Int()), CacheWriteTokens: max(0, other.Get("cache_creation_tokens").Int()),
		DurationMS: int64(log.UseTime) * 1000, StatusCode: status, Failed: log.Type == LogTypeError, StatusEstimated: log.Type == LogTypeError && status == 0,
		ErrorCode: other.Get("error_code").String(), RequestID: log.RequestId, Stream: log.IsStream}
	if admin {
		item.ChannelID, item.ChannelName = log.ChannelId, log.ChannelName
	}
	return item
}

func GetRelayForLogs(q RelayForLogQuery, admin bool) (RelayForLogPage, error) {
	page := RelayForLogPage{Items: []RelayForLogItem{}, Page: q.Page, PageSize: q.PageSize}
	usage, err := GetRelayForUsage(q)
	if err != nil {
		return page, err
	}
	page.Summary, page.Total = usage, usage.Requests
	page.TotalPages = (page.Total + int64(q.PageSize) - 1) / int64(q.PageSize)
	columns := map[string]string{"created_at": "created_at", "token_name": "token_name", "model": "model_name", "total_tokens": "(prompt_tokens + completion_tokens)", "quota": "quota", "duration_ms": "use_time", "status_code": relayForStatusSQL()}
	column := columns[q.SortBy]
	if column == "" {
		column = "created_at"
	}
	direction := " DESC"
	if q.SortOrder == "asc" {
		direction = " ASC"
	}
	var logs []Log
	err = relayForLogsQuery(q).Order(column + direction).Order("id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&logs).Error
	if err != nil {
		return page, err
	}
	channelNames := map[int]string{}
	if admin {
		ids := make([]int, 0, len(logs))
		for _, log := range logs {
			if log.ChannelId > 0 {
				ids = append(ids, log.ChannelId)
			}
		}
		if len(ids) > 0 {
			var channels []Channel
			if err := DB.Select("id, name").Where("id IN ?", ids).Find(&channels).Error; err != nil {
				return page, err
			}
			for _, channel := range channels {
				channelNames[channel.Id] = channel.Name
			}
		}
	}
	for _, log := range logs {
		log.ChannelName = channelNames[log.ChannelId]
		page.Items = append(page.Items, relayForLogItem(log, admin))
	}
	return page, nil
}

// ExportRelayForLogs keeps only one projected row in memory and honors
// cancellation if the owner stops the download.
func ExportRelayForLogs(ctx context.Context, q RelayForLogQuery, emit func(RelayForLogItem) error) error {
	rows, err := relayForLogsQuery(q).WithContext(ctx).Order("created_at DESC, id DESC").Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var log Log
		if err := LOG_DB.ScanRows(rows, &log); err != nil {
			return err
		}
		if err := emit(relayForLogItem(log, false)); err != nil {
			return err
		}
	}
	return rows.Err()
}

type RelayForTrend struct {
	DayIndex    int64   `json:"-"`
	Date        string  `json:"date"`
	Requests    int64   `json:"requests"`
	Quota       int64   `json:"quota"`
	Tokens      int64   `json:"tokens"`
	ActiveUsers int64   `json:"active_users"`
	Errors      int64   `json:"errors"`
	ErrorRate   float64 `json:"error_rate"`
}

var relayForStatsLocation = time.FixedZone("UTC+8", 8*3600)

func RelayForTodayStart(now time.Time) int64 {
	local := now.In(relayForStatsLocation)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, relayForStatsLocation).Unix()
}

func GetRelayForTrend(days int, now time.Time) ([]RelayForTrend, error) {
	start := RelayForTodayStart(now) - int64(days-1)*86400
	var rows []RelayForTrend
	err := relayForLogsQuery(RelayForLogQuery{StartTimestamp: start}).Select("FLOOR((created_at + 28800) / 86400) AS day_index, COUNT(*) AS requests, COALESCE(SUM(quota),0) AS quota, COALESCE(SUM(prompt_tokens + completion_tokens),0) AS tokens, COUNT(DISTINCT user_id) AS active_users, SUM(CASE WHEN type=5 THEN 1 ELSE 0 END) AS errors").Group("day_index").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	byDay := map[int64]RelayForTrend{}
	for _, row := range rows {
		byDay[row.DayIndex] = row
	}
	result := make([]RelayForTrend, 0, days)
	for day := 0; day < days; day++ {
		timestamp := start + int64(day)*86400
		point := byDay[(timestamp+28800)/86400]
		point.Date = time.Unix(timestamp, 0).In(relayForStatsLocation).Format("2006-01-02")
		if point.Requests > 0 {
			point.ErrorRate = float64(point.Errors) * 100 / float64(point.Requests)
		}
		result = append(result, point)
	}
	return result, nil
}

type RelayForRanking struct {
	UserID         int     `json:"user_id,omitempty"`
	Username       string  `json:"username,omitempty"`
	Model          string  `json:"model,omitempty"`
	ChannelID      int     `json:"channel_id,omitempty"`
	ChannelName    string  `json:"channel_name,omitempty"`
	TokenID        int     `json:"token_id,omitempty"`
	TokenName      string  `json:"token_name,omitempty"`
	Requests       int64   `json:"requests"`
	Quota          int64   `json:"quota"`
	Tokens         int64   `json:"tokens"`
	Errors         int64   `json:"errors"`
	RateLimited    int64   `json:"rate_limited"`
	ServerErrors   int64   `json:"server_errors"`
	ErrorRate      float64 `json:"error_rate"`
	FrequencyRatio float64 `json:"frequency_ratio,omitempty"`
}

func GetRelayForRankings(days int, now time.Time) (map[string]any, error) {
	q := RelayForLogQuery{StartTimestamp: RelayForTodayStart(now) - int64(days-1)*86400}
	result := map[string]any{"window_days": days}
	aggregates := "COUNT(*) AS requests, COALESCE(SUM(quota),0) AS quota, COALESCE(SUM(prompt_tokens+completion_tokens),0) AS tokens, SUM(CASE WHEN type=5 THEN 1 ELSE 0 END) AS errors"
	for _, spec := range []struct{ key, fields, group, order string }{
		{"top_users", "user_id, MAX(username) AS username", "user_id", "quota DESC"},
		{"top_models", "model_name AS model", "model_name", "requests DESC"},
		{"top_error_channels", "channel_id", "channel_id", "errors DESC"},
		{"high_frequency_keys", "token_id, MAX(token_name) AS token_name, MAX(user_id) AS user_id, MAX(username) AS username", "token_id", "requests DESC"},
	} {
		items := []RelayForRanking{}
		query := relayForLogsQuery(q).Select(spec.fields + ", " + aggregates).Group(spec.group).Order(spec.order).Limit(10)
		if spec.key == "top_error_channels" {
			query = query.Where("channel_id > 0").Having("SUM(CASE WHEN type=5 THEN 1 ELSE 0 END)>0").Select(spec.fields + ", " + aggregates + ", SUM(CASE WHEN (" + relayForStatusSQL() + ")=429 THEN 1 ELSE 0 END) AS rate_limited, SUM(CASE WHEN (" + relayForStatusSQL() + ")>=500 THEN 1 ELSE 0 END) AS server_errors")
		}
		if spec.key == "high_frequency_keys" {
			query = query.Where("token_id > 0").Having("COUNT(*) >= ?", days*100)
		}
		if err := query.Scan(&items).Error; err != nil {
			return nil, err
		}
		for index := range items {
			if items[index].Requests > 0 {
				items[index].ErrorRate = float64(items[index].Errors) * 100 / float64(items[index].Requests)
			}
		}
		result[spec.key] = items
	}
	return result, nil
}

type RelayForTrace struct {
	RequestID         string `json:"request_id"`
	Username          string `json:"username"`
	Model             string `json:"model"`
	StartedAt         int64  `json:"started_at"`
	DurationMS        int64  `json:"duration_ms"`
	Status            string `json:"status"`
	Path              string `json:"path"`
	Method            string `json:"method"`
	ChannelID         int    `json:"channel_id"`
	RetryCount        int    `json:"retry_count"`
	UpstreamRequestID string `json:"upstream_request_id,omitempty"`
	Source            string `json:"source"`
}

type RelayForTraceEvent struct {
	Stage      string         `json:"stage"`
	Message    string         `json:"message"`
	CreatedAt  int64          `json:"created_at"`
	DurationMS int64          `json:"duration_ms"`
	StatusCode int64          `json:"status_code,omitempty"`
	ChannelID  int64          `json:"channel_id,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

func GetRelayForTrace(requestID string) (RelayForTrace, []RelayForTraceEvent, error) {
	var logs []Log
	err := LOG_DB.Where("request_id = ? AND type IN ?", requestID, []int{LogTypeConsume, LogTypeError}).Order("created_at ASC, id ASC").Limit(500).Find(&logs).Error
	if err != nil {
		return RelayForTrace{}, nil, err
	}
	if len(logs) == 0 {
		return RelayForTrace{}, nil, gorm.ErrRecordNotFound
	}
	last := logs[len(logs)-1]
	item := relayForLogItem(last, true)
	other := gjson.Parse(last.Other)
	trace := RelayForTrace{RequestID: requestID, Username: last.Username, Model: last.ModelName, StartedAt: logs[0].CreatedAt - int64(logs[0].UseTime), DurationMS: item.DurationMS, Status: "succeeded", Path: other.Get("request_path").String(), ChannelID: last.ChannelId, UpstreamRequestID: last.UpstreamRequestId, Source: "persisted_logs"}
	if last.Type == LogTypeError {
		trace.Status = "failed"
		if item.StatusCode >= 400 && item.StatusCode < 500 {
			trace.Status = "rejected"
		}
	}
	// Method is absent in historical New API usage logs. Leave it empty
	// instead of inventing a transport or an in-flight state.
	events := []RelayForTraceEvent{}
	policy := other.Get("admin_info.request_policy").Array()
	if len(policy) > 0 {
		for _, event := range policy {
			elapsed := event.Get("elapsed_ms").Int()
			trace.RetryCount = max(trace.RetryCount, int(event.Get("attempt").Int())-1)
			events = append(events, RelayForTraceEvent{Stage: event.Get("decision.action").String(), Message: event.Get("decision.reason").String(), CreatedAt: trace.StartedAt + elapsed/1000, DurationMS: elapsed, StatusCode: event.Get("status").Int(), ChannelID: event.Get("channel_id").Int(), Metadata: map[string]any{"error_code": event.Get("error_code").String(), "source": event.Get("decision.source").String()}})
		}
	} else {
		for _, log := range logs {
			entry := relayForLogItem(log, true)
			stage := "success"
			if log.Type == LogTypeError {
				stage = "failure"
			}
			events = append(events, RelayForTraceEvent{Stage: stage, Message: entry.ErrorCode, CreatedAt: log.CreatedAt, DurationMS: entry.DurationMS, StatusCode: int64(entry.StatusCode), ChannelID: int64(log.ChannelId)})
		}
		trace.RetryCount = max(0, len(logs)-1)
	}
	return trace, events, nil
}

func GetRelayForTraces(q RelayForLogQuery, status string) (map[string]any, error) {
	base := relayForLogsQuery(q).Where("request_id <> ''").Select("request_id, MAX(created_at) AS latest").Group("request_id")
	success := "MAX(CASE WHEN type=2 THEN 1 ELSE 0 END)"
	if status == "succeeded" {
		base = base.Having(success + "=1")
	}
	if status == "failed" {
		base = base.Having(success + "=0 AND (MAX(" + relayForStatusSQL() + ")>=500 OR MIN(" + relayForStatusSQL() + ")=0)")
	}
	if status == "rejected" {
		base = base.Having(success + "=0 AND MAX(" + relayForStatusSQL() + ")<500 AND MIN(" + relayForStatusSQL() + ")>=400")
	}
	if status == "running" || status == "aborted" {
		base = base.Having("1=0")
	}
	var total int64
	if err := LOG_DB.Table("(?) AS trace_groups", base).Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []struct{ RequestID string }
	if err := base.Order("latest DESC, request_id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := []RelayForTrace{}
	for _, row := range rows {
		trace, _, err := GetRelayForTrace(row.RequestID)
		if err != nil {
			return nil, err
		}
		items = append(items, trace)
	}
	return map[string]any{"items": items, "total": total, "page": q.Page, "page_size": q.PageSize, "total_pages": (total + int64(q.PageSize) - 1) / int64(q.PageSize), "source": "persisted_logs"}, nil
}

func GetRelayForUserOverview(userID int, now time.Time) (map[string]any, error) {
	var user User
	if err := DB.First(&user, "id = ?", userID).Error; err != nil {
		return nil, err
	}
	today, err := GetRelayForUsage(RelayForLogQuery{UserID: userID, StartTimestamp: RelayForTodayStart(now)})
	if err != nil {
		return nil, err
	}
	var tokenCount int64
	if err := DB.Model(&Token{}).Where("user_id = ? AND status = ? AND (expired_time = -1 OR expired_time > ?) AND (unlimited_quota = ? OR remain_quota > 0)", userID, common.TokenStatusEnabled, now.Unix(), true).Count(&tokenCount).Error; err != nil {
		return nil, err
	}
	provider := "local"
	if user.LinuxDOId != "" {
		provider = "linuxdo"
	} else if user.GitHubId != "" {
		provider = "github"
	} else if user.OidcId != "" {
		provider = "oidc"
	} else if user.DiscordId != "" {
		provider = "discord"
	}
	subscriptions := []map[string]any{}
	timedQuota := int64(0)
	var subs []struct {
		UserSubscription
		PlanName string
	}
	if err := DB.Table("user_subscriptions AS s").Select("s.*, p.title AS plan_name").Joins("LEFT JOIN subscription_plans AS p ON p.id = s.plan_id").Where("s.user_id = ? AND s.status = ? AND s.end_time > ?", userID, "active", now.Unix()).Order("s.end_time DESC").Scan(&subs).Error; err != nil {
		return nil, err
	}
	for _, sub := range subs {
		isTimed := false
		for _, source := range relayForTimedSources {
			isTimed = isTimed || sub.Source == source
		}
		if isTimed {
			timedQuota += max(int64(0), sub.AmountTotal-sub.AmountUsed)
			continue
		}
		subscriptions = append(subscriptions, map[string]any{"id": sub.Id, "plan_id": sub.PlanId, "plan_name": sub.PlanName, "quota_remaining": max(int64(0), sub.AmountTotal-sub.AmountUsed), "unlimited_quota": sub.AmountTotal <= 0, "expires_at": sub.EndTime, "next_reset_at": sub.NextResetTime})
	}
	priority := "subscription_first"
	if DB.Migrator().HasTable(&RelayForSubscriptionPreference{}) {
		timedFirst, err := GetRelayForBillingPriority(userID)
		if err != nil {
			return nil, err
		}
		if timedFirst {
			priority = "timed_first"
		}
	}
	return map[string]any{"user": map[string]any{"id": user.Id, "username": user.Username, "display_name": user.DisplayName, "email": user.Email, "role": user.Role, "status": user.Status, "created_at": user.CreatedAt, "quota": user.Quota, "used_quota": user.UsedQuota, "group": user.Group, "billing_priority": priority}, "identity": map[string]any{"provider": provider, "password_change_available": user.Password != ""}, "requests": user.RequestCount, "token_count": tokenCount, "today": map[string]any{"requests": today.Requests, "input_tokens": today.PromptTokens, "output_tokens": today.CompletionTokens, "quota": today.Quota}, "active_subscriptions": subscriptions, "timed_quota": timedQuota, "timezone": "Asia/Shanghai"}, nil
}

func GetRelayForAdminOverview(days int, now time.Time) (map[string]any, error) {
	usage, err := GetRelayForUsage(RelayForLogQuery{})
	if err != nil {
		return nil, err
	}
	today, err := GetRelayForUsage(RelayForLogQuery{StartTimestamp: RelayForTodayStart(now)})
	if err != nil {
		return nil, err
	}
	recent, err := GetRelayForUsage(RelayForLogQuery{StartTimestamp: now.Unix() - 60, EndTimestamp: now.Unix()})
	if err != nil {
		return nil, err
	}
	trend, err := GetRelayForTrend(days, now)
	if err != nil {
		return nil, err
	}
	rankings, err := GetRelayForRankings(days, now)
	if err != nil {
		return nil, err
	}
	var users, enabledUsers, channels, tokens int64
	if err := DB.Model(&User{}).Count(&users).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&User{}).Where("status = ?", common.UserStatusEnabled).Count(&enabledUsers).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&Channel{}).Count(&channels).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&Token{}).Where("status = ? AND (expired_time = -1 OR expired_time > ?) AND (unlimited_quota = ? OR remain_quota > 0)", common.TokenStatusEnabled, now.Unix(), true).Count(&tokens).Error; err != nil {
		return nil, err
	}
	errorRate := float64(0)
	if today.Requests > 0 {
		errorRate = float64(today.Errors) * 100 / float64(today.Requests)
	}
	return map[string]any{"users": users, "enabled_users": enabledUsers, "channels": channels, "active_tokens": tokens, "logs": usage.Requests, "input_tokens": usage.PromptTokens, "output_tokens": usage.CompletionTokens, "quota": usage.Quota, "today": map[string]any{"requests": today.Requests, "quota": today.Quota, "tokens": today.PromptTokens + today.CompletionTokens, "active_users": today.ActiveUsers, "errors": today.Errors, "error_rate": errorRate}, "recent_minute": map[string]any{"requests": recent.Requests, "tokens": recent.PromptTokens + recent.CompletionTokens, "quota": recent.Quota, "active_users": recent.ActiveUsers, "errors": recent.Errors}, "trend": trend, "rankings": rankings, "timezone": "Asia/Shanghai", "source": "persisted_logs"}, nil
}
