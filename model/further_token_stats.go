package model

import "gorm.io/gorm"

// FurtherTokenUsage groups recorded calls by owner and token ID. Keys and
// request bodies are intentionally absent from this administrative report.
type FurtherTokenUsage struct {
	UserID           int    `json:"user_id"`
	Username         string `json:"username"`
	TokenID          int    `json:"token_id"`
	TokenName        string `json:"token_name"`
	Requests         int64  `json:"requests"`
	Errors           int64  `json:"errors"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	Quota            int64  `json:"quota"`
	LastUsedAt       int64  `json:"last_used_at"`
}

type FurtherTokenUsagePage struct {
	Items      []FurtherTokenUsage `json:"items"`
	Total      int64               `json:"total"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"page_size"`
	TotalPages int64               `json:"total_pages"`
	Summary    FurtherUsage        `json:"summary"`
}

func GetFurtherTokenUsage(q FurtherLogQuery) (FurtherTokenUsagePage, error) {
	page := FurtherTokenUsagePage{Items: []FurtherTokenUsage{}, Page: q.Page, PageSize: q.PageSize}
	query := func() *gorm.DB { return furtherLogsQuery(q) }
	// Error attempts count as calls, but only successful consumption logs
	// contribute to billed quota and token usage.
	usageSelect := "COUNT(*) AS requests, COUNT(DISTINCT user_id) AS active_users, " +
		"COALESCE(SUM(CASE WHEN type = 5 THEN 1 ELSE 0 END),0) AS errors, " +
		"COALESCE(SUM(CASE WHEN type = 2 THEN quota ELSE 0 END),0) AS quota, " +
		"COALESCE(SUM(CASE WHEN type = 2 THEN prompt_tokens ELSE 0 END),0) AS prompt_tokens, " +
		"COALESCE(SUM(CASE WHEN type = 2 THEN completion_tokens ELSE 0 END),0) AS completion_tokens"
	if err := query().Select(usageSelect).Scan(&page.Summary).Error; err != nil {
		return page, err
	}
	grouped := query().Select("user_id, token_id").Group("user_id, token_id")
	if err := LOG_DB.Table("(?) AS token_usage_groups", grouped).Count(&page.Total).Error; err != nil {
		return page, err
	}
	page.TotalPages = (page.Total + int64(q.PageSize) - 1) / int64(q.PageSize)
	err := query().Select("user_id, token_id, MAX(username) AS username, MAX(token_name) AS token_name, MAX(created_at) AS last_used_at, " + usageSelect).
		Group("user_id, token_id").Order("quota DESC, requests DESC, user_id ASC, token_id ASC").
		Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Scan(&page.Items).Error
	return page, err
}
