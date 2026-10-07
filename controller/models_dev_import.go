package controller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

const modelsDevImportURL = "https://models.dev/api.json"

var modelsDevImportClient = &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
	if len(via) >= 3 || req.URL.Scheme != "https" || req.URL.Host != "models.dev" {
		return errors.New("价格源重定向无效")
	}
	return nil
}}

type devImportPrice struct {
	Input  *float64 `json:"input_price"`
	Output *float64 `json:"output_price"`
	Read   *float64 `json:"cache_read_price"`
	Write  *float64 `json:"cache_write_price"`
}
type devImportCandidate struct {
	ProviderID     string          `json:"provider_id"`
	ProviderName   string          `json:"provider_name"`
	ModelID        string          `json:"model_id"`
	Input          float64         `json:"input_price"`
	Output         float64         `json:"output_price"`
	Read           *float64        `json:"cache_read_price,omitempty"`
	Write          *float64        `json:"cache_write_price,omitempty"`
	Recommended    bool            `json:"recommended"`
	EffectivePrice *devImportPrice `json:"effective_price,omitempty"`
}
type devImportItem struct {
	Model               string                            `json:"model"`
	Status              string                            `json:"status"`
	Reason              string                            `json:"reason"`
	ChannelCount        int                               `json:"channel_count"`
	Current             devImportPrice                    `json:"current"`
	Candidates          []devImportCandidate              `json:"candidates"`
	CompletionRatioInfo ratio_setting.CompletionRatioInfo `json:"completion_ratio_info"`
	entry               model.ModelPricingEntry
}
type devImportSnapshot struct {
	ID           string          `json:"snapshot_id"`
	GeneratedAt  string          `json:"generated_at"`
	ExpiresAt    string          `json:"expires_at"`
	Source       string          `json:"source"`
	Items        []devImportItem `json:"items"`
	MissingOnly  bool            `json:"missing_only"`
	owner        int
	expiry       time.Time
	applying     bool
	fingerprint  string
	updated      int
	quotaPerUnit float64
}

var devImportSnapshots = struct {
	sync.Mutex
	entries map[string]*devImportSnapshot
}{entries: make(map[string]*devImportSnapshot)}

