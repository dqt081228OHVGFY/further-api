package model

import (
	"sort"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
)

type FurtherHealthWindow struct {
	Requests    int64   `json:"requests"`
	Successful  int64   `json:"successful"`
	SuccessRate float64 `json:"success_rate"`
}

// Public health deliberately omits channels, revenue, cache usage and latency.
type FurtherPublicModelHealthEntry struct {
	Model       string              `json:"model"`
	DisplayName string              `json:"display_name"`
	Recent60    FurtherHealthWindow `json:"recent_60"`
	Today       FurtherHealthWindow `json:"today"`
}

func FurtherPublicModelHealth(data map[string]any, allowed map[string]bool) map[string]any {
	items := make([]FurtherPublicModelHealthEntry, 0)
	for _, entry := range data["models"].([]*FurtherHealthEntry) {
		if !allowed[entry.Model] {
			continue
		}
		items = append(items, FurtherPublicModelHealthEntry{
			Model: entry.Model, DisplayName: entry.DisplayName,
			Recent60: entry.Recent60, Today: entry.Today,
		})
	}
	return map[string]any{"items": items, "generated_at": data["generated_at"], "timezone": data["timezone"]}
}

type FurtherHealthEntry struct {
	Model                  string              `json:"model,omitempty"`
	DisplayName            string              `json:"display_name,omitempty"`
	ChannelID              int                 `json:"channel_id,omitempty"`
	Name                   string              `json:"name,omitempty"`
	Status                 int                 `json:"status"`
	Enabled                bool                `json:"enabled"`
	Recent60               FurtherHealthWindow `json:"recent_60"`
	Today                  FurtherHealthWindow `json:"today"`
	PromptTokens           int64               `json:"prompt_tokens"`
	CachedTokens           int64               `json:"cached_tokens"`
	CacheHitRate           float64             `json:"cache_hit_rate"`
	TodayChargeQuota       int64               `json:"today_charge_quota"`
	AverageTTFTMS          float64             `json:"average_ttft_ms"`
	AverageTokensPerSecond float64             `json:"average_tokens_per_second"`
	ttftSum                float64
	ttftCount              int64
	outputTokens           int64
	generationMS           float64
}

type furtherHealthRow struct {
	ModelName    string
	ChannelID    int
	Successful   int64
	Quota        int64
	PromptTokens int64
	CachedTokens int64
	TTFTSum      float64
	TTFTCount    int64
	OutputTokens int64
	GenerationMS float64
}

// Fold retry attempts into one outcome per request before computing rates.
func furtherHealthRequests(channel bool, start, end int64, recent bool) *gorm.DB {
	entity := "model_name"
	if channel {
		entity = "channel_id"
	}
	query := LOG_DB.Model(&Log{}).Where("type IN ?", []int{LogTypeConsume, LogTypeError})
	query = query.Where("created_at <= ?", end)
	if start > 0 {
		query = query.Where("created_at >= ?", start)
	}
	if channel {
		query = query.Where("channel_id > 0")
	}
	fields := entity + ", request_id, MAX(created_at) AS latest, MAX(id) AS latest_id, MAX(CASE WHEN type = 2 THEN 1 ELSE 0 END) AS successful"
	if !recent {
		frt := furtherJSONNumber("frt")
		measured := "type = 2 AND is_stream = ? AND " + frt + " > 0"
		generation := "(use_time * 1000 - " + frt + ")"
		fields += ", SUM(CASE WHEN type = 2 THEN quota ELSE 0 END) AS quota, SUM(CASE WHEN type = 2 THEN prompt_tokens ELSE 0 END) AS prompt_tokens"
		fields += ", SUM(CASE WHEN type = 2 THEN " + furtherJSONNumber("cache_tokens") + " ELSE 0 END) AS cached_tokens"
		fields += ", SUM(CASE WHEN " + measured + " THEN " + frt + " ELSE 0 END) AS ttft_sum"
		fields += ", SUM(CASE WHEN " + measured + " THEN 1 ELSE 0 END) AS ttft_count"
		fields += ", SUM(CASE WHEN " + measured + " AND " + generation + " > 0 AND completion_tokens > 0 THEN completion_tokens ELSE 0 END) AS output_tokens"
		fields += ", SUM(CASE WHEN " + measured + " AND " + generation + " > 0 AND completion_tokens > 0 THEN " + generation + " ELSE 0 END) AS generation_ms"
		query = query.Select(fields, true, true, true, true)
	} else {
		query = query.Select(fields)
	}
	return query.Group(entity + ", request_id, CASE WHEN request_id IS NULL OR request_id = '' THEN id ELSE 0 END")
}

func readFurtherHealthRows(query *gorm.DB, visit func(furtherHealthRow)) error {
	result, err := query.Rows()
	if err != nil {
		return err
	}
	defer result.Close()
	for result.Next() {
		var row furtherHealthRow
		if err := LOG_DB.ScanRows(result, &row); err != nil {
			return err
		}
		visit(row)
	}
	return result.Err()
}

