package controller

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func relayForQuery(c *gin.Context, admin bool) (model.RelayForLogQuery, error) {
	query := model.RelayForLogQuery{Page: 1, PageSize: 25, SortBy: c.DefaultQuery("sort_by", "created_at"), SortOrder: c.DefaultQuery("sort_order", "desc"), Keyword: strings.TrimSpace(c.Query("keyword"))}
	if !admin {
		query.UserID = c.GetInt("id")
	}
	for key, destination := range map[string]*int{"page": &query.Page, "page_size": &query.PageSize, "status_code": &query.StatusCode, "exclude_status_code": &query.ExcludeStatusCode} {
		if value := c.Query(key); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return query, errors.New("invalid " + key)
			}
			*destination = parsed
		}
	}
	for key, destination := range map[string]*int64{"start_timestamp": &query.StartTimestamp, "end_timestamp": &query.EndTimestamp} {
		if value := c.Query(key); value != "" {
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil || parsed < 0 {
				return query, errors.New("invalid " + key)
			}
			*destination = parsed
		}
	}
	if query.Page < 1 || query.Page > 100000 || query.PageSize < 1 || query.PageSize > 100 || len(query.Keyword) > 200 {
		return query, errors.New("invalid pagination or keyword")
	}
	if query.EndTimestamp > 0 && query.StartTimestamp > query.EndTimestamp {
		return query, errors.New("invalid time range")
	}
	for _, status := range []int{query.StatusCode, query.ExcludeStatusCode} {
		if status != 0 && (status < 100 || status > 599) {
			return query, errors.New("invalid status code")
		}
	}
	if query.SortOrder != "asc" && query.SortOrder != "desc" {
		return query, errors.New("invalid sort order")
	}
	switch query.SortBy {
	case "created_at", "token_name", "model", "total_tokens", "quota", "duration_ms", "status_code":
	default:
		return query, errors.New("invalid sort column")
	}
	return query, nil
}

func relayForRespond(c *gin.Context, data any, err error) {
	if err != nil {
		status := http.StatusInternalServerError
		message := "读取网关数据失败"
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
			message = "记录不存在"
		}
		c.JSON(status, gin.H{"success": false, "message": message})
		return
	}
	common.ApiSuccess(c, data)
}

func RelayForUserOverview(c *gin.Context) {
	data, err := model.GetRelayForUserOverview(c.GetInt("id"), time.Now())
	relayForRespond(c, data, err)
}
func RelayForAdminOverview(c *gin.Context) {
	days, err := strconv.Atoi(c.DefaultQuery("days", "7"))
	if err != nil || (days != 7 && days != 30) {
		c.JSON(400, gin.H{"success": false, "message": "统计周期仅支持7或30天"})
		return
	}
	data, err := model.GetRelayForAdminOverview(days, time.Now())
	relayForRespond(c, data, err)
}

func RelayForModelHealth(c *gin.Context) {
	data, err := model.GetRelayForModelHealth(time.Now())
	relayForRespond(c, data, err)
}

func RelayForUserModelHealth(c *gin.Context) {
	group, err := model.GetUserGroup(c.GetInt("id"), true)
	if err != nil {
		relayForRespond(c, nil, err)
		return
	}
	allowed := make(map[string]bool)
	for _, entry := range filterPricingByUsableGroups(model.GetPricing(), service.GetUserUsableGroups(group)) {
		allowed[entry.ModelName] = true
	}
	data, err := model.GetRelayForModelHealth(time.Now())
	if err != nil {
		relayForRespond(c, nil, err)
		return
	}
	relayForRespond(c, model.RelayForPublicModelHealth(data, allowed), nil)
}

func relayForLogs(c *gin.Context, admin bool) {
	q, err := relayForQuery(c, admin)
	if err != nil {
		c.JSON(400, gin.H{"success": false, "message": err.Error()})
		return
	}
	data, err := model.GetRelayForLogs(q, admin)
	relayForRespond(c, data, err)
}
func RelayForUserLogs(c *gin.Context)  { relayForLogs(c, false) }
func RelayForAdminLogs(c *gin.Context) { relayForLogs(c, true) }

