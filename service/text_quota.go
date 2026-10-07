package service

import (
	"math"
	"sort"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/pkg/billingexpr"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// ToolSurchargeItem is one billable tool-call line for consume logs.
type ToolSurchargeItem struct {
	Name  string  `json:"name"`
	Count int     `json:"count"`
	Price float64 `json:"price"`
}

func appendToolSurchargeLogInfo(other *model.LogOther, items []ToolSurchargeItem) {
	if len(items) == 0 {
		return
	}
	other.SetPublic("tool_surcharges", items)
}

type textQuotaSummary struct {
	PromptTokens           int
	CompletionTokens       int
	TotalTokens            int
	CacheTokens            int
	CacheCreationTokens    int
	CacheCreationTokens5m  int
	CacheCreationTokens1h  int
	ImageTokens            int
	AudioTokens            int
	ModelName              string
	TokenName              string
	UseTimeSeconds         int64
	CompletionRatio        float64
	CacheRatio             float64
	ImageRatio             float64
	ModelRatio             float64
	GroupRatio             float64
	ModelPrice             float64
	CacheCreationRatio     float64
	CacheCreationRatio5m   float64
	CacheCreationRatio1h   float64
	Quota                  int
	IsClaudeUsageSemantic  bool
	UsageSemantic          string
	AudioInputPrice        float64
	ToolSurchargeItems     []ToolSurchargeItem
	ToolCallSurchargeQuota decimal.Decimal
	FixedPriceBilling      bool
}

// hasBillableUsage reports whether this request should incur any charge.
// A request can carry zero tokens yet still be billable via a tool-call
// surcharge (e.g. /v1/alpha/search returns no usage but bills one web_search
// call), so token count alone is not sufficient to decide.
func (s *textQuotaSummary) hasBillableUsage() bool {
	return s.FixedPriceBilling || s.TotalTokens > 0 || !s.ToolCallSurchargeQuota.IsZero()
}

func cacheWriteTokensTotal(summary textQuotaSummary) int {
	if summary.CacheCreationTokens5m > 0 || summary.CacheCreationTokens1h > 0 {
		splitCacheWriteTokens := summary.CacheCreationTokens5m + summary.CacheCreationTokens1h
		if summary.CacheCreationTokens > splitCacheWriteTokens {
			return summary.CacheCreationTokens
		}
		return splitCacheWriteTokens
	}
	return summary.CacheCreationTokens
}

func isLegacyClaudeDerivedOpenAIUsage(relayInfo *relaycommon.RelayInfo, usage *dto.Usage) bool {
	if relayInfo == nil || usage == nil {
		return false
	}
	if relayInfo.GetFinalRequestRelayFormat() == types.RelayFormatClaude {
		return false
	}
	if usage.UsageSource != "" || usage.UsageSemantic != "" {
		return false
	}
	return usage.ClaudeCacheCreation5mTokens > 0 || usage.ClaudeCacheCreation1hTokens > 0
}

func collectToolSurchargeItem(items []ToolSurchargeItem, name string, count int, modelName string) []ToolSurchargeItem {
	if count <= 0 {
		return items
	}
	price := operation_setting.GetToolPriceForModel(name, modelName)
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return items
	}
	return append(items, ToolSurchargeItem{
		Name:  name,
		Count: count,
		Price: price,
	})
}

func mergeToolSurchargeItems(items []ToolSurchargeItem) []ToolSurchargeItem {
	if len(items) == 0 {
		return nil
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == items[j].Name {
			return items[i].Price < items[j].Price
		}
		return items[i].Name < items[j].Name
	})

	merged := items[:0]
	for _, item := range items {
		lastIndex := len(merged) - 1
		if lastIndex >= 0 &&
			merged[lastIndex].Name == item.Name &&
			merged[lastIndex].Price == item.Price {
			if item.Count > math.MaxInt-merged[lastIndex].Count {
				common.SysError("tool surcharge call count overflow for " + item.Name)
				merged[lastIndex].Count = math.MaxInt
			} else {
				merged[lastIndex].Count += item.Count
			}
			continue
		}
		merged = append(merged, item)
	}
	return merged
}