func (entry *FurtherHealthEntry) record(row furtherHealthRow, recent bool) {
	window := &entry.Today
	if recent {
		window = &entry.Recent60
	}
	window.Requests++
	window.Successful += row.Successful
	window.SuccessRate = float64(window.Successful) * 100 / float64(window.Requests)
	if recent {
		return
	}
	entry.TodayChargeQuota += row.Quota
	entry.PromptTokens += row.PromptTokens
	entry.CachedTokens += row.CachedTokens
	entry.ttftSum += row.TTFTSum
	entry.ttftCount += row.TTFTCount
	entry.outputTokens += row.OutputTokens
	entry.generationMS += row.GenerationMS
	if entry.PromptTokens > 0 {
		entry.CacheHitRate = float64(entry.CachedTokens) * 100 / float64(entry.PromptTokens)
	}
	if entry.ttftCount > 0 {
		entry.AverageTTFTMS = entry.ttftSum / float64(entry.ttftCount)
	}
	if entry.generationMS > 0 {
		entry.AverageTokensPerSecond = float64(entry.outputTokens) * 1000 / entry.generationMS
	}
}

func GetFurtherModelHealth(now time.Time) (map[string]any, error) {
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	local := now.In(zone)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone).Unix()
	models := map[string]*FurtherHealthEntry{}
	channels := map[int]*FurtherHealthEntry{}
	var configured []Model
	if err := DB.Select("model_name", "status").Find(&configured).Error; err != nil {
		return nil, err
	}
	for _, item := range configured {
		models[item.ModelName] = &FurtherHealthEntry{Model: item.ModelName, DisplayName: item.ModelName, Enabled: item.Status == 1}
	}
	var configuredChannels []Channel
	if err := DB.Select("id", "name", "status", "models").Find(&configuredChannels).Error; err != nil {
		return nil, err
	}
	for _, channel := range configuredChannels {
		channels[channel.Id] = &FurtherHealthEntry{ChannelID: channel.Id, Name: channel.Name, Status: channel.Status}
		for name := range strings.SplitSeq(channel.Models, ",") {
			name = strings.TrimSpace(name)
			if name == "" || name == "*" {
				continue
			}
			if models[name] == nil {
				models[name] = &FurtherHealthEntry{Model: name, DisplayName: name}
			}
			if channel.Status == common.ChannelStatusEnabled {
				models[name].Enabled = true
			}
		}
	}
	var summary FurtherHealthEntry
	for _, byChannel := range []bool{false, true} {
		for _, recent := range []bool{false, true} {
			timestamp := start
			if recent {
				timestamp = 0
			}
			query := furtherHealthRequests(byChannel, timestamp, now.Unix(), recent)
			if recent {
				entity := "model_name"
				if byChannel {
					entity = "channel_id"
				}
				ranked := LOG_DB.Table("(?) AS health_requests", query).
					Select("health_requests.*, ROW_NUMBER() OVER (PARTITION BY " + entity + " ORDER BY latest DESC, latest_id DESC) AS health_rank")
				query = LOG_DB.Table("(?) AS health_ranked", ranked).Where("health_rank <= ?", 60)
			}
			err := readFurtherHealthRows(query, func(row furtherHealthRow) {
				var entry *FurtherHealthEntry
				if byChannel {
					entry = channels[row.ChannelID]
					if entry == nil {
						entry = &FurtherHealthEntry{ChannelID: row.ChannelID, Name: "已删除渠道"}
						channels[row.ChannelID] = entry
					}
				} else {
					if !recent {
						summary.record(row, false)
					}
					if row.ModelName == "" {
						return
					}
					entry = models[row.ModelName]
					if entry == nil {
						entry = &FurtherHealthEntry{Model: row.ModelName, DisplayName: row.ModelName}
						models[row.ModelName] = entry
					}
				}
				entry.record(row, recent)
			})
			if err != nil {
				return nil, err
			}
		}
	}
	modelRows := make([]*FurtherHealthEntry, 0, len(models))
	for _, item := range models {
		modelRows = append(modelRows, item)
	}
	sort.Slice(modelRows, func(i, j int) bool { return modelRows[i].Model < modelRows[j].Model })
	channelRows := make([]*FurtherHealthEntry, 0, len(channels))
	for _, item := range channels {
		channelRows = append(channelRows, item)
	}
	sort.Slice(channelRows, func(i, j int) bool { return channelRows[i].ChannelID < channelRows[j].ChannelID })
	return map[string]any{
		"generated_at": now.Format(time.RFC3339), "timezone": "Asia/Shanghai",
		"summary": map[string]any{"today": summary.Today, "prompt_tokens": summary.PromptTokens, "cached_tokens": summary.CachedTokens, "cache_hit_rate": summary.CacheHitRate, "charge_quota": summary.TodayChargeQuota},
		"models":  modelRows, "channels": channelRows,
	}, nil
}
