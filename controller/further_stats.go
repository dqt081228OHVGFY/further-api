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

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func furtherQuery(c *gin.Context, admin bool) (model.FurtherLogQuery, error) {
	query := model.FurtherLogQuery{Page: 1, PageSize: 25, SortBy: c.DefaultQuery("sort_by", "created_at"), SortOrder: c.DefaultQuery("sort_order", "desc"), Keyword: strings.TrimSpace(c.Query("keyword"))}
	if !admin {
		query.UserID = c.GetInt("id")
	}
	if value := c.Query("only_errors"); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return query, errors.New("invalid only_errors")
		}
		query.OnlyErrors = parsed
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

func furtherRespond(c *gin.Context, data any, err error) {
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

func FurtherUserOverview(c *gin.Context) {
	data, err := model.GetFurtherUserOverview(c.GetInt("id"), time.Now())
	furtherRespond(c, data, err)
}
func FurtherAdminOverview(c *gin.Context) {
	days, err := strconv.Atoi(c.DefaultQuery("days", "7"))
	if err != nil || (days != 7 && days != 30) {
		c.JSON(400, gin.H{"success": false, "message": "统计周期仅支持7或30天"})
		return
	}
	data, err := model.GetFurtherAdminOverview(days, time.Now())
	furtherRespond(c, data, err)
}

func FurtherModelHealth(c *gin.Context) {
	data, err := model.GetFurtherModelHealth(time.Now())
	furtherRespond(c, data, err)
}

func FurtherUserModelHealth(c *gin.Context) {
	group, err := model.GetUserGroup(c.GetInt("id"), true)
	if err != nil {
		furtherRespond(c, nil, err)
		return
	}
	allowed := make(map[string]bool)
	for _, entry := range filterPricingByUsableGroups(model.GetPricing(), service.GetUserUsableGroups(group)) {
		allowed[entry.ModelName] = true
	}
	data, err := model.GetFurtherModelHealth(time.Now())
	if err != nil {
		furtherRespond(c, nil, err)
		return
	}
	furtherRespond(c, model.FurtherPublicModelHealth(data, allowed), nil)
}

func furtherLogs(c *gin.Context, admin bool) {
	q, err := furtherQuery(c, admin)
	if err != nil {
		c.JSON(400, gin.H{"success": false, "message": err.Error()})
		return
	}
	data, err := model.GetFurtherLogs(q, admin)
	furtherRespond(c, data, err)
}
func FurtherUserLogs(c *gin.Context)  { furtherLogs(c, false) }
func FurtherAdminLogs(c *gin.Context) { furtherLogs(c, true) }

func FurtherExportUserLogs(c *gin.Context) {
	q, err := furtherQuery(c, false)
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
		c.Header("Content-Disposition", `attachment; filename="further-api-usage.csv"`)
		_, _ = c.Writer.Write([]byte{0xef, 0xbb, 0xbf})
		return writer.Write([]string{"created_at", "token_name", "model", "prompt_tokens", "cached_tokens", "cache_write_tokens", "completion_tokens", "quota", "duration_ms", "status_code", "request_id"})
	}
	count := 0
	err = model.ExportFurtherLogs(c.Request.Context(), q, func(item model.FurtherLogItem) error {
		if err := start(); err != nil {
			return err
		}
		if err := writer.Write([]string{strconv.FormatInt(item.CreatedAt, 10), furtherCSVText(item.TokenName), furtherCSVText(item.Model), strconv.Itoa(item.PromptTokens), strconv.FormatInt(item.CachedTokens, 10), strconv.FormatInt(item.CacheWriteTokens, 10), strconv.Itoa(item.CompletionTokens), strconv.Itoa(item.Quota), strconv.FormatInt(item.DurationMS, 10), strconv.Itoa(item.StatusCode), furtherCSVText(item.RequestID)}); err != nil {
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
			furtherRespond(c, nil, err)
		} else {
			common.SysError("usage CSV export interrupted")
		}
		return
	}
	_ = start()
	writer.Flush()
}

func furtherCSVText(text string) string {
	trimmed := strings.TrimLeft(text, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + text
	}
	return text
}
func FurtherAdminTraces(c *gin.Context) {
	q, err := furtherQuery(c, true)
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
	data, err := model.GetFurtherTraces(q, status)
	furtherRespond(c, data, err)
}
func FurtherAdminTrace(c *gin.Context) {
	id := c.Param("request_id")
	if id == "" || len(id) > 64 {
		c.JSON(400, gin.H{"success": false, "message": "invalid request id"})
		return
	}
	trace, events, err := model.GetFurtherTrace(id)
	furtherRespond(c, gin.H{"trace": trace, "events": events}, err)
}

func FurtherGetPreferences(c *gin.Context) {
	data, err := model.GetFurtherPreferences(c.GetInt("id"))
	furtherRespond(c, data, err)
}

func FurtherPatchPreferences(c *gin.Context) {
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
				text = model.FurtherActivityPageAlias(text)
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
	data, err := model.PatchFurtherPreferences(c.GetInt("id"), updates)
	furtherRespond(c, data, err)
}
