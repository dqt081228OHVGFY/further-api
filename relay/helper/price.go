package helper

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/pkg/billingexpr"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert/reasoning"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting/billing_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/ratio_setting"
	hostreasoning "github.com/dqt081228OHVGFY/further-api/setting/reasoning"
	hosttypes "github.com/dqt081228OHVGFY/further-api/types"
	"github.com/gin-gonic/gin"
)

const claudeCacheCreation1hMultiplier = 6.0 / 3.75

func modelPriceNotConfiguredError(name string, userID int) error {
	if model.IsAdmin(userID) {
		return fmt.Errorf("模型 %s 的价格未配置，请在系统设置的模型定价中配置，或开启自用模式。", name)
	}
	return fmt.Errorf("模型 %s 的价格尚未配置，请联系站点管理员开启。", name)
}

func HandleGroupRatio(c *gin.Context, info *relaycommon.RelayInfo) hosttypes.GroupRatioInfo {
	result := hosttypes.GroupRatioInfo{GroupRatio: 1, GroupSpecialRatio: -1}
	if info == nil {
		return result
	}
	if c != nil {
		if group, ok := c.Get("auto_group"); ok {
			if name, ok := group.(string); ok && name != "" {
				info.UsingGroup = name
			}
		}
	}
	if special, available := ratio_setting.GetGroupGroupRatio(info.UserGroup, info.UsingGroup); available {
		result.GroupRatio, result.GroupSpecialRatio, result.HasSpecialRatio = special, special, true
	} else {
		result.GroupRatio = ratio_setting.GetGroupRatio(info.UsingGroup)
	}
	return result
}

func HasModelBillingConfig(name string) bool {
	if _, ok := ratio_setting.GetModelPrice(name, false); ok {
		return true
	}
	if _, ok, _ := ratio_setting.GetModelRatio(name); ok {
		return true
	}
	expr, ok := billing_setting.GetBillingExpr(name)
	return billing_setting.GetBillingMode(name) == billing_setting.BillingModeTieredExpr && ok && strings.TrimSpace(expr) != ""
}

func HasPriceOrRatioEntry(name string) bool {
	name = ratio_setting.FormatMatchingModelName(name)
	if _, ok := ratio_setting.GetModelPrice(name, false); ok {
		return true
	}
	return ratio_setting.HasConfiguredModelRatio(name) || billing_setting.GetBillingMode(name) == billing_setting.BillingModeTieredExpr
}

func resolveBillingModelName(origin string) string {
	base := hostreasoning.BaseModelName(origin)
	names := hostreasoning.CanonicalBillingModelNames(origin)
	if !reasoning.ParseModelModifiers(origin).HasModifiers() {
		names = append([]string{origin}, names...)
	}
	names = append(names, base)
	seen := make(map[string]bool)
	for _, name := range names {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if HasPriceOrRatioEntry(name) {
			return name
		}
	}
	return base
}

func ModelPriceHelper(c *gin.Context, info *relaycommon.RelayInfo, prompt int, meta *types.TokenCountMeta) (hosttypes.PriceData, error) {
	if info == nil || prompt < 0 {
		return hosttypes.PriceData{}, errors.New("模型报价请求无效。")
	}
	if c != nil && c.Request != nil && c.Request.Context().Err() != nil {
		return hosttypes.PriceData{}, c.Request.Context().Err()
	}
	if meta == nil {
		meta = &types.TokenCountMeta{}
	}
	resolved := resolveBillingModelName(info.GetOriginModelName())
	if resolved != "" && resolved != info.OriginModelName {
		info.BillingModelName = resolved
	}
	name, group := info.GetBillingModelName(), HandleGroupRatio(c, info)
	if billing_setting.GetBillingMode(name) == billing_setting.BillingModeTieredExpr {
		return modelPriceHelperTiered(c, info, name, prompt, group)
	}
	price := hosttypes.PriceData{GroupRatioInfo: group}
	price.ModelPrice, price.UsePrice = ratio_setting.GetModelPrice(name, false)
	base := 0.0
	if price.UsePrice {
		if meta.ImagePriceRatio != 0 {
			price.ModelPrice *= meta.ImagePriceRatio
		}
		base = price.ModelPrice * common.QuotaPerUnit
		for name, factor := range meta.BillingRatios {
			price.AddOtherRatio(name, factor)
		}
	} else {
		var configured bool
		var matched string
		price.ModelRatio, configured, matched = ratio_setting.GetModelRatio(name)
		if !configured && !info.UserSetting.AcceptUnsetRatioModel {
			return hosttypes.PriceData{}, modelPriceNotConfiguredError(matched, info.UserId)
		}
		multiplier, err := operation_setting.InputPreConsumeMultiplier()
		if err != nil {
			return hosttypes.PriceData{}, err
		}
		base = float64(prompt) * multiplier * price.ModelRatio
		price.CompletionRatio = ratio_setting.GetCompletionRatio(name)
		price.CacheRatio, _ = ratio_setting.GetCacheRatio(name)
		price.CacheCreationRatio, _ = ratio_setting.GetCreateCacheRatio(name)
		price.CacheCreation5mRatio = price.CacheCreationRatio
		price.CacheCreation1hRatio = price.CacheCreationRatio * claudeCacheCreation1hMultiplier
		price.ImageRatio, _ = ratio_setting.GetImageRatio(name)
		price.AudioRatio = ratio_setting.GetAudioRatio(name)
		price.AudioCompletionRatio = ratio_setting.GetAudioCompletionRatio(name)
	}
	if request, image := info.Request.(*dto.ImageRequest); image {
		info.ImageQuotaBeforeGroup = base
		count, err := request.ImageCount(false)
		if err != nil {
			return hosttypes.PriceData{}, err
		}
		if price.UsePrice {
			price.AddOtherRatio("n", float64(count))
		}
	}
	price.FreeModel = !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume &&
		(group.GroupRatio == 0 || price.UsePrice && price.ModelPrice == 0 || !price.UsePrice && price.ModelRatio == 0)
	quote, err := service.PriceReservation(service.ReservationQuoteInput{Base: base, GroupRatio: group.GroupRatio, Factors: price.OtherRatios()})
	if err != nil {
		return hosttypes.PriceData{}, err
	}
	price.QuotaToPreConsume = quote.Quota
	info.TieredBillingSnapshot = nil
	info.PriceData = price
	return price, nil
}

