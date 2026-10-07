package service

import (
	"context"
	"errors"
	"math"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/setting/ratio_setting"
	"github.com/dqt081228OHVGFY/further-api/types"
)

type TokenDetails struct{ TextTokens, AudioTokens int }
type QuotaInfo struct {
	InputDetails, OutputDetails        TokenDetails
	ModelName                          string
	UsePrice                           bool
	ModelPrice, ModelRatio, GroupRatio float64
}

func hasCustomModelRatio(name string, ratio float64) bool {
	standard, exists := ratio_setting.GetDefaultModelRatioMap()[name]
	return !exists || standard != ratio
}

// Solve the observed upstream cost for cache writes. Non-finite input and a
// singular price difference produce an explicit unavailable estimate.
func CalcOpenRouterCacheCreateTokens(usage dto.Usage, price types.PriceData) int {
	if price.CacheCreationRatio == 1 {
		return 0
	}
	cost, valid := usage.Cost.(float64)
	base := price.ModelRatio / common.QuotaPerUnit
	denominator := base * (price.CacheCreationRatio - 1)
	if !valid || math.IsNaN(cost) || math.IsInf(cost, 0) || denominator == 0 || math.IsNaN(denominator) {
		return -1
	}
	accounted := float64(usage.PromptTokens)*base + float64(usage.CompletionTokens)*base*price.CompletionRatio
	credit := float64(usage.PromptTokensDetails.CachedTokens) * base * (1 - price.CacheRatio)
	estimate, clamp := common.QuotaRoundChecked((cost - accounted + credit) / denominator)
	if clamp != nil {
		return -1
	}
	return estimate
}

func PreConsumeTokenQuota(info *relaycommon.RelayInfo, amount int) error {
	if info == nil {
		return errors.New("访问令牌计费上下文无效。")
	}
	return reserveTokenAllowance(context.Background(), info, amount)
}

type postConsumeQuotaResult struct{ FundingApplied, TokenApplied bool }

func PostConsumeQuota(info *relaycommon.RelayInfo, delta, reserved int, notify bool) error {
	_, err := postConsumeQuotaWithResult(info, delta, reserved, notify)
	return err
}

// The compatibility entry reports each committed ledger stage so persisted
// legacy jobs can compensate exactly the stages that succeeded.
func postConsumeQuotaWithResult(info *relaycommon.RelayInfo, delta, reserved int, notify bool) (postConsumeQuotaResult, error) {
	result := postConsumeQuotaResult{}
	if info == nil || delta < -common.MaxQuota || delta > common.MaxQuota || reserved < 0 {
		return result, ErrFundingAmountInvalid
	}
	if err := applyQuotaFundingDelta(info, delta); err != nil {
		return result, err
	}
	result.FundingApplied = true
	if !info.IsPlayground {
		if err := applyQuotaTokenDelta(info, delta); err != nil {
			return result, err
		}
		result.TokenApplied = true
	}
	if notify && int64(delta)+int64(reserved) > 0 {
		if info.BillingSource == BillingSourceSubscription {
			checkAndSendSubscriptionQuotaNotify(info)
		} else {
			checkAndSendQuotaNotify(info, delta, reserved)
		}
	}
	return result, nil
}

func applyQuotaFundingDelta(info *relaycommon.RelayInfo, delta int) error {
	if info.BillingSource == BillingSourceSubscription {
		if info.SubscriptionId <= 0 {
			return errors.New("订阅计费记录缺少订阅编号。")
		}
		if err := model.PostConsumeUserSubscriptionDeltaContext(context.Background(), info.SubscriptionId, int64(delta)); err != nil {
			return err
		}
		info.SubscriptionPostDelta += int64(delta)
		return nil
	}
	if delta == 0 {
		return nil
	}
	if delta < 0 {
		return model.IncreaseUserQuotaContext(context.Background(), info.UserId, -delta, false)
	}
	return model.DecreaseUserQuota(info.UserId, delta, false)
}

func applyQuotaTokenDelta(info *relaycommon.RelayInfo, delta int) error {
	if delta == 0 {
		return nil
	}
	if delta < 0 {
		return model.IncreaseTokenQuotaContext(context.Background(), info.TokenId, info.TokenKey, -delta)
	}
	return model.DecreaseTokenQuota(info.TokenId, info.TokenKey, delta)
}
