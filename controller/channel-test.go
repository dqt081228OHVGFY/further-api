package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/pkg/billingexpr"
	"github.com/dqt081228OHVGFY/further-api/relay"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	hosttypes "github.com/dqt081228OHVGFY/further-api/types"

	"github.com/samber/lo"
	"github.com/tidwall/gjson"

	"github.com/gin-gonic/gin"
)

type testResult struct {
	context      *gin.Context
	localErr     error
	gatewayError *types.GatewayError
}

func normalizeChannelTestEndpoint(channel *model.Channel, endpointType string) string {
	normalized := strings.TrimSpace(endpointType)
	if normalized != "" {
		return normalized
	}
	if channel != nil && channel.Type == constant.ChannelTypeCodex {
		return string(constant.EndpointTypeOpenAIResponse)
	}
	return normalized
}

func resolveChannelTestUserID(c *gin.Context) (int, error) {
	if c != nil {
		if userID := c.GetInt("id"); userID > 0 {
			return userID, nil
		}
	}

	var rootUser model.User
	if err := model.DB.Select("id").Where("role = ?", common.RoleRootUser).First(&rootUser).Error; err != nil {
		return 0, fmt.Errorf("failed to resolve channel test user: %w", err)
	}
	if rootUser.Id == 0 {
		return 0, errors.New("failed to resolve channel test user")
	}
	return rootUser.Id, nil
}

func testChannel(ctx context.Context, ch *model.Channel, userID int, name, endpoint string, stream bool) testResult {
	if ctx == nil {
		ctx = context.Background()
	}
	failed := func(c *gin.Context, err error, code types.ErrorCode) testResult {
		classified := types.NewError(err, code, types.ErrOptionWithSkipRetry())
		var upstream *types.GatewayError
		if errors.As(err, &upstream) {
			classified = upstream
		}
		return testResult{context: c, localErr: err, gatewayError: classified}
	}
	if err := ctx.Err(); err != nil {
		return failed(nil, err, types.ErrorCodeDoRequestFailed)
	}
	if ch == nil || userID <= 0 {
		return failed(nil, errors.New("渠道探测上下文无效。"), types.ErrorCodeInvalidRequest)
	}
	switch ch.Type {
	case constant.ChannelTypeMidjourney, constant.ChannelTypeMidjourneyPlus, constant.ChannelTypeSunoAPI,
		constant.ChannelTypeKling, constant.ChannelTypeJimeng, constant.ChannelTypeDoubaoVideo, constant.ChannelTypeVidu, constant.ChannelTypeTaskPlugin:
		return failed(nil, fmt.Errorf("%s channel test is not supported", constant.GetChannelTypeName(ch.Type)), types.ErrorCodeInvalidApiType)
	}
	name = strings.TrimSpace(name)
	if name == "" && ch.TestModel != nil {
		name = strings.TrimSpace(*ch.TestModel)
	}
	if name == "" {
		for _, candidate := range ch.GetModels() {
			if name = strings.TrimSpace(candidate); name != "" {
				break
			}
		}
	}
	if name == "" {
		name = "gpt-4o-mini"
	}
	endpoint = normalizeChannelTestEndpoint(ch, endpoint)
	path, format := channelProbeRoute(ch, name, endpoint, stream)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequestWithContext(ctx, http.MethodPost, path, nil)
	c.Request.Header.Set("Content-Type", "application/json")
	owner, err := model.GetUserCacheContext(ctx, userID)
	if err != nil {
		return failed(c, err, types.ErrorCodeQueryDataError)
	}
	owner.WriteContext(c)
	c.Set("id", userID)
	c.Set("channel", ch.Type)
	c.Set("base_url", ch.GetBaseURL())
	c.Set("group", owner.Group)
	if err := middleware.SetupContextForSelectedChannel(c, ch, name); err != nil {
		return failed(c, err, types.ErrorCodeInvalidRequest)
	}
	request := buildTestRequest(name, endpoint, ch, stream)
	info, err := relaycommon.GenRelayInfo(c, format, request, nil)
	if err != nil {
		return failed(c, err, types.ErrorCodeGenRelayInfoFailed)
	}
	info.IsChannelTest = true
	info.InitChannelMeta(c)
	if err := attachTestBillingRequestInput(info, request); err != nil {
		return failed(c, err, types.ErrorCodeJsonMarshalFailed)
	}
	if err := helper.ModelMappedHelper(c, info, request); err != nil {
		return failed(c, err, types.ErrorCodeChannelModelMappedError)
	}
	if err := helper.ApplyReasoningModelSuffix(c, info, request); err != nil {
		return failed(c, err, types.ErrorCodeConvertRequestFailed)
	}
	if info.RelayMode == relayconstant.RelayModeResponsesCompact && !common.SupportsResponsesCompact(ch.Type, info.ApiType) {
		return failed(c, errors.New("此渠道不支持Responses压缩探测。"), types.ErrorCodeInvalidApiType)
	}
	price, err := helper.ModelPriceHelper(c, info, 0, request.GetTokenCountMeta())
	if err != nil {
		return failed(c, err, types.ErrorCodeModelPriceError)
	}
	started := time.Now()
	usage, failure := relay.ExecuteChannelProbe(c, info, request)
	if failure != nil {
		return failed(c, failure, types.ErrorCodeDoRequestFailed)
	}
	usage, err = coerceTestUsage(usage, stream, info.GetEstimatePromptTokens())
	if err != nil {
		return failed(c, err, types.ErrorCodeBadResponseBody)
	}
	response := writer.Result()
	body, err := readTestResponseBody(response.Body, stream)
	if err != nil {
		return failed(c, err, types.ErrorCodeReadResponseBodyFailed)
	}
	if err := validateTestResponseBody(body, stream); err != nil {
		return failed(c, err, types.ErrorCodeBadResponseBody)
	}
	info.SetEstimatePromptTokens(usage.PromptTokens)
	quota, expression := settleTestQuota(info, price, usage)
	metadata := buildTestLogOther(c, info, price, usage, expression)
	model.RecordConsumeLog(c, userID, model.RecordConsumeLogParams{ChannelId: ch.Id, PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens,
		ModelName: info.OriginModelName, TokenName: "模型测试", Quota: quota, Content: "模型测试", UseTimeSeconds: int(time.Since(started).Seconds()),
		IsStream: info.IsStream, Group: info.UsingGroup, Other: metadata})
	return testResult{context: c}
}