func ModelPriceHelperPerCall(c *gin.Context, info *relaycommon.RelayInfo) (hosttypes.PriceData, error) {
	if info == nil {
		return hosttypes.PriceData{}, errors.New("按次报价请求无效。")
	}
	price := hosttypes.PriceData{GroupRatioInfo: HandleGroupRatio(c, info)}
	price.ModelPrice, price.UsePrice = ratio_setting.GetModelPrice(info.OriginModelName, true)
	if !price.UsePrice {
		if fallback, ok := ratio_setting.GetDefaultModelPriceMap()[info.OriginModelName]; ok {
			price.ModelPrice, price.UsePrice = fallback, true
		}
	}
	base := 0.0
	if price.UsePrice {
		base = price.ModelPrice * common.QuotaPerUnit
	} else {
		var configured bool
		var matched string
		price.ModelRatio, configured, matched = ratio_setting.GetModelRatio(info.OriginModelName)
		if !configured && !info.UserSetting.AcceptUnsetRatioModel {
			return hosttypes.PriceData{}, modelPriceNotConfiguredError(matched, info.UserId)
		}
		price.ModelPrice = -1
		base = price.ModelRatio * common.QuotaPerUnit / 2
	}
	price.FreeModel = !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume &&
		(price.GroupRatioInfo.GroupRatio == 0 || price.UsePrice && price.ModelPrice == 0 || !price.UsePrice && price.ModelRatio == 0)
	quote, err := service.PriceReservation(service.ReservationQuoteInput{Base: base, GroupRatio: price.GroupRatioInfo.GroupRatio})
	price.Quota = quote.Quota
	return price, err
}

func modelPriceHelperTiered(c *gin.Context, info *relaycommon.RelayInfo, name string, prompt int, group hosttypes.GroupRatioInfo) (hosttypes.PriceData, error) {
	expr, exists := billing_setting.GetBillingExpr(name)
	if !exists || strings.TrimSpace(expr) == "" {
		return hosttypes.PriceData{}, fmt.Errorf("模型 %s 缺少分层计费表达式。", name)
	}
	hash := billingexpr.ExprHashString(expr)
	if info.RelayFormat == types.RelayFormatOpenAIRealtime && billingexpr.UsesFixedPricingByHash(expr, hash) {
		return hosttypes.PriceData{}, errors.New("Realtime 请求暂不支持固定单价表达式。")
	}
	multiplier, err := operation_setting.InputPreConsumeMultiplier()
	if err != nil {
		return hosttypes.PriceData{}, err
	}
	input, err := ResolveIncomingBillingExprRequestInput(c, info)
	if err != nil {
		return hosttypes.PriceData{}, err
	}
	if billingexpr.UsedVarsByHash(expr, hash)["image_count"] {
		input, err = ResolveImageBillingRequestInput(c, info, input)
		if err != nil {
			return hosttypes.PriceData{}, err
		}
	}
	cost, trace, err := billingexpr.RunExprByHashWithRequest(expr, hash, billingexpr.TokenParams{P: float64(prompt), Len: float64(prompt)}, input)
	if err != nil {
		return hosttypes.PriceData{}, fmt.Errorf("模型 %s 的计费表达式执行失败：%w", name, err)
	}
	base := cost * common.QuotaPerUnit / 1_000_000
	if trace.BillingUnit != billingexpr.BillingUnitRequest {
		base *= multiplier
	}
	quote, err := service.PriceReservation(service.ReservationQuoteInput{Base: base, GroupRatio: group.GroupRatio, Round: true})
	if err != nil {
		return hosttypes.PriceData{}, err
	}
	snapshot := &billingexpr.BillingSnapshot{
		EstimatedImageCount: trace.ImageCount, BillingMode: billing_setting.BillingModeTieredExpr,
		ModelName: name, ExprString: expr, ExprHash: hash, GroupRatio: group.GroupRatio,
		EstimatedPromptTokens: prompt, PreConsumeMultiplier: multiplier,
		EstimatedQuotaBeforeGroup: base, EstimatedQuotaAfterGroup: quote.Quota,
		EstimatedTier: trace.MatchedTier, EstimatedBillingUnit: trace.BillingUnit,
		EstimatedFixedPrice: trace.FixedPrice, QuotaPerUnit: common.QuotaPerUnit, ExprVersion: billingexpr.ExprVersion(expr),
	}
	price := hosttypes.PriceData{GroupRatioInfo: group, QuotaToPreConsume: quote.Quota,
		FreeModel: !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume && group.GroupRatio == 0,
	}
	info.TieredBillingSnapshot, info.BillingRequestInput, info.PriceData = snapshot, &input, price
	return price, nil
}
