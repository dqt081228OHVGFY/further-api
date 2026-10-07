package service

import (
	"fmt"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/pkg/billingexpr"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type usagePublication struct {
	log      model.RecordConsumeLogParams
	billable bool
}

// Accounting and its public record have one publication owner. Re-entering a
// completion callback cannot increment usage counters or publish a second log.
func claimUsagePublication(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if session, ok := info.Billing.(*BillingSession); ok {
		session.mu.Lock()
		defer session.mu.Unlock()
		if session.usagePublished {
			return false
		}
		session.usagePublished = true
		return true
	}
	if c != nil {
		const key = "further_usage_publication"
		if _, published := c.Get(key); published {
			return false
		}
		c.Set(key, true)
	}
	return true
}

func publishUsage(c *gin.Context, info *relaycommon.RelayInfo, publication usagePublication) {
	charged, err := SettleBillingWithFinalQuota(c, info, publication.log.Quota)
	publication.log.Quota = charged
	if err != nil {
		logger.LogError(c, "请求计费结算未完整提交："+err.Error())
		publication.log.Other.SetAdmin("settlement_error", err.Error())
	}
	if publication.billable {
		model.UpdateUserUsedQuotaAndRequestCount(info.UserId, charged)
		model.UpdateChannelUsedQuota(info.ChannelId, charged)
	}
	attachQuotaSaturation(c, info, publication.log.Other)
	model.RecordConsumeLog(c, info.UserId, publication.log)
	info.PerformanceOutputTokens = int64(publication.log.CompletionTokens)
}

func PostTextConsumeQuota(c *gin.Context, info *relaycommon.RelayInfo, original *dto.Usage, annotations []string) {
	if info == nil || !claimUsagePublication(c, info) {
		return
	}
	usage := effectiveBillingUsage(original)
	summary := calculateTextQuotaSummary(c, info, usage)
	if original != nil {
		ObserveChannelAffinityUsageCacheByRelayFormat(c, usage, info.GetFinalRequestRelayFormat())
	}
	if usage == nil && info.TieredBillingSnapshot != nil && billingexpr.UsesFixedPricingByHash(info.TieredBillingSnapshot.ExprString, info.TieredBillingSnapshot.ExprHash) {
		usage = &dto.Usage{PromptTokens: summary.PromptTokens, CompletionTokens: summary.CompletionTokens, TotalTokens: summary.TotalTokens}
	}
	var settlement *billingexpr.TieredResult
	tieredApplied := false
	if usage != nil {
		var variables map[string]bool
		if snapshot := info.TieredBillingSnapshot; snapshot != nil {
			variables = billingexpr.UsedVarsByHash(snapshot.ExprString, snapshot.ExprHash)
		}
		if applied, quota, result := TryTieredSettle(info, BuildTieredTokenParams(usage, summary.IsClaudeUsageSemantic, variables)); applied {
			tieredApplied = true
			settlement = result
			summary.Quota = composeTieredTextQuota(info, summary, quota, result)
			summary.FixedPriceBilling = isFixedPriceSettlement(info, result)
			if summary.FixedPriceBilling {
				summary.AudioInputPrice = 0
			}
		}
	}
	content := append([]string(nil), annotations...)
	if original == nil {
		content = append(content, "上游无计费信息")
	}
	content = append(content, textSurchargeDescriptions(summary)...)
	if !summary.hasBillableUsage() {
		content = append(content, "上游没有返回计费信息，无法扣费（可能是上游超时）")
		logger.LogWarn(c, fmt.Sprintf("请求没有可计费用量（user=%d, channel=%d, model=%s）", info.UserId, info.ChannelId, summary.ModelName))
	}
	logModel := summary.ModelName
	for _, prefix := range []string{"gpt-4-gizmo", "gpt-4o-gizmo"} {
		if strings.HasPrefix(logModel, prefix) {
			logModel = prefix + "-*"
			content = append(content, "模型 "+summary.ModelName)
			break
		}
	}
	other := textPublicationMetadata(c, info, original, usage, summary)
	if tieredApplied {
		InjectTieredBillingInfo(other, info, settlement)
	}
	publishUsage(c, info, usagePublication{billable: summary.hasBillableUsage(), log: model.RecordConsumeLogParams{
		ChannelId: info.ChannelId, PromptTokens: summary.PromptTokens, CompletionTokens: summary.CompletionTokens,
		ModelName: logModel, TokenName: summary.TokenName, Quota: summary.Quota, Content: strings.Join(content, ", "),
		TokenId: info.TokenId, UseTimeSeconds: int(summary.UseTimeSeconds), IsStream: info.IsStream, Group: info.UsingGroup, Other: other,
	}})
}