func RelayForExportUserLogs(c *gin.Context) {
	q, err := relayForQuery(c, false)
	if err != nil {
		c.JSON(400, gin.H{"success": false, "message": err.Error()})
		return
	}
	writer := csv.NewWriter(c.Writer)
	started := false
	start := func() error {
		if started {
			return nil
		}
		started = true
		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Header("Content-Disposition", `attachment; filename="relayfor-usage.csv"`)
		_, _ = c.Writer.Write([]byte{0xef, 0xbb, 0xbf})
		return writer.Write([]string{"created_at", "token_name", "model", "prompt_tokens", "cached_tokens", "cache_write_tokens", "completion_tokens", "quota", "duration_ms", "status_code", "request_id"})
	}
	count := 0
	err = model.ExportRelayForLogs(c.Request.Context(), q, func(item model.RelayForLogItem) error {
		if err := start(); err != nil {
			return err
		}
		if err := writer.Write([]string{strconv.FormatInt(item.CreatedAt, 10), relayForCSVText(item.TokenName), relayForCSVText(item.Model), strconv.Itoa(item.PromptTokens), strconv.FormatInt(item.CachedTokens, 10), strconv.FormatInt(item.CacheWriteTokens, 10), strconv.Itoa(item.CompletionTokens), strconv.Itoa(item.Quota), strconv.FormatInt(item.DurationMS, 10), strconv.Itoa(item.StatusCode), relayForCSVText(item.RequestID)}); err != nil {
			return err
		}
		count++
		if count%100 == 0 {
			writer.Flush()
			if err := writer.Error(); err != nil {
				return err
			}
			c.Writer.Flush()
		}
		return nil
	})
	if err != nil {
		if !started {
			relayForRespond(c, nil, err)
		} else {
			common.SysError("usage CSV export interrupted")
		}
		return
	}
	_ = start()
	writer.Flush()
}

func relayForCSVText(text string) string {
	trimmed := strings.TrimLeft(text, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + text
	}
	return text
}
func RelayForAdminTraces(c *gin.Context) {
	q, err := relayForQuery(c, true)
	if err != nil {
		c.JSON(400, gin.H{"success": false, "message": err.Error()})
		return
	}
	status := c.Query("status")
	switch status {
	case "", "succeeded", "failed", "rejected", "running", "aborted":
	default:
		c.JSON(400, gin.H{"success": false, "message": "invalid trace status"})
		return
	}
	data, err := model.GetRelayForTraces(q, status)
	relayForRespond(c, data, err)
}
func RelayForAdminTrace(c *gin.Context) {
	id := c.Param("request_id")
	if id == "" || len(id) > 64 {
		c.JSON(400, gin.H{"success": false, "message": "invalid request id"})
		return
	}
	trace, events, err := model.GetRelayForTrace(id)
	relayForRespond(c, gin.H{"trace": trace, "events": events}, err)
}

func RelayForGetPreferences(c *gin.Context) {
	data, err := model.GetRelayForPreferences(c.GetInt("id"))
	relayForRespond(c, data, err)
}

func RelayForPatchPreferences(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var updates map[string]any
	decoder := json.NewDecoder(c.Request.Body)
	if err := decoder.Decode(&updates); err != nil || updates == nil {
		c.JSON(400, gin.H{"success": false, "message": "invalid preferences"})
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		c.JSON(400, gin.H{"success": false, "message": "invalid preferences"})
		return
	}
	for key, value := range updates {
		valid := false
		switch key {
		case "help_docs_visible", "reduced_motion", "hide_locked_models", "sidebar_collapsed", "show_seconds", "hide_successful_logs":
			_, valid = value.(bool)
		case "theme":
			text, ok := value.(string)
			valid = ok && (text == "system" || text == "light" || text == "dark")
		case "timezone":
			text, ok := value.(string)
			if ok && len(text) <= 64 {
				if text == "local" {
					valid = true
				} else {
					_, err := time.LoadLocation(text)
					valid = err == nil
				}
			}
		case "default_page":
			text, ok := value.(string)
			if ok {
				text = model.RelayForActivityPageAlias(text)
				updates[key] = text
				switch text {
				case "overview", "tokens", "models", "usage", "redeem", "subscriptions", "activities", "profile", "docs":
					valid = true
				}
			}
		case "hour_cycle":
			text, ok := value.(string)
			valid = ok && (text == "24" || text == "12")
		case "page_size":
			number, ok := value.(float64)
			valid = ok && (number == 10 || number == 25 || number == 50 || number == 100)
			if valid {
				updates[key] = int(number)
			}
		}
		if !valid {
			c.JSON(400, gin.H{"success": false, "message": "invalid preference: " + key})
			return
		}
	}
	data, err := model.PatchRelayForPreferences(c.GetInt("id"), updates)
	relayForRespond(c, data, err)
}