func channelProbeRoute(ch *model.Channel, name, endpoint string, stream bool) (string, types.RelayFormat) {
	path, format := "/v1/chat/completions", types.RelayFormatOpenAI
	endpoints := map[constant.EndpointType]types.RelayFormat{
		constant.EndpointTypeOpenAI: types.RelayFormatOpenAI, constant.EndpointTypeOpenAIResponse: types.RelayFormatOpenAIResponses,
		constant.EndpointTypeOpenAIResponseCompact: types.RelayFormatOpenAIResponsesCompaction, constant.EndpointTypeAnthropic: types.RelayFormatClaude,
		constant.EndpointTypeGemini: types.RelayFormatGemini, constant.EndpointTypeJinaRerank: types.RelayFormatRerank,
		constant.EndpointTypeImageGeneration: types.RelayFormatOpenAIImage, constant.EndpointTypeEmbeddings: types.RelayFormatEmbedding,
	}
	if endpoint != "" {
		if info, exists := common.GetDefaultEndpointInfo(constant.EndpointType(endpoint)); exists {
			path = info.Path
		}
		if requested, exists := endpoints[constant.EndpointType(endpoint)]; exists {
			format = requested
		}
	} else {
		lower := strings.ToLower(name)
		switch {
		case strings.Contains(lower, "codex"):
			path, format = "/v1/responses", types.RelayFormatOpenAIResponses
		case ch.Type == constant.ChannelTypeVolcEngine && strings.Contains(lower, "seedream"):
			path, format = "/v1/images/generations", types.RelayFormatOpenAIImage
		case strings.Contains(lower, "embedding") || strings.HasPrefix(lower, "m3e") || strings.Contains(lower, "bge-") || strings.Contains(lower, "embed") || ch.Type == constant.ChannelTypeMokaAI:
			path, format = "/v1/embeddings", types.RelayFormatEmbedding
		case strings.Contains(lower, "rerank"):
			path, format = "/v1/rerank", types.RelayFormatRerank
		}
	}
	if stream && format == types.RelayFormatGemini {
		path = strings.Replace(path, ":generateContent", ":streamGenerateContent", 1)
	}
	return path, format
}
func attachTestBillingRequestInput(info *relaycommon.RelayInfo, request dto.Request) error {
	if info == nil {
		return nil
	}

	input, err := helper.BuildBillingExprRequestInputFromRequest(request, info.RequestHeaders)
	if err != nil {
		return err
	}
	info.BillingRequestInput = &input
	return nil
}