func readDevImportCatalog(ctx context.Context) (map[string][]devImportCandidate, map[string]bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsDevImportURL, nil)
	if err != nil {
		return nil, nil, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := modelsDevImportClient.Do(request)
	if err != nil {
		return nil, nil, fmt.Errorf("读取 models.dev 失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("models.dev 返回 HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (16<<20)+1))
	if err != nil {
		return nil, nil, err
	}
	if len(body) > 16<<20 {
		return nil, nil, errors.New("models.dev 数据超过读取上限")
	}
	var providers map[string]struct {
		Name   string `json:"name"`
		Models map[string]struct {
			ID   string                     `json:"id"`
			Cost map[string]json.RawMessage `json:"cost"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &providers); err != nil || len(providers) == 0 {
		return nil, nil, errors.New("models.dev 价格数据格式无效")
	}
	catalog, unsupported := make(map[string][]devImportCandidate), make(map[string]bool)
	providerIDs := make([]string, 0, len(providers))
	for id := range providers {
		providerIDs = append(providerIDs, id)
	}
	sort.Strings(providerIDs)
	for _, providerID := range providerIDs {
		provider := providers[providerID]
		for key, source := range provider.Models {
			name := source.ID
			if name == "" {
				name = key
			}
			if name == "" || len(name) > 256 {
				continue
			}
			readCost := func(key string) *float64 {
				raw, exists := source.Cost[key]
				if !exists || string(raw) == "null" {
					return nil
				}
				var value float64
				if json.Unmarshal(raw, &value) != nil || !isValidNonNegativeCost(value) {
					return nil
				}
				return &value
			}
			input, output, cacheRead, cacheWrite := readCost("input"), readCost("output"), readCost("cache_read"), readCost("cache_write")
			complex := false
			for costKey, raw := range source.Cost {
				if costKey != "input" && costKey != "output" && costKey != "cache_read" && costKey != "cache_write" && string(raw) != "null" && string(raw) != "0" && string(raw) != "[]" && string(raw) != "{}" {
					complex = true
				}
			}
			if input == nil || output == nil || complex || (*input == 0 && (*output > 0 || cacheRead != nil && *cacheRead > 0 || cacheWrite != nil && *cacheWrite > 0)) || input != nil && output != nil && *input > 0 && (!isValidNonNegativeCost(*output / *input) || cacheRead != nil && !isValidNonNegativeCost(*cacheRead / *input) || cacheWrite != nil && !isValidNonNegativeCost(*cacheWrite / *input)) {
				unsupported[name] = true
				continue
			}
			candidate := devImportCandidate{ProviderID: providerID, ProviderName: provider.Name, ModelID: name, Input: *input, Output: *output, Read: cacheRead, Write: cacheWrite}
			if candidate.ProviderName == "" {
				candidate.ProviderName = providerID
			}
			catalog[name] = append(catalog[name], candidate)
		}
	}
	return catalog, unsupported, nil
}

func devCurrentPrice(entry model.ModelPricingEntry) devImportPrice {
	result := devImportPrice{}
	inputRatio, ok := entry.Effective["ModelRatio"].(float64)
	if !ok || common.QuotaPerUnit <= 0 {
		return result
	}
	input := inputRatio * 1e6 / common.QuotaPerUnit
	if !isValidNonNegativeCost(input) {
		return result
	}
	result.Input = &input
	price := func(key string) *float64 {
		value, ok := entry.Effective[key].(float64)
		if !ok || !isValidNonNegativeCost(value) {
			return nil
		}
		value *= input
		if !isValidNonNegativeCost(value) {
			return nil
		}
		return &value
	}
	result.Output, result.Read, result.Write = price("CompletionRatio"), price("CacheRatio"), price("CreateCacheRatio")
	return result
}

func devImportProtected(entry model.ModelPricingEntry) string {
	if _, ok := entry.Effective["ModelPrice"]; ok {
		return "按次计费保留现有价格"
	}
	if entry.Effective["billing_setting.billing_mode"] == "tiered_expr" || len(entry.PluginVariants) > 0 {
		return "表达式或插件计费保留现有配置"
	}
	if value, ok := entry.Effective["ModelRatio"].(float64); ok && value == 0 {
		return "免费模型保留现有配置"
	}
	if _, ok := entry.Effective["AudioRatio"]; ok {
		return "音频模型需单独配置价格"
	}
	if _, ok := entry.Effective["AudioCompletionRatio"]; ok {
		return "音频模型需单独配置价格"
	}
	if common.IsImageGenerationModel(entry.ModelName) || entry.BillingDetails.AudioInputPrice != nil || entry.BillingDetails.ImageCount {
		return "图片或音频计费保留现有配置"
	}
	if ratio_setting.FormatMatchingModelName(entry.ModelName) != entry.ModelName {
		return "共享通配价格需在模型计费中配置"
	}
	return ""
}

func preferredDevProvider(name string) string {
	switch {
	case strings.HasPrefix(name, "claude-"):
		return "anthropic"
	case strings.HasPrefix(name, "gemini-"):
		return "google"
	case strings.HasPrefix(name, "gpt-"), strings.HasPrefix(name, "chatgpt-"), strings.HasPrefix(name, "text-embedding-"), strings.HasPrefix(name, "o1"), strings.HasPrefix(name, "o3"), strings.HasPrefix(name, "o4"):
		return "openai"
	}
	return ""
}

// Do not offer a price that the ratio engine cannot actually charge. Some
// legacy models enforce their output ratio even when a different one is saved.
func devImportPriceRepresentable(name string, candidate devImportCandidate, quotaPerUnit float64) bool {
	ratio := candidate.Input * (quotaPerUnit / 1e6)
	if !isValidNonNegativeCost(ratio) || candidate.Input > 0 && ratio == 0 {
		return false
	}
	if candidate.Input == 0 {
		return candidate.Output == 0
	}
	outputRatio := candidate.Output / candidate.Input
	if !isValidNonNegativeCost(outputRatio) || candidate.Output > 0 && outputRatio == 0 {
		return false
	}
	info := ratio_setting.ResolveCompletionRatio(name, &outputRatio)
	return !info.Locked || math.Abs(info.Ratio-outputRatio) <= math.Max(1, math.Max(info.Ratio, outputRatio))*1e-9
}

func buildDevImportSnapshot(ctx context.Context, owner int, missingOnly bool, requestedModel string) (*devImportSnapshot, error) {
	catalog, unsupported, err := readDevImportCatalog(ctx)
	if err != nil {
		return nil, err
	}
	metadata, _, err := model.SearchModelsWithChannels("", "", "", "", 0, -1)
	if err != nil {
		return nil, err
	}
	bound, err := model.GetConfiguredModelChannels()
	if err != nil {
		return nil, err
	}
	names, seen := make([]string, 0, len(metadata)), map[string]bool{}
	for _, entry := range metadata {
		if !seen[entry.ModelName] {
			names = append(names, entry.ModelName)
			seen[entry.ModelName] = true
		}
	}
	if requestedModel != "" {
		names = []string{requestedModel}
	}
	if len(names) > 10000 {
		return nil, errors.New("模型超过一万项，请缩小目录后重试")
	}
	if common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
		return nil, errors.New("站点额度单位无效")
	}
	state := &model.ModelPricingSnapshot{}
	if len(names) > 0 {
		state, err = model.GetModelPricingSnapshot(names)
		if err != nil {
			return nil, err
		}
	}
	now := time.Now().UTC()
	snapshot := &devImportSnapshot{ID: common.GetUUID(), GeneratedAt: now.Format(time.RFC3339), ExpiresAt: now.Add(10 * time.Minute).Format(time.RFC3339), Source: modelsDevImportURL, Items: make([]devImportItem, 0, len(names)), owner: owner, expiry: now.Add(10 * time.Minute), quotaPerUnit: common.QuotaPerUnit}
	snapshot.MissingOnly = missingOnly
	for _, entry := range state.Entries {
		item := devImportItem{Model: entry.ModelName, Status: "unmatched", Current: devCurrentPrice(entry), ChannelCount: len(bound[entry.ModelName]), Candidates: []devImportCandidate{}, CompletionRatioInfo: entry.CompletionRatioInfo, entry: entry}
		if item.ChannelCount == 0 && requestedModel == "" {
			item.Status, item.Reason = "unbound", "先关联渠道，再导入价格"
		} else if reason := devImportProtected(entry); reason != "" {
			item.Status, item.Reason = "protected", reason
		} else if missingOnly && item.Current.Input != nil && item.Current.Output != nil {
			item.Status, item.Reason = "configured", "已有有效价格，保留现有配置"
		} else {
			incompatible := false
			for _, candidate := range catalog[entry.ModelName] {
				draft, draftErr := devImportDraft(item, candidate)
				if draftErr != nil {
					incompatible = true
					continue
				}
				candidate.EffectivePrice = &devImportPrice{Input: &candidate.Input, Output: &candidate.Output}
				valid := true
				for _, cache := range []struct {
					key    string
					target **float64
				}{{"CacheRatio", &candidate.EffectivePrice.Read}, {"CreateCacheRatio", &candidate.EffectivePrice.Write}} {
					ratio, ok := draft[cache.key].(float64)
					if !ok {
						ratio, _ = entry.Effective[cache.key].(float64)
					}
					price := candidate.Input * ratio
					if !isValidNonNegativeCost(price) {
						valid = false
						break
					}
					*cache.target = &price
				}
				if valid {
					item.Candidates = append(item.Candidates, candidate)
				} else {
					incompatible = true
				}
			}
			item.Reason = "models.dev 未找到完整的按量价格"
			if unsupported[entry.ModelName] {
				item.Reason = "价格源含分段或特殊计费，需手动配置"
			}
			if incompatible && len(item.Candidates) == 0 {
				item.Reason = "参考价格无法由当前模型的固定输出倍率或额度单位准确表示，请在模型编辑中检查计费规则"
			}
			if len(item.Candidates) > 0 {
				item.Status, item.Reason = "ambiguous", "多个来源有不同价格，请选择来源"
				preferred := preferredDevProvider(entry.ModelName)
				for index := range item.Candidates {
					if len(item.Candidates) == 1 || preferred != "" && item.Candidates[index].ProviderID == preferred {
						item.Candidates[index].Recommended = true
						item.Status, item.Reason = "matched", ""
					}
				}
			}
			if item.ChannelCount == 0 && len(item.Candidates) > 0 {
				// A draft can read reference prices, but cannot import until it is bound.
				item.Status, item.Reason = "reference", "参考价格可填入模型草稿；模型仍需关联渠道"
			}
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func PreviewModelsDevImport(c *gin.Context) {
	owner := c.GetInt("id")
	if owner <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "请先登录"})
		return
	}
	requestedModel := strings.TrimSpace(c.Query("model"))
	if requestedModel != "" {
		if _, err := validSelectedChannelModels([]string{requestedModel}); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
			return
		}
	}
	snapshot, err := buildDevImportSnapshot(c.Request.Context(), owner, c.Query("missing_only") == "true", requestedModel)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	devImportSnapshots.Lock()
	for id, entry := range devImportSnapshots.entries {
		if time.Now().After(entry.expiry) || entry.owner == owner && !entry.applying {
			delete(devImportSnapshots.entries, id)
		}
	}
	if len(devImportSnapshots.entries) >= 32 {
		devImportSnapshots.Unlock()
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "导入预览较多，请稍后重试"})
		return
	}
	devImportSnapshots.entries[snapshot.ID] = snapshot
	devImportSnapshots.Unlock()
	common.ApiSuccess(c, snapshot)
}

type devImportSelection struct {
	Model      string `json:"model"`
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
}

func devImportDraft(item devImportItem, candidate devImportCandidate) (model.PricingValues, error) {
	if !devImportPriceRepresentable(item.Model, candidate, common.QuotaPerUnit) {
		return nil, errors.New("参考价格无法由当前模型计费规则准确表示")
	}
	pricing := maps.Clone(item.entry.Configured)
	if pricing == nil {
		pricing = model.PricingValues{}
	}
	pricing["billing_setting.billing_mode"] = "ratio"
	pricing["ModelRatio"] = candidate.Input * (common.QuotaPerUnit / 1e6)
	pricing["CompletionRatio"] = float64(1)
	if candidate.Input > 0 {
		pricing["CompletionRatio"] = candidate.Output / candidate.Input
	}
	for _, cache := range []struct {
		key             string
		source, current *float64
	}{{"CacheRatio", candidate.Read, item.Current.Read}, {"CreateCacheRatio", candidate.Write, item.Current.Write}} {
		value := cache.source
		if value == nil {
			value = cache.current
		}
		if value == nil {
			continue
		}
		if candidate.Input == 0 {
			if *value > 0 {
				return nil, errors.New("零输入价格无法表示现有缓存价格")
			}
			pricing[cache.key] = float64(1)
		} else {
			ratio := *value / candidate.Input
			if !isValidNonNegativeCost(ratio) || *value > 0 && ratio == 0 {
				return nil, errors.New("缓存价格超出倍率可表示的范围")
			}
			pricing[cache.key] = ratio
		}
	}
	return pricing, nil
}

func ApplyModelsDevImport(c *gin.Context) {
	owner := c.GetInt("id")
	if owner <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "请先登录"})
		return
	}
	var input struct {
		SnapshotID string               `json:"snapshot_id"`
		Selections []devImportSelection `json:"selections"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
	if err := common.DecodeJson(c.Request.Body, &input); err != nil || len(input.Selections) == 0 || len(input.Selections) > 10000 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请选择要导入的模型价格"})
		return
	}
	sort.Slice(input.Selections, func(i, j int) bool { return input.Selections[i].Model < input.Selections[j].Model })
	encoded, _ := json.Marshal(input.Selections)
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(encoded))
	devImportSnapshots.Lock()
	snapshot := devImportSnapshots.entries[input.SnapshotID]
	if snapshot == nil || snapshot.owner != owner || time.Now().After(snapshot.expiry) || snapshot.quotaPerUnit != common.QuotaPerUnit || snapshot.applying || snapshot.fingerprint != "" && snapshot.fingerprint != fingerprint {
		devImportSnapshots.Unlock()
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "价格预览已过期或正在应用，请重新读取"})
		return
	}
	if snapshot.fingerprint == fingerprint {
		count := snapshot.updated
		devImportSnapshots.Unlock()
		common.ApiSuccess(c, gin.H{"updated_count": count, "skipped_count": 0})
		return
	}
	snapshot.applying = true
	devImportSnapshots.Unlock()
	defer func() { devImportSnapshots.Lock(); snapshot.applying = false; devImportSnapshots.Unlock() }()
	items := make(map[string]devImportItem, len(snapshot.Items))
	for _, item := range snapshot.Items {
		items[item.Model] = item
	}
	bound, err := model.GetConfiguredModelChannels()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	changes := make([]model.ModelPricingChange, 0, len(input.Selections))
	seen := map[string]bool{}
	for _, selection := range input.Selections {
		item, exists := items[selection.Model]
		if !exists || seen[selection.Model] || len(bound[selection.Model]) == 0 || item.Status != "matched" && item.Status != "ambiguous" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "模型选择无效或已不再关联渠道"})
			return
		}
		seen[selection.Model] = true
		var selected *devImportCandidate
		for index := range item.Candidates {
			candidate := &item.Candidates[index]
			if candidate.ProviderID == selection.ProviderID && candidate.ModelID == selection.ModelID {
				selected = candidate
				break
			}
		}
		if selected == nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "价格来源已失效，请重新读取"})
			return
		}
		pricing, err := devImportDraft(item, *selected)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
			return
		}
		changes = append(changes, model.ModelPricingChange{ModelName: item.Model, ExpectedVersion: item.entry.Version, Pricing: pricing})
	}
	if err := model.UpdateModelPricingContext(c.Request.Context(), changes); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrModelPricingConflict) {
			status = http.StatusConflict
		}
		message := "价格无法应用，请重新检查选择。"
		if status == http.StatusConflict {
			message = "模型价格已被其他操作修改，请重新读取后导入。"
		}
		c.JSON(status, gin.H{"success": false, "message": message})
		return
	}
	devImportSnapshots.Lock()
	snapshot.fingerprint, snapshot.updated = fingerprint, len(changes)
	devImportSnapshots.Unlock()
	recordManageAudit(c, "model.pricing.models_dev_import", map[string]any{"source": modelsDevImportURL, "updated_count": len(changes)})
	common.ApiSuccess(c, gin.H{"updated_count": len(changes), "skipped_count": 0})
}
