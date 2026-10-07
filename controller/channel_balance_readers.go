package controller

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
)

type gatewayBalanceReader func(context.Context, *model.Channel) (float64, error)

var gatewayBalanceReaders = map[int]gatewayBalanceReader{
	constant.ChannelTypeOpenAI:  readOpenAIBalance,
	constant.ChannelTypeCustom:  readOpenAIBalance,
	constant.ChannelTypeAIProxy: readAIProxyBalance,
	constant.ChannelTypeAPI2GPT: func(ctx context.Context, ch *model.Channel) (float64, error) {
		return readBalanceJSON[API2GPTUsageResponse](ctx, ch, "https://api.api2gpt.com", "/dashboard/billing/credit_grants", GetAuthHeader(ch.Key), func(r API2GPTUsageResponse) (float64, error) { return r.TotalRemaining, nil })
	},
	constant.ChannelTypeAIGC2D: func(ctx context.Context, ch *model.Channel) (float64, error) {
		return readBalanceJSON[APGC2DGPTUsageResponse](ctx, ch, "https://api.aigc2d.com", "/dashboard/billing/credit_grants", GetAuthHeader(ch.Key), func(r APGC2DGPTUsageResponse) (float64, error) { return r.TotalAvailable, nil })
	},
	constant.ChannelTypeSiliconFlow: readSiliconFlowBalance,
	constant.ChannelTypeDeepSeek: func(ctx context.Context, ch *model.Channel) (float64, error) {
		return readBalanceJSON[DeepSeekUsageResponse](ctx, ch, "https://api.deepseek.com", "/user/balance", GetAuthHeader(ch.Key), func(r DeepSeekUsageResponse) (float64, error) {
			return getDeepSeekBalanceUSD(r, operation_setting.USDExchangeRate)
		})
	},
	constant.ChannelTypeOpenRouter: func(ctx context.Context, ch *model.Channel) (float64, error) {
		return readBalanceJSON[OpenRouterCreditResponse](ctx, ch, "https://openrouter.ai/api", "/v1/credits", GetAuthHeader(ch.Key), func(r OpenRouterCreditResponse) (float64, error) { return r.Data.TotalCredits - r.Data.TotalUsage, nil })
	},
	constant.ChannelTypeMoonshot: readMoonshotBalance,
}

func balanceEndpoint(ch *model.Channel, fallback, suffix string) string {
	base := strings.TrimRight(ch.GetBaseURL(), "/")
	if base == "" {
		base = strings.TrimRight(fallback, "/")
	}
	return base + suffix
}

func readBalanceJSON[T any](ctx context.Context, ch *model.Channel, base, path string, headers http.Header, project func(T) (float64, error)) (float64, error) {
	body, err := getResponseBodyContext(ctx, http.MethodGet, balanceEndpoint(ch, base, path), ch, headers)
	if err != nil {
		return 0, err
	}
	var response T
	if err := common.Unmarshal(body, &response); err != nil {
		return 0, err
	}
	return project(response)
}

func readOpenAIBalance(ctx context.Context, ch *model.Channel) (float64, error) {
	target := balanceEndpoint(ch, constant.GetChannelBaseURL(constant.ChannelTypeOpenAI), "/v1/dashboard/billing/subscription")
	body, err := getResponseBodyContext(ctx, http.MethodGet, target, ch, GetAuthHeader(ch.Key))
	if err != nil {
		return 0, err
	}
	var subscription OpenAISubscriptionResponse
	if err := common.Unmarshal(body, &subscription); err != nil {
		return 0, err
	}
	now := time.Now()
	start := now.Format("2006-01") + "-01"
	if !subscription.HasPaymentMethod {
		start = now.AddDate(0, 0, -100).Format("2006-01-02")
	}
	query := url.Values{"start_date": {start}, "end_date": {now.Format("2006-01-02")}}
	target = balanceEndpoint(ch, constant.GetChannelBaseURL(constant.ChannelTypeOpenAI), "/v1/dashboard/billing/usage") + "?" + query.Encode()
	body, err = getResponseBodyContext(ctx, http.MethodGet, target, ch, GetAuthHeader(ch.Key))
	if err != nil {
		return 0, err
	}
	var usage OpenAIUsageResponse
	if err := common.Unmarshal(body, &usage); err != nil {
		return 0, err
	}
	return subscription.HardLimitUSD - usage.TotalUsage/100, nil
}

func readAIProxyBalance(ctx context.Context, ch *model.Channel) (float64, error) {
	headers := http.Header{"Api-Key": {ch.Key}}
	return readBalanceJSON[AIProxyUserOverviewResponse](ctx, ch, "https://aiproxy.io", "/api/report/getUserOverview", headers, func(response AIProxyUserOverviewResponse) (float64, error) {
		if !response.Success {
			return 0, fmt.Errorf("余额服务返回错误 %d。", response.ErrorCode)
		}
		return response.Data.TotalPoints, nil
	})
}

func readSiliconFlowBalance(ctx context.Context, ch *model.Channel) (float64, error) {
	return readBalanceJSON[SiliconFlowUsageResponse](ctx, ch, "https://api.siliconflow.cn", "/v1/user/info", GetAuthHeader(ch.Key), func(response SiliconFlowUsageResponse) (float64, error) {
		if response.Code != 20000 {
			return 0, fmt.Errorf("余额服务返回错误 %d。", response.Code)
		}
		return strconv.ParseFloat(response.Data.TotalBalance, 64)
	})
}

func readMoonshotBalance(ctx context.Context, ch *model.Channel) (float64, error) {
	type response struct {
		Code   int  `json:"code"`
		Status bool `json:"status"`
		Data   struct {
			Available float64 `json:"available_balance"`
		} `json:"data"`
	}
	return readBalanceJSON[response](ctx, ch, "https://api.moonshot.cn", "/v1/users/me/balance", GetAuthHeader(ch.Key), func(value response) (float64, error) {
		if !value.Status || value.Code != 0 {
			return 0, fmt.Errorf("余额服务返回错误 %d。", value.Code)
		}
		if !validBalanceExchange(operation_setting.Price) {
			return 0, errors.New("余额换算汇率无效。")
		}
		return value.Data.Available / operation_setting.Price, nil
	})
}

func validBalanceExchange(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