func settleTestQuota(info *relaycommon.RelayInfo, priceData hosttypes.PriceData, usage *dto.Usage) (int, *billingexpr.TieredResult) {
	if usage != nil && info != nil && info.TieredBillingSnapshot != nil {
		isClaudeUsageSemantic := usage.UsageSemantic == "anthropic" || info.GetFinalRequestRelayFormat() == types.RelayFormatClaude
		usedVars := billingexpr.UsedVars(info.TieredBillingSnapshot.ExprString)
		if ok, quota, result := service.TryTieredSettle(info, service.BuildTieredTokenParams(usage, isClaudeUsageSemantic, usedVars)); ok {
			return quota, result
		}
	}

	quota := 0
	if !priceData.UsePrice {
		completionQuota := common.QuotaRound(float64(usage.CompletionTokens) * priceData.CompletionRatio)
		quota = common.QuotaRound(float64(usage.PromptTokens) + float64(completionQuota))
		quota = common.QuotaRound(float64(quota) * priceData.ModelRatio)
		if priceData.ModelRatio != 0 && quota <= 0 {
			quota = 1
		}
		return quota, nil
	}

	return common.QuotaFromFloat(priceData.ModelPrice * common.QuotaPerUnit), nil
}

func buildTestLogOther(c *gin.Context, info *relaycommon.RelayInfo, priceData hosttypes.PriceData, usage *dto.Usage, tieredResult *billingexpr.TieredResult) *model.LogOther {
	other := service.GenerateTextOtherInfo(c, info, priceData.ModelRatio, priceData.GroupRatioInfo.GroupRatio, priceData.CompletionRatio,
		usage.PromptTokensDetails.CachedTokens, priceData.CacheRatio, priceData.ModelPrice, priceData.GroupRatioInfo.GroupSpecialRatio)
	if tieredResult != nil {
		service.InjectTieredBillingInfo(other, info, tieredResult)
	}
	return other
}

func coerceTestUsage(usageAny any, isStream bool, estimatePromptTokens int) (*dto.Usage, error) {
	switch u := usageAny.(type) {
	case *dto.Usage:
		return u, nil
	case dto.Usage:
		return &u, nil
	case nil:
		if !isStream {
			return nil, errors.New("usage is nil")
		}
		usage := &dto.Usage{
			PromptTokens: estimatePromptTokens,
		}
		usage.TotalTokens = usage.PromptTokens
		return usage, nil
	default:
		if !isStream {
			return nil, fmt.Errorf("invalid usage type: %T", usageAny)
		}
		usage := &dto.Usage{
			PromptTokens: estimatePromptTokens,
		}
		usage.TotalTokens = usage.PromptTokens
		return usage, nil
	}
}

func readTestResponseBody(body io.ReadCloser, isStream bool) ([]byte, error) {
	defer func() { _ = body.Close() }()
	const maxStreamLogBytes = 8 << 10
	if isStream {
		return io.ReadAll(io.LimitReader(body, maxStreamLogBytes))
	}
	return io.ReadAll(body)
}

