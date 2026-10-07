package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/advancedcustom"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"

	"github.com/shopspring/decimal"

	"github.com/gin-gonic/gin"
)

type OpenAISubscriptionResponse struct {
	Object             string  `json:"object"`
	HasPaymentMethod   bool    `json:"has_payment_method"`
	SoftLimitUSD       float64 `json:"soft_limit_usd"`
	HardLimitUSD       float64 `json:"hard_limit_usd"`
	SystemHardLimitUSD float64 `json:"system_hard_limit_usd"`
	AccessUntil        int64   `json:"access_until"`
}

type OpenAIUsageDailyCost struct {
	Timestamp float64 `json:"timestamp"`
	LineItems []struct {
		Name string  `json:"name"`
		Cost float64 `json:"cost"`
	}
}

type OpenAICreditGrants struct {
	Object         string  `json:"object"`
	TotalGranted   float64 `json:"total_granted"`
	TotalUsed      float64 `json:"total_used"`
	TotalAvailable float64 `json:"total_available"`
}

const maxAdvancedCustomBalanceResponseBytes = 256 << 10
const channelBalanceRequestTimeout = 30 * time.Second

type channelBalanceResult struct {
	Balance     float64
	RawResponse string
}

type OpenAIUsageResponse struct {
	Object string `json:"object"`
	//DailyCosts []OpenAIUsageDailyCost `json:"daily_costs"`
	TotalUsage float64 `json:"total_usage"` // unit: 0.01 dollar
}

type OpenAISBUsageResponse struct {
	Msg  string `json:"msg"`
	Data *struct {
		Credit string `json:"credit"`
	} `json:"data"`
}

type AIProxyUserOverviewResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	ErrorCode int    `json:"error_code"`
	Data      struct {
		TotalPoints float64 `json:"totalPoints"`
	} `json:"data"`
}

type API2GPTUsageResponse struct {
	Object         string  `json:"object"`
	TotalGranted   float64 `json:"total_granted"`
	TotalUsed      float64 `json:"total_used"`
	TotalRemaining float64 `json:"total_remaining"`
}

type APGC2DGPTUsageResponse struct {
	//Grants         interface{} `json:"grants"`
	Object         string  `json:"object"`
	TotalAvailable float64 `json:"total_available"`
	TotalGranted   float64 `json:"total_granted"`
	TotalUsed      float64 `json:"total_used"`
}

type SiliconFlowUsageResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  bool   `json:"status"`
	Data    struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Image         string `json:"image"`
		Email         string `json:"email"`
		IsAdmin       bool   `json:"isAdmin"`
		Balance       string `json:"balance"`
		Status        string `json:"status"`
		Introduction  string `json:"introduction"`
		Role          string `json:"role"`
		ChargeBalance string `json:"chargeBalance"`
		TotalBalance  string `json:"totalBalance"`
		Category      string `json:"category"`
	} `json:"data"`
}

type DeepSeekUsageResponse struct {
	IsAvailable  bool `json:"is_available"`
	BalanceInfos []struct {
		Currency        string `json:"currency"`
		TotalBalance    string `json:"total_balance"`
		GrantedBalance  string `json:"granted_balance"`
		ToppedUpBalance string `json:"topped_up_balance"`
	} `json:"balance_infos"`
}

type OpenRouterCreditResponse struct {
	Data struct {
		TotalCredits float64 `json:"total_credits"`
		TotalUsage   float64 `json:"total_usage"`
	} `json:"data"`
}

// GetAuthHeader get auth header
func GetAuthHeader(token string) http.Header {
	h := http.Header{}
	h.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	return h
}

// GetClaudeAuthHeader get claude auth header
func GetClaudeAuthHeader(token string) http.Header {
	h := http.Header{}
	h.Add("x-api-key", token)
	h.Add("anthropic-version", "2023-06-01")
	return h
}

func GetResponseBody(method, url string, channel *model.Channel, headers http.Header) ([]byte, error) {
	return getResponseBodyContext(context.Background(), method, url, channel, headers)
}