func calculateTextToolCallSurcharge(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, summary *textQuotaSummary) decimal.Decimal {
	dGroupRatio := decimal.NewFromFloat(summary.GroupRatio)
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)

	var items []ToolSurchargeItem

	if relayInfo.ResponsesUsageInfo != nil {
		for name, tool := range relayInfo.ResponsesUsageInfo.BuiltInTools {
			if tool == nil {
				continue
			}
			items = collectToolSurchargeItem(items, name, tool.CallCount, summary.ModelName)
		}
	}
	if relayInfo.RelayMode != relayconstant.RelayModeResponses &&
		strings.HasSuffix(summary.ModelName, "search-preview") {
		items = collectToolSurchargeItem(items, dto.BuildInToolWebSearchPreview, 1, summary.ModelName)
	}

	items = collectToolSurchargeItem(
		items,
		dto.BuildInToolWebSearch,
		ctx.GetInt("claude_web_search_requests"),
		summary.ModelName,
	)

	if ctx.GetBool("gemini_google_search_call") {
		items = collectToolSurchargeItem(items, dto.BuildInToolGoogleSearch, 1, summary.ModelName)
	}

	summary.ToolSurchargeItems = mergeToolSurchargeItems(items)
	var surcharge decimal.Decimal
	for _, item := range summary.ToolSurchargeItems {
		surcharge = surcharge.Add(decimal.NewFromFloat(item.Price).
			Mul(decimal.NewFromInt(int64(item.Count))).
			Div(decimal.NewFromInt(1000)).
			Mul(dGroupRatio).
			Mul(dQuotaPerUnit))
	}

	return surcharge
}

// noteQuotaClamp records the first quota saturation event onto relayInfo so it
// can later be attached to the consume/task log for admin auditing. First
// non-nil clamp wins (a single request may hit multiple conversions).
func noteQuotaClamp(relayInfo *relaycommon.RelayInfo, clamp *common.QuotaClamp) {
	if clamp == nil || relayInfo == nil {
		return
	}
	if relayInfo.QuotaClamp == nil {
		relayInfo.QuotaClamp = clamp
	}
}

func composeTieredTextQuota(relayInfo *relaycommon.RelayInfo, summary textQuotaSummary, tieredQuota int, tieredResult *billingexpr.TieredResult) int {
	if summary.ToolCallSurchargeQuota.IsZero() {
		return tieredQuota
	}

	if tieredResult != nil {
		if snap := relayInfo.TieredBillingSnapshot; snap != nil {
			quota, clamp := common.QuotaFromDecimalChecked(decimal.NewFromFloat(tieredResult.ActualQuotaBeforeGroup).
				Mul(decimal.NewFromFloat(snap.GroupRatio)).
				Add(summary.ToolCallSurchargeQuota))
			noteQuotaClamp(relayInfo, clamp)
			return quota
		}
	}

	// Saturate the final sum, not just the surcharge: tieredQuota can be near
	// MaxQuota and adding the surcharge could push the total past the
	// single-request quota policy bound.
	total, clamp := common.QuotaFromDecimalChecked(
		decimal.NewFromInt(int64(tieredQuota)).Add(summary.ToolCallSurchargeQuota),
	)
	noteQuotaClamp(relayInfo, clamp)
	return total
}

// calculateTextQuotaSummary expects a usage already remapped by
// effectiveBillingUsage; PostTextConsumeQuota performs that remap once and shares
// the result with tiered billing, affinity observation and logging.
func usageSemanticFromUsage(relayInfo *relaycommon.RelayInfo, usage *dto.Usage) string {
	if usage != nil && usage.UsageSemantic != "" {
		return usage.UsageSemantic
	}
	if relayInfo != nil && relayInfo.GetFinalRequestRelayFormat() == types.RelayFormatClaude {
		return "anthropic"
	}
	return "openai"
}