func detectErrorFromTestResponseBody(respBody []byte) error {
	b := bytes.TrimSpace(respBody)
	if len(b) == 0 {
		return nil
	}
	if message := detectErrorMessageFromJSONBytes(b); message != "" {
		return fmt.Errorf("upstream error: %s", message)
	}

	for line := range bytes.SplitSeq(b, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		if message := detectErrorMessageFromJSONBytes(payload); message != "" {
			return fmt.Errorf("upstream error: %s", message)
		}
	}

	return nil
}

func validateStreamTestResponseBody(respBody []byte) error {
	b := bytes.TrimSpace(respBody)
	if len(b) == 0 {
		return errors.New("stream response body is empty")
	}

	for line := range bytes.SplitSeq(b, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}

		return nil
	}

	return errors.New("stream response body does not contain a valid stream event")
}

func validateTestResponseBody(respBody []byte, isStream bool) error {
	if bodyErr := detectErrorFromTestResponseBody(respBody); bodyErr != nil {
		return bodyErr
	}
	if isStream {
		return validateStreamTestResponseBody(respBody)
	}
	return nil
}

func shouldUseStreamForAutomaticChannelTest(channel *model.Channel) bool {
	return channel != nil && channel.Type == constant.ChannelTypeCodex
}

func detectErrorMessageFromJSONBytes(jsonBytes []byte) string {
	if len(jsonBytes) == 0 {
		return ""
	}
	if jsonBytes[0] != '{' && jsonBytes[0] != '[' {
		return ""
	}
	errVal := gjson.GetBytes(jsonBytes, "error")
	if !errVal.Exists() || errVal.Type == gjson.Null {
		return ""
	}

	message := gjson.GetBytes(jsonBytes, "error.message").String()
	if message == "" {
		message = gjson.GetBytes(jsonBytes, "error.error.message").String()
	}
	if message == "" && errVal.Type == gjson.String {
		message = errVal.String()
	}
	if message == "" {
		message = errVal.Raw
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return "upstream returned error payload"
	}
	return message
}