func getResponseBodyContext(ctx context.Context, method, target string, ch *model.Channel, headers http.Header) ([]byte, error) {
	if ch == nil {
		return nil, errors.New("余额查询渠道为空。")
	}
	return service.ReadManagementResponseContext(ctx, method, target, ch.GetSetting().Proxy, headers)
}
func getDeepSeekBalanceUSD(response DeepSeekUsageResponse, usdExchangeRate float64) (float64, error) {
	var usdBalance *string
	var cnyBalance *string

	for i := range response.BalanceInfos {
		balanceInfo := &response.BalanceInfos[i]
		switch balanceInfo.Currency {
		case "USD":
			if usdBalance == nil {
				usdBalance = &balanceInfo.TotalBalance
			}
		case "CNY":
			if cnyBalance == nil {
				cnyBalance = &balanceInfo.TotalBalance
			}
		}
	}

	if usdBalance != nil {
		balance, err := strconv.ParseFloat(*usdBalance, 64)
		if err != nil {
			return 0, err
		}
		if math.IsNaN(balance) || math.IsInf(balance, 0) {
			return 0, errors.New("USD balance must be finite")
		}
		if balance < 0 {
			return 0, errors.New("USD balance must be non-negative")
		}
		return balance, nil
	}
	if cnyBalance == nil {
		return 0, errors.New("currency USD or CNY not found")
	}
	if math.IsNaN(usdExchangeRate) || math.IsInf(usdExchangeRate, 0) {
		return 0, errors.New("USD exchange rate must be finite")
	}
	if usdExchangeRate <= 0 {
		return 0, errors.New("USD exchange rate must be greater than zero")
	}

	balanceCNY, err := strconv.ParseFloat(*cnyBalance, 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(balanceCNY) || math.IsInf(balanceCNY, 0) {
		return 0, errors.New("CNY balance must be finite")
	}
	if balanceCNY < 0 {
		return 0, errors.New("CNY balance must be non-negative")
	}
	balanceUSD := decimal.NewFromFloat(balanceCNY).Div(decimal.NewFromFloat(usdExchangeRate)).InexactFloat64()
	if math.IsNaN(balanceUSD) || math.IsInf(balanceUSD, 0) {
		return 0, errors.New("converted USD balance must be finite")
	}
	if balanceUSD < 0 {
		return 0, errors.New("converted USD balance must be non-negative")
	}
	return balanceUSD, nil
}

func fetchAdvancedCustomBalance(channel *model.Channel) (channelBalanceResult, error) {
	return fetchAdvancedCustomBalanceContext(context.Background(), channel)
}

func fetchAdvancedCustomBalanceContext(ctx context.Context, channel *model.Channel) (channelBalanceResult, error) {
	key := strings.TrimSpace(channel.Key)
	info := &relaycommon.RelayInfo{
		RelayFormat:    types.RelayFormatOpenAI,
		RelayMode:      relayconstant.RelayModeUnknown,
		RequestURLPath: dto.AdvancedCustomBalancePath,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:          constant.ChannelTypeAdvancedCustom,
			ChannelBaseUrl:       channel.GetBaseURL(),
			ApiKey:               key,
			ChannelOtherSettings: channel.GetOtherSettings(),
		},
	}
	requestURL, headers, err := (&advancedcustom.Adaptor{}).BuildBalanceRequest(info)
	if err != nil {
		return channelBalanceResult{}, sanitizeFetchModelsError(err, key)
	}
	if err := applyFetchModelsHeaderOverrides(channel, key, headers); err != nil {
		return channelBalanceResult{}, sanitizeFetchModelsError(err, key)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return channelBalanceResult{}, sanitizeFetchModelsError(err, key)
	}
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
		if strings.EqualFold(name, "Host") {
			request.Host = headers.Get(name)
		}
	}
	client, err := service.GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return channelBalanceResult{}, sanitizeFetchModelsError(err, key)
	}
	response, err := client.Do(request)
	if err != nil {
		return channelBalanceResult{}, sanitizeAdvancedCustomRequestError(err, key, requestURL)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return channelBalanceResult{}, fmt.Errorf("status code: %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxAdvancedCustomBalanceResponseBytes+1))
	if err != nil {
		return channelBalanceResult{}, sanitizeAdvancedCustomRequestError(err, key, requestURL)
	}
	if len(body) > maxAdvancedCustomBalanceResponseBytes {
		return channelBalanceResult{}, fmt.Errorf("balance response exceeds %d bytes", maxAdvancedCustomBalanceResponseBytes)
	}

	var validated json.RawMessage
	if err := common.Unmarshal(body, &validated); err != nil {
		return channelBalanceResult{}, fmt.Errorf("invalid balance JSON response: %w", err)
	}
	if common.GetJsonType(validated) == "object" {
		var creditSummary struct {
			Object         string          `json:"object"`
			TotalAvailable json.RawMessage `json:"total_available"`
		}
		if err := common.Unmarshal(body, &creditSummary); err != nil {
			return channelBalanceResult{}, fmt.Errorf("invalid balance JSON response: %w", err)
		}
		if creditSummary.Object == "credit_summary" &&
			common.GetJsonType(creditSummary.TotalAvailable) == "number" {
			var balance float64
			if err := common.Unmarshal(creditSummary.TotalAvailable, &balance); err == nil &&
				balance >= 0 &&
				!math.IsNaN(balance) &&
				!math.IsInf(balance, 0) {
				channel.UpdateBalance(balance)
				return channelBalanceResult{Balance: balance}, nil
			}
		}
	}

	formatted, err := common.IndentJson(body)
	if err != nil {
		return channelBalanceResult{}, fmt.Errorf("invalid balance JSON response: %w", err)
	}
	return channelBalanceResult{RawResponse: string(formatted)}, nil
}

func updateChannelBalance(channel *model.Channel) (channelBalanceResult, error) {
	return updateChannelBalanceContext(context.Background(), channel)
}

func updateChannelBalanceContext(ctx context.Context, channel *model.Channel) (channelBalanceResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if channel == nil {
		return channelBalanceResult{}, errors.New("余额查询渠道为空。")
	}
	ctx, cancel := context.WithTimeout(ctx, channelBalanceRequestTimeout)
	defer cancel()
	snapshot := *channel
	channel = &snapshot
	if channel.Type == constant.ChannelTypeAdvancedCustom {
		return fetchAdvancedCustomBalanceContext(ctx, channel)
	}
	balance, err := updateStandardChannelBalance(ctx, channel)
	return channelBalanceResult{Balance: balance}, err
}

func updateStandardChannelBalance(ctx context.Context, ch *model.Channel) (float64, error) {
	if ch == nil {
		return 0, errors.New("余额查询渠道为空。")
	}
	reader := gatewayBalanceReaders[ch.Type]
	if reader == nil {
		return 0, errors.New("此渠道暂不提供余额查询。")
	}
	amount, err := reader(ctx, ch)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, errors.New("渠道余额必须为有限数值。")
	}
	if ch.Id > 0 {
		timestamp := common.GetTimestamp()
		result := model.DB.WithContext(ctx).Model(&model.Channel{}).Where("id = ?", ch.Id).Updates(map[string]any{"balance": amount, "balance_updated_time": timestamp})
		if result.Error != nil {
			return 0, result.Error
		}
		ch.Balance, ch.BalanceUpdatedTime = amount, timestamp
	}
	return amount, nil
}
func UpdateChannelBalance(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.CacheGetChannel(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if channel.Type == constant.ChannelTypeTaskPlugin {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Task Plugin channels do not support balance queries"})
		return
	}
	if channel.ChannelInfo.IsMultiKey {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "多密钥渠道不支持余额查询",
		})
		return
	}
	result, err := updateChannelBalanceContext(c.Request.Context(), channel)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response := gin.H{
		"success": true,
		"message": "",
	}
	if result.RawResponse == "" {
		response["balance"] = result.Balance
	} else {
		response["raw_response"] = result.RawResponse
	}
	c.JSON(http.StatusOK, response)
}

func updateAllChannelsBalance() error {
	return updateAllChannelsBalanceContext(context.Background())
}

func updateAllChannelsBalanceContext(ctx context.Context) error {
	_, err := runChannelBalanceCycle(ctx)
	return err
}
func UpdateAllChannelsBalance(c *gin.Context) {
	summary, err := runChannelBalanceCycle(c.Request.Context())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": summary})
}
func AutomaticallyUpdateChannels(frequency int) {
	if frequency <= 0 || int64(frequency) > math.MaxInt64/int64(time.Minute) {
		return
	}
	ctx := common.BackgroundContext()
	ticker := time.NewTicker(time.Duration(frequency) * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if _, err := runChannelBalanceCycle(ctx); err != nil && ctx.Err() == nil {
			common.SysError("渠道余额刷新失败：" + err.Error())
		}
	}
}
func waitChannelBalanceInterval(ctx context.Context, interval time.Duration) bool {
	if ctx.Err() != nil {
		return false
	}
	if interval <= 0 {
		return true
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return ctx.Err() == nil
	}
}
