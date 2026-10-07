package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/pkg/billingexpr"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func PreWssConsumeQuota(c *gin.Context, info *relaycommon.RelayInfo, usage *dto.RealtimeUsage) error {
	if info == nil || usage == nil {
		return errors.New("Realtime 用量上下文无效。")
	}
	if info.UsePrice {
		return nil
	}
	if group, ok := common.GetContextKey(c, constant.ContextKeyAutoGroup); ok {
		if name, valid := group.(string); valid && name != "" {
			info.UsingGroup = name
		}
	}
	group := ratio_setting.GetGroupRatio(info.UsingGroup)
	if special, exists := ratio_setting.GetGroupGroupRatio(info.UserGroup, info.UsingGroup); exists {
		group = special
	}
	ratio, _, _ := ratio_setting.GetModelRatio(info.OriginModelName)
	quota, clamp := calculateAudioQuota(QuotaInfo{
		InputDetails:  TokenDetails{TextTokens: usage.InputTokenDetails.TextTokens, AudioTokens: usage.InputTokenDetails.AudioTokens},
		OutputDetails: TokenDetails{TextTokens: usage.OutputTokenDetails.TextTokens, AudioTokens: usage.OutputTokenDetails.AudioTokens},
		ModelName:     info.OriginModelName, ModelRatio: ratio, GroupRatio: group,
	})
	noteQuotaClamp(info, clamp)
	if clamp != nil {
		return clamp
	}
	if info.Billing == nil {
		info.ForcePreConsume = true
		if err := PreConsumeBilling(c, 0, info); err != nil {
			return err
		}
	}
	if session, ok := info.Billing.(*BillingSession); ok {
		return session.reserveRealtimeUsage(quota)
	}
	return errors.New("Realtime 计费会话类型无效。")
}

func PostAudioConsumeQuota(c *gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage, note string) {
	if info == nil || !claimUsagePublication(c, info) {
		return
	}
	usage = effectiveBillingUsage(usage)
	if usage == nil {
		usage = &dto.Usage{PromptTokens: info.GetEstimatePromptTokens(), TotalTokens: info.GetEstimatePromptTokens()}
	}
	name := info.GetBillingModelName()
	publication, result := mediaUsageQuote(info, name,
		TokenDetails{TextTokens: usage.PromptTokensDetails.TextTokens, AudioTokens: usage.PromptTokensDetails.AudioTokens},
		TokenDetails{TextTokens: usage.CompletionTokenDetails.TextTokens, AudioTokens: usage.CompletionTokenDetails.AudioTokens},
		usage.TotalTokens, mediaTieredTokens(info, usage), note)
	publication.log.PromptTokens, publication.log.CompletionTokens = usage.PromptTokens, usage.CompletionTokens
	publication.log.Other = GenerateAudioOtherInfo(c, info, usage, info.PriceData.ModelRatio, info.PriceData.GroupRatioInfo.GroupRatio,
		ratio_setting.GetCompletionRatio(name), ratio_setting.GetAudioRatio(name), ratio_setting.GetAudioCompletionRatio(name), info.PriceData.ModelPrice, info.PriceData.GroupRatioInfo.GroupSpecialRatio)
	publication.log.TokenName = c.GetString("token_name")
	if result != nil {
		InjectTieredBillingInfo(publication.log.Other, info, result)
	}
	publishUsage(c, info, publication)
}

func PostWssConsumeQuota(c *gin.Context, info *relaycommon.RelayInfo, name string, usage *dto.RealtimeUsage, note string) {
	if info == nil || !claimUsagePublication(c, info) {
		return
	}
	if usage == nil {
		usage = &dto.RealtimeUsage{}
	}
	publication, result := mediaUsageQuote(info, name,
		TokenDetails{TextTokens: usage.InputTokenDetails.TextTokens, AudioTokens: usage.InputTokenDetails.AudioTokens},
		TokenDetails{TextTokens: usage.OutputTokenDetails.TextTokens, AudioTokens: usage.OutputTokenDetails.AudioTokens},
		usage.TotalTokens, billingexpr.TokenParams{P: float64(usage.InputTokens), C: float64(usage.OutputTokens), Len: float64(usage.InputTokens)}, note)
	publication.log.PromptTokens, publication.log.CompletionTokens = usage.InputTokens, usage.OutputTokens
	publication.log.Other = GenerateWssOtherInfo(c, info, usage, info.PriceData.ModelRatio, info.PriceData.GroupRatioInfo.GroupRatio,
		ratio_setting.GetCompletionRatio(name), ratio_setting.GetAudioRatio(info.OriginModelName), ratio_setting.GetAudioCompletionRatio(name), info.PriceData.ModelPrice, info.PriceData.GroupRatioInfo.GroupSpecialRatio)
	publication.log.TokenName = c.GetString("token_name")
	if result != nil {
		InjectTieredBillingInfo(publication.log.Other, info, result)
	}
	publishUsage(c, info, publication)
}

func mediaTieredTokens(info *relaycommon.RelayInfo, usage *dto.Usage) billingexpr.TokenParams {
	var variables map[string]bool
	if snapshot := info.TieredBillingSnapshot; snapshot != nil {
		variables = billingexpr.UsedVarsByHash(snapshot.ExprString, snapshot.ExprHash)
	}
	return BuildTieredTokenParams(usage, false, variables)
}

func mediaUsageQuote(info *relaycommon.RelayInfo, name string, input, output TokenDetails, total int, tokens billingexpr.TokenParams, note string) (usagePublication, *billingexpr.TieredResult) {
	price := info.PriceData
	quota, clamp := calculateAudioQuota(QuotaInfo{InputDetails: input, OutputDetails: output, ModelName: name,
		UsePrice: price.UsePrice, ModelPrice: price.ModelPrice, ModelRatio: price.ModelRatio, GroupRatio: price.GroupRatioInfo.GroupRatio})
	noteQuotaClamp(info, clamp)
	var settlement *billingexpr.TieredResult
	fixed := false
	if applied, actual, result := TryTieredSettle(info, tokens); applied {
		quota, settlement, fixed = actual, result, isFixedPriceSettlement(info, result)
	}
	content := fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f", price.ModelPrice, price.GroupRatioInfo.GroupRatio)
	if !price.UsePrice {
		content = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f",
			price.ModelRatio, ratio_setting.GetCompletionRatio(name), ratio_setting.GetAudioRatio(name), ratio_setting.GetAudioCompletionRatio(name), price.GroupRatioInfo.GroupRatio)
	}
	billable := total > 0 || fixed
	if !billable {
		quota = 0
		content += "（可能是上游超时）"
	}
	if note != "" {
		content += ", " + note
	}
	seconds := int(time.Since(info.StartTime).Seconds())
	if seconds < 0 {
		seconds = 0
	}
	return usagePublication{billable: billable, log: model.RecordConsumeLogParams{ChannelId: info.ChannelId, ModelName: name,
		Quota: quota, Content: content, TokenId: info.TokenId, UseTimeSeconds: seconds, IsStream: info.IsStream, Group: info.UsingGroup}}, settlement
}