func buildTestRequest(model string, endpointType string, channel *model.Channel, isStream bool) dto.Request {
	testResponsesInput := json.RawMessage(`[{"role":"user","content":"hi"}]`)

	// 根据端点类型构建不同的测试请求
	if endpointType != "" {
		switch constant.EndpointType(endpointType) {
		case constant.EndpointTypeEmbeddings:
			// 返回 EmbeddingRequest
			return &dto.EmbeddingRequest{
				Model: model,
				Input: []any{"hello world"},
			}
		case constant.EndpointTypeImageGeneration:
			// 返回 ImageRequest
			return &dto.ImageRequest{
				Model:  model,
				Prompt: "a cute cat",
				N:      lo.ToPtr(uint(1)),
				Size:   "1024x1024",
			}
		case constant.EndpointTypeJinaRerank:
			// 返回 RerankRequest
			return &dto.RerankRequest{
				Model:     model,
				Query:     "What is Deep Learning?",
				Documents: []any{"Deep Learning is a subset of machine learning.", "Machine learning is a field of artificial intelligence."},
				TopN:      lo.ToPtr(2),
			}
		case constant.EndpointTypeOpenAIResponse:
			// 返回 OpenAIResponsesRequest
			return &dto.OpenAIResponsesRequest{
				Model:  model,
				Input:  json.RawMessage(`[{"role":"user","content":"hi"}]`),
				Stream: lo.ToPtr(isStream),
			}
		case constant.EndpointTypeOpenAIResponseCompact:
			// 返回 OpenAIResponsesCompactionRequest
			return &dto.OpenAIResponsesCompactionRequest{
				Model: model,
				Input: testResponsesInput,
			}
		case constant.EndpointTypeAnthropic:
			return &dto.ClaudeRequest{
				Model:     model,
				Stream:    lo.ToPtr(isStream),
				MaxTokens: lo.ToPtr(uint(16)),
				Messages: []dto.ClaudeMessage{
					{
						Role:    "user",
						Content: "hi",
					},
				},
			}
		case constant.EndpointTypeGemini:
			return &dto.GeminiChatRequest{
				Contents: []dto.GeminiChatContent{
					{
						Role:  "user",
						Parts: []dto.GeminiPart{{Text: "hi"}},
					},
				},
				GenerationConfig: dto.GeminiChatGenerationConfig{
					MaxOutputTokens: lo.ToPtr(uint(3000)),
				},
			}
		case constant.EndpointTypeOpenAI:
			req := &dto.GeneralOpenAIRequest{
				Model:  model,
				Stream: lo.ToPtr(isStream),
				Messages: []dto.Message{
					{
						Role:    "user",
						Content: "hi",
					},
				},
				MaxTokens: lo.ToPtr(uint(16)),
			}
			if isStream {
				req.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
			}
			return req
		}
	}

	// 自动检测逻辑（保持原有行为）
	if strings.Contains(strings.ToLower(model), "rerank") {
		return &dto.RerankRequest{
			Model:     model,
			Query:     "What is Deep Learning?",
			Documents: []any{"Deep Learning is a subset of machine learning.", "Machine learning is a field of artificial intelligence."},
			TopN:      lo.ToPtr(2),
		}
	}

	// 先判断是否为 Embedding 模型
	if strings.Contains(strings.ToLower(model), "embedding") ||
		strings.HasPrefix(model, "m3e") ||
		strings.Contains(model, "bge-") {
		// 返回 EmbeddingRequest
		return &dto.EmbeddingRequest{
			Model: model,
			Input: []any{"hello world"},
		}
	}

	// Responses-only models (e.g. codex series)
	if strings.Contains(strings.ToLower(model), "codex") {
		return &dto.OpenAIResponsesRequest{
			Model:  model,
			Input:  json.RawMessage(`[{"role":"user","content":"hi"}]`),
			Stream: lo.ToPtr(isStream),
		}
	}

	// Chat/Completion 请求 - 返回 GeneralOpenAIRequest
	testRequest := &dto.GeneralOpenAIRequest{
		Model:  model,
		Stream: lo.ToPtr(isStream),
		Messages: []dto.Message{
			{
				Role:    "user",
				Content: "hi",
			},
		},
	}
	if isStream {
		testRequest.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
	}

	if dto.IsOpenAIReasoningOModel(model) {
		testRequest.MaxCompletionTokens = lo.ToPtr(uint(16))
	} else if strings.Contains(model, "thinking") {
		if !strings.Contains(model, "claude") {
			testRequest.MaxTokens = lo.ToPtr(uint(50))
		}
	} else if strings.Contains(model, "gemini") {
		testRequest.MaxTokens = lo.ToPtr(uint(3000))
	} else {
		testRequest.MaxTokens = lo.ToPtr(uint(16))
	}

	return testRequest
}