func textSurchargeDescriptions(summary textQuotaSummary) []string {
	result := make([]string, 0, len(summary.ToolSurchargeItems)+1)
	conversion := decimal.NewFromFloat(summary.GroupRatio).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	for _, item := range summary.ToolSurchargeItems {
		cost := decimal.NewFromFloat(item.Price).Mul(decimal.NewFromInt(int64(item.Count))).Div(decimal.NewFromInt(1000)).Mul(conversion)
		result = append(result, fmt.Sprintf("%s 调用 %d 次，调用花费 %s", item.Name, item.Count, logger.LogQuota(common.QuotaFromDecimal(cost))))
	}
	if summary.AudioInputPrice > 0 && summary.AudioTokens > 0 {
		cost := decimal.NewFromFloat(summary.AudioInputPrice).Mul(decimal.NewFromInt(int64(summary.AudioTokens))).Div(decimal.NewFromInt(1_000_000)).Mul(conversion)
		result = append(result, "Audio Input 花费 "+logger.LogQuota(common.QuotaFromDecimal(cost)))
	}
	return result
}

func textPublicationMetadata(c *gin.Context, info *relaycommon.RelayInfo, original, usage *dto.Usage, summary textQuotaSummary) *model.LogOther {
	var other *model.LogOther
	special := info.PriceData.GroupRatioInfo.GroupSpecialRatio
	if summary.IsClaudeUsageSemantic {
		other = GenerateClaudeOtherInfo(c, info, summary.ModelRatio, summary.GroupRatio, summary.CompletionRatio,
			summary.CacheTokens, summary.CacheRatio, summary.CacheCreationTokens, summary.CacheCreationRatio,
			summary.CacheCreationTokens5m, summary.CacheCreationRatio5m, summary.CacheCreationTokens1h, summary.CacheCreationRatio1h, summary.ModelPrice, special)
		other.SetPublic("usage_semantic", "anthropic")
	} else {
		other = GenerateTextOtherInfo(c, info, summary.ModelRatio, summary.GroupRatio, summary.CompletionRatio, summary.CacheTokens, summary.CacheRatio, summary.ModelPrice, special)
	}
	appendUsageBillingPathForLog(other, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens), original)
	if reason := common.GetContextKeyString(c, constant.ContextKeyAdminRejectReason); reason != "" {
		other.SetAdmin("reject_reason", reason)
	}
	if summary.ImageTokens != 0 {
		other.SetPublic("image", true)
		other.SetPublic("image_ratio", summary.ImageRatio)
		other.SetPublic("image_output", summary.ImageTokens)
	}
	appendToolSurchargeLogInfo(other, summary.ToolSurchargeItems)
	if summary.AudioInputPrice > 0 && summary.AudioTokens > 0 {
		other.SetPublic("audio_input_seperate_price", true)
		other.SetPublic("audio_input_token_count", summary.AudioTokens)
		other.SetPublic("audio_input_price", summary.AudioInputPrice)
	}
	for _, cache := range []struct {
		key      string
		count    int
		ratio    float64
		ratioKey string
	}{
		{"cache_creation_tokens", summary.CacheCreationTokens, summary.CacheCreationRatio, "cache_creation_ratio"},
		{"cache_creation_tokens_5m", summary.CacheCreationTokens5m, summary.CacheCreationRatio5m, "cache_creation_ratio_5m"},
		{"cache_creation_tokens_1h", summary.CacheCreationTokens1h, summary.CacheCreationRatio1h, "cache_creation_ratio_1h"},
	} {
		if cache.count > 0 {
			other.SetPublic(cache.key, cache.count)
			other.SetPublic(cache.ratioKey, cache.ratio)
		}
	}
	if count := cacheWriteTokensTotal(summary); count > 0 {
		other.SetPublic("cache_write_tokens", count)
	}
	// Only a tagged normalization may declare an authoritative input total.
	if info.GetFinalRequestRelayFormat() != types.RelayFormatClaude && usage != nil && usage.UsageSource != "" && usage.InputTokens > 0 {
		other.SetPublic("input_tokens_total", usage.InputTokens)
	}
	return other
}
