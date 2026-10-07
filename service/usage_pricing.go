package service

import (
	"math"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func finiteUsageRate(value float64) (decimal.Decimal, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return decimal.Zero, false
	}
	return decimal.NewFromFloat(value), true
}

func calculateAudioQuota(info QuotaInfo) (int, *common.QuotaClamp) {
	if info.UsePrice {
		amount, valid := finiteUsageRate(info.ModelPrice * common.QuotaPerUnit * info.GroupRatio)
		if !valid {
			return common.QuotaFromFloatChecked(info.ModelPrice * common.QuotaPerUnit * info.GroupRatio)
		}
		return common.QuotaFromDecimalChecked(amount)
	}
	audio := ratio_setting.GetAudioRatio(info.ModelName)
	outputAudio := ratio_setting.GetAudioCompletionRatio(info.ModelName)
	components := []RatedUsageComponent{
		{Units: info.InputDetails.TextTokens, Rate: 1},
		{Units: info.OutputDetails.TextTokens, Rate: ratio_setting.GetCompletionRatio(info.ModelName)},
		{Units: info.InputDetails.AudioTokens, Rate: audio},
		{Units: info.OutputDetails.AudioTokens, Rate: audio * outputAudio},
	}
	return PriceUsageComponents(components, info.ModelRatio*info.GroupRatio, nil, decimal.Zero, true)
}

func usageCountSum(left, right int) int {
	if left < 0 {
		left = 0
	}
	if right < 0 {
		right = 0
	}
	if right > math.MaxInt-left {
		return math.MaxInt
	}
	return left + right
}