func TestChannel(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.CacheGetChannel(channelId)
	if err != nil {
		channel, err = model.GetChannelById(channelId, true)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	//defer func() {
	//	if channel.ChannelInfo.IsMultiKey {
	//		go func() { _ = channel.SaveChannelInfo() }()
	//	}
	//}()
	testModel := c.Query("model")
	endpointType := c.Query("endpoint_type")
	isStream, _ := strconv.ParseBool(c.Query("stream"))
	testUserID, err := resolveChannelTestUserID(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tik := time.Now()
	requestCtx := context.Background()
	if c.Request != nil {
		requestCtx = c.Request.Context()
	}
	result := testChannel(requestCtx, channel, testUserID, testModel, endpointType, isStream)
	if result.localErr != nil {
		resp := gin.H{
			"success": false,
			"message": result.localErr.Error(),
			"time":    0.0,
		}
		if result.gatewayError != nil {
			resp["error_code"] = result.gatewayError.GetErrorCode()
		}
		c.JSON(http.StatusOK, resp)
		return
	}
	tok := time.Now()
	milliseconds := tok.Sub(tik).Milliseconds()
	go channel.UpdateResponseTime(milliseconds)
	consumedTime := float64(milliseconds) / 1000.0
	if result.gatewayError != nil {
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"message":    result.gatewayError.Error(),
			"time":       consumedTime,
			"error_code": result.gatewayError.GetErrorCode(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"time":    consumedTime,
	})
}

// channelTestSummary records the outcome of one channel test cycle so the
// system task can persist a per-run result for history.
type channelTestSummary struct {
	Tested    int `json:"tested"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Disabled  int `json:"disabled"`
	Enabled   int `json:"enabled"`
}

func testChannelForHealthCheck(ctx context.Context, channel *model.Channel, testUserID int, allowDisable bool, disableThreshold int64) channelTestSummary {
	summary := channelTestSummary{}
	isChannelEnabled := channel.Status == common.ChannelStatusEnabled
	tik := time.Now()
	result := testChannel(ctx, channel, testUserID, "", "", shouldUseStreamForAutomaticChannelTest(channel))
	milliseconds := time.Since(tik).Milliseconds()
	if ctx.Err() != nil {
		return summary
	}

	summary.Tested++

	shouldBanChannel := false
	gatewayError := result.gatewayError
	if gatewayError != nil {
		shouldBanChannel = service.ShouldDisableChannel(result.gatewayError)
	}

	if common.AutomaticDisableChannelEnabled && !shouldBanChannel {
		if milliseconds > disableThreshold {
			err := fmt.Errorf("响应时间 %.2fs 超过阈值 %.2fs", float64(milliseconds)/1000.0, float64(disableThreshold)/1000.0)
			gatewayError = types.NewOpenAIError(err, types.ErrorCodeChannelResponseTimeExceeded, http.StatusRequestTimeout)
			shouldBanChannel = true
		}
	}

	if gatewayError == nil {
		summary.Succeeded++
	} else {
		summary.Failed++
	}

	if allowDisable && isChannelEnabled && shouldBanChannel && channel.GetAutoBan() {
		processChannelError(result.context, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(result.context, constant.ContextKeyChannelKey), channel.GetAutoBan()), gatewayError, nil)
		summary.Disabled++
	}

	if result.localErr == nil && !isChannelEnabled && service.ShouldEnableChannel(gatewayError, channel.Status) {
		service.EnableChannel(channel.Id, common.GetContextKeyString(result.context, constant.ContextKeyChannelKey), channel.Name)
		summary.Enabled++
	}

	channel.UpdateResponseTime(milliseconds)
	return summary
}

// runChannelTestWorkers executes independent channel tests with bounded
// concurrency. Results and progress are reduced by the caller goroutine, so
// summary counts and the progress reporter remain serialized.
func runChannelTestWorkers(
	ctx context.Context,
	channels []*model.Channel,
	concurrency int,
	run func(context.Context, *model.Channel) channelTestSummary,
	report func(processed, total int),
) channelTestSummary {
	if ctx == nil {
		ctx = context.Background()
	}
	total := len(channels)
	if report != nil {
		report(0, total)
	}
	if total == 0 {
		return channelTestSummary{}
	}

	workerCount := min(operation_setting.NormalizeChannelTestConcurrency(concurrency), total)
	jobs := make(chan *model.Channel)
	results := make(chan channelTestSummary)

	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case channel, ok := <-jobs:
					if !ok {
						return
					}
					if ctx.Err() != nil {
						return
					}

					result := channelTestSummary{}
					if channel != nil && channel.Status != common.ChannelStatusManuallyDisabled {
						result = run(ctx, channel)
					}

					results <- result

					if common.RequestInterval > 0 {
						select {
						case <-ctx.Done():
							return
						case <-time.After(common.RequestInterval):
						}
					}
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, channel := range channels {
			select {
			case <-ctx.Done():
				return
			case jobs <- channel:
			}
		}
	}()

	go func() {
		workers.Wait()
		close(results)
	}()

	summary := channelTestSummary{}
	processed := 0
	for result := range results {
		summary.Tested += result.Tested
		summary.Succeeded += result.Succeeded
		summary.Failed += result.Failed
		summary.Disabled += result.Disabled
		summary.Enabled += result.Enabled
		processed++
		if report != nil && ctx.Err() == nil {
			report(processed, total)
		}
	}
	return summary
}

// performChannelTests runs channel health checks with the configured bounded
// concurrency and honors cancellation when a system-task runner loses its
// lease.
func performChannelTests(ctx context.Context, channels []*model.Channel, testUserID int, allowDisable bool, concurrency int, report func(processed, total int)) channelTestSummary {
	if ctx == nil {
		ctx = context.Background()
	}
	disableThreshold := int64(common.ChannelDisableThreshold * 1000)
	if disableThreshold == 0 {
		disableThreshold = 10000000 // an impossible value
	}
	return runChannelTestWorkers(
		ctx,
		channels,
		concurrency,
		func(ctx context.Context, channel *model.Channel) channelTestSummary {
			return testChannelForHealthCheck(ctx, channel, testUserID, allowDisable, disableThreshold)
		},
		report,
	)
}

// runChannelTestTask runs one synchronous channel test cycle for the system task
// runner (both the scheduled job and the manual "test all channels" trigger go
// through here). It honors ctx cancellation so a runner that loses its lease
// stops promptly. mode selects the channel set: an empty mode falls back to the
// configured monitor ChannelTestMode (scheduled behavior), while a manual
// trigger passes ChannelTestModeScheduledAll to test every channel. When notify
// is set the root user is notified on completion. Cross-instance execution is
// guarded by the system task per-type lock, so no process-local guard is needed.
func runChannelTestTask(ctx context.Context, mode string, notify bool, report func(processed, total int)) (channelTestSummary, error) {
	testUserID, err := resolveChannelTestUserID(nil)
	if err != nil {
		return channelTestSummary{}, err
	}
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return channelTestSummary{}, err
	}
	if strings.TrimSpace(mode) == "" {
		mode = operation_setting.GetMonitorSetting().ChannelTestMode
	}
	selected := selectChannelsForAutomaticTest(channels, mode)
	allowDisable := mode != operation_setting.ChannelTestModePassiveRecovery
	concurrency := operation_setting.GetMonitorSetting().ChannelTestConcurrency
	summary := performChannelTests(ctx, selected, testUserID, allowDisable, concurrency, report)
	if notify && (ctx == nil || ctx.Err() == nil) {
		service.NotifyRootUser(dto.NotifyTypeChannelTest, "通道测试完成", "所有通道测试已完成")
	}
	return summary, nil
}

func selectChannelsForAutomaticTest(channels []*model.Channel, mode string) []*model.Channel {
	selected := make([]*model.Channel, 0, len(channels))
	for _, channel := range channels {
		if channel.Status == common.ChannelStatusManuallyDisabled {
			continue
		}
		if mode == operation_setting.ChannelTestModeAutoBanOnly && !channel.GetAutoBan() {
			continue
		}
		if mode == operation_setting.ChannelTestModePassiveRecovery && channel.Status != common.ChannelStatusAutoDisabled {
			continue
		}
		selected = append(selected, channel)
	}
	return selected
}

// TestAllChannels enqueues a channel_test system task instead of running the
// test loop inline. If any channel_test task is already active, the manual run is
// rejected so the caller does not mistake a scheduled run for this manual one.
func TestAllChannels(c *gin.Context) {
	task, created, err := service.EnqueueSystemTask(model.SystemTaskTypeChannelTest, channelTestTaskPayload{
		Mode:   operation_setting.ChannelTestModeScheduledAll,
		Notify: true,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !created {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "已有通道测试任务正在运行或等待中，不能启动本次手动任务",
			"data": gin.H{
				"task_id": task.TaskID,
				"status":  task.Status,
				"type":    task.Type,
			},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"task_id": task.TaskID,
			"status":  task.Status,
		},
	})
}