// Normalize the provider's usage facts before quoting. Vendor cache semantics
// are explicit here; the amount calculation has no provider or request I/O.
func calculateTextQuotaSummary(c *gin.Context, info *relaycommon.RelayInfo, supplied *dto.Usage) textQuotaSummary {
	usage := supplied
	if usage == nil {
		usage = &dto.Usage{PromptTokens: info.GetEstimatePromptTokens()}
	}
	price := info.PriceData
	summary := textQuotaSummary{
		ModelName: info.GetBillingModelName(), UsageSemantic: usageSemanticFromUsage(info, supplied),
		PromptTokens: max(0, usage.PromptTokens), CompletionTokens: max(0, usage.CompletionTokens),
		CacheTokens:           max(0, usage.PromptTokensDetails.CachedTokens),
		CacheCreationTokens:   max(0, usage.PromptTokensDetails.CacheCreationTokensTotal()),
		CacheCreationTokens5m: max(0, usage.ClaudeCacheCreation5mTokens),
		CacheCreationTokens1h: max(0, usage.ClaudeCacheCreation1hTokens),
		ImageTokens:           max(0, usage.PromptTokensDetails.ImageTokens), AudioTokens: max(0, usage.PromptTokensDetails.AudioTokens),
		CompletionRatio: price.CompletionRatio, CacheRatio: price.CacheRatio, ImageRatio: price.ImageRatio,
		ModelRatio: price.ModelRatio, GroupRatio: price.GroupRatioInfo.GroupRatio, ModelPrice: price.ModelPrice,
		CacheCreationRatio: price.CacheCreationRatio, CacheCreationRatio5m: price.CacheCreation5mRatio,
		CacheCreationRatio1h: price.CacheCreation1hRatio,
	}
	if c != nil {
		summary.TokenName = c.GetString("token_name")
	}
	if !info.StartTime.IsZero() {
		summary.UseTimeSeconds = int64(time.Since(info.StartTime).Seconds())
	}
	summary.TotalTokens = usageCountSum(summary.PromptTokens, summary.CompletionTokens)
	summary.IsClaudeUsageSemantic = summary.UsageSemantic == "anthropic"
	independentCache := summary.IsClaudeUsageSemantic || isLegacyClaudeDerivedOpenAIUsage(info, usage)
	if info.ChannelMeta != nil && info.ChannelType == constant.ChannelTypeOpenRouter && summary.IsClaudeUsageSemantic {
		summary.PromptTokens = max(0, summary.PromptTokens-summary.CacheTokens)
		if summary.CacheCreationTokens == 0 && price.CacheCreationRatio != 1 && usage.Cost != 0 && !price.UsePrice && !hasCustomModelRatio(summary.ModelName, price.ModelRatio) {
			inferred := CalcOpenRouterCacheCreateTokens(*usage, price)
			if inferred >= 0 && inferred <= summary.PromptTokens {
				summary.CacheCreationTokens = inferred
			}
		}
		summary.PromptTokens = max(0, summary.PromptTokens-summary.CacheCreationTokens)
	}
	summary.ToolCallSurchargeQuota = calculateTextToolCallSurcharge(c, info, &summary)
	if price.UsePrice {
		amount, valid := finiteUsageRate(price.ModelPrice * common.QuotaPerUnit * summary.GroupRatio)
		if valid {
			quota, clamp := PriceUsageComponents(nil, 1, nil, price.ApplyOtherRatiosToDecimal(amount).Add(summary.ToolCallSurchargeQuota), false)
			summary.Quota = quota
			noteQuotaClamp(info, clamp)
		} else {
			quota, clamp := common.QuotaFromFloatChecked(price.ModelPrice * common.QuotaPerUnit * summary.GroupRatio)
			summary.Quota = quota
			noteQuotaClamp(info, clamp)
		}
	} else {
		base := summary.PromptTokens
		components := []RatedUsageComponent{{Units: summary.CompletionTokens, Rate: summary.CompletionRatio}}
		if summary.CacheTokens > 0 {
			if !independentCache {
				base = max(0, base-summary.CacheTokens)
			}
			components = append(components, RatedUsageComponent{Units: summary.CacheTokens, Rate: summary.CacheRatio})
		}
		if !independentCache {
			base = max(0, base-summary.CacheCreationTokens)
			components = append(components, RatedUsageComponent{Units: summary.CacheCreationTokens, Rate: summary.CacheCreationRatio})
		} else {
			remaining := max(0, summary.CacheCreationTokens-summary.CacheCreationTokens5m-summary.CacheCreationTokens1h)
			components = append(components,
				RatedUsageComponent{Units: remaining, Rate: summary.CacheCreationRatio},
				RatedUsageComponent{Units: summary.CacheCreationTokens5m, Rate: summary.CacheCreationRatio5m},
				RatedUsageComponent{Units: summary.CacheCreationTokens1h, Rate: summary.CacheCreationRatio1h},
			)
		}
		base = max(0, base-summary.ImageTokens)
		components = append(components, RatedUsageComponent{Units: summary.ImageTokens, Rate: summary.ImageRatio})
		var absoluteAudio decimal.Decimal
		if summary.AudioTokens > 0 {
			summary.AudioInputPrice = operation_setting.GetGeminiInputAudioPricePerMillionTokens(summary.ModelName)
			if summary.AudioInputPrice > 0 {
				base = max(0, base-summary.AudioTokens)
				absoluteAudio = decimal.NewFromInt(int64(summary.AudioTokens)).Mul(decimal.NewFromFloat(summary.AudioInputPrice)).Div(decimal.NewFromInt(1_000_000)).Mul(decimal.NewFromFloat(summary.GroupRatio)).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
			}
		}
		components = append(components, RatedUsageComponent{Units: base, Rate: 1})
		ratio := summary.ModelRatio * summary.GroupRatio
		// Absolute audio prices still participate in request-level quantity
		// factors; tool call charges are added after those factors.
		absoluteAudio = price.ApplyOtherRatiosToDecimal(absoluteAudio)
		quota, clamp := PriceUsageComponents(components, ratio, price.OtherRatios(), absoluteAudio.Add(summary.ToolCallSurchargeQuota), true)
		summary.Quota = quota
		noteQuotaClamp(info, clamp)
	}
	if !summary.hasBillableUsage() {
		summary.Quota = 0
	} else if summary.ModelRatio*summary.GroupRatio != 0 && summary.Quota == 0 {
		summary.Quota = 1
	}
	return summary
}
