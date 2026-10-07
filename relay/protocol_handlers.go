package relay

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func settleConversationalUsage(c *gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage, configuredAudio bool) {
	audio := usage.CompletionTokenDetails.AudioTokens > 0 || usage.PromptTokensDetails.AudioTokens > 0
	if configuredAudio {
		audio = audio && (ratio_setting.ContainsAudioRatio(info.OriginModelName) || ratio_setting.ContainsAudioCompletionRatio(info.OriginModelName))
	}
	if audio {
		service.PostAudioConsumeQuota(c, info, usage, "")
	} else {
		service.PostTextConsumeQuota(c, info, usage, nil)
	}
}

func TextHelper(c *gin.Context, info *relaycommon.RelayInfo) *types.GatewayError {
	attempt, err := beginProtocolAttempt(c, info)
	if err != nil {
		return err
	}
	request, err := cloneProtocolRequest[*dto.GeneralOpenAIRequest](attempt, true)
	if err != nil {
		return err
	}
	if request.WebSearchOptions != nil {
		c.Set("chat_completion_web_search_context_size", request.WebSearchOptions.SearchContextSize)
	}
	info.ShouldIncludeUsage = request.StreamOptions == nil || request.StreamOptions.IncludeUsage
	streaming := request.Stream != nil && *request.Stream
	switch {
	case !info.SupportStreamOptions || !streaming:
		request.StreamOptions = nil
	case constant.ForceStreamOption:
		request.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
	}
	if err := attempt.bindAdaptor(); err != nil {
		return err
	}
	if !attempt.passThrough {
		applySystemPromptIfNeeded(c, info, request)
	}
	if info.RelayMode == relayconstant.RelayModeChatCompletions && !attempt.passThrough &&
		service.ShouldChatCompletionsUseResponsesGlobal(info.ChannelId, info.ChannelType, info.OriginModelName) {
		usage, err := textRequestViaResponses(c, info, attempt.adaptor, request)
		if err != nil {
			return err
		}
		settleConversationalUsage(c, info, usage, true)
		return nil
	}
	body, err := attempt.convertedBody(true, func() (any, error) { return attempt.adaptor.ConvertOpenAIRequest(c, info, request) }, nil)
	if err != nil {
		return err
	}
	defer body.Close()
	usage, err := attempt.perform(body.reader)
	if err != nil {
		return err
	}
	settleConversationalUsage(c, info, usage, true)
	return nil
}

func injectClaudeSystem(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) {
	prompt := info.ChannelSetting.SystemPrompt
	if prompt == "" {
		return
	}
	if request.System == nil {
		request.SetStringSystem(prompt)
		return
	}
	if !info.ChannelSetting.SystemPromptOverride {
		return
	}
	common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
	if request.IsStringSystem() {
		existing := strings.TrimSpace(request.GetStringSystem())
		if existing != "" {
			prompt += "\n" + existing
		}
		request.SetStringSystem(prompt)
		return
	}
	first := dto.ClaudeMediaMessage{Type: dto.ContentTypeText}
	first.SetText(prompt)
	request.System = append([]dto.ClaudeMediaMessage{first}, request.ParseSystem()...)
}

func ClaudeHelper(c *gin.Context, info *relaycommon.RelayInfo) *types.GatewayError {
	attempt, err := beginProtocolAttempt(c, info)
	if err != nil {
		return err
	}
	request, err := cloneProtocolRequest[*dto.ClaudeRequest](attempt, true)
	if err != nil {
		return err
	}
	if err := attempt.bindAdaptor(); err != nil {
		return err
	}
	if !attempt.passThrough {
		injectClaudeSystem(c, info, request)
	}
	if !attempt.passThrough && service.ShouldChatCompletionsUseResponsesGlobal(info.ChannelId, info.ChannelType, info.OriginModelName) {
		usage, err := textRequestViaResponses(c, info, attempt.adaptor, request)
		if err != nil {
			return err
		}
		service.PostTextConsumeQuota(c, info, usage, nil)
		return nil
	}
	body, err := attempt.convertedBody(true, func() (any, error) { return attempt.adaptor.ConvertClaudeRequest(c, info, request) }, nil)
	if err != nil {
		return err
	}
	defer body.Close()
	usage, err := attempt.perform(body.reader)
	if err != nil {
		return err
	}
	service.PostTextConsumeQuota(c, info, usage, nil)
	return nil
}

func injectGeminiSystem(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) {
	prompt := info.ChannelSetting.SystemPrompt
	if prompt == "" {
		return
	}
	if request.SystemInstructions == nil {
		request.SystemInstructions = &dto.GeminiChatContent{}
	}
	parts := request.SystemInstructions.Parts
	if len(parts) == 0 {
		request.SystemInstructions.Parts = []dto.GeminiPart{{Text: prompt}}
		return
	}
	if !info.ChannelSetting.SystemPromptOverride {
		return
	}
	common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
	for index := range parts {
		if parts[index].Text != "" {
			parts[index].Text = prompt + "\n" + parts[index].Text
			return
		}
	}
	request.SystemInstructions.Parts = append([]dto.GeminiPart{{Text: prompt}}, parts...)
}

func GeminiHelper(c *gin.Context, info *relaycommon.RelayInfo) *types.GatewayError {
	attempt, err := beginProtocolAttempt(c, info)
	if err != nil {
		return err
	}
	request, err := cloneProtocolRequest[*dto.GeminiChatRequest](attempt, true)
	if err != nil {
		return err
	}
	if err := attempt.bindAdaptor(); err != nil {
		return err
	}
	if !attempt.passThrough {
		injectGeminiSystem(c, info, request)
	}
	body, err := attempt.convertedBody(false, func() (any, error) { return attempt.adaptor.ConvertGeminiRequest(c, info, request) }, nil)
	if err != nil {
		return err
	}
	defer body.Close()
	usage, err := attempt.perform(body.reader)
	if err != nil {
		return err
	}
	service.PostTextConsumeQuota(c, info, usage, nil)
	return nil
}

func EmbeddingHelper(c *gin.Context, info *relaycommon.RelayInfo) *types.GatewayError {
	attempt, err := beginProtocolAttempt(c, info)
	if err != nil {
		return err
	}
	request, err := cloneProtocolRequest[*dto.EmbeddingRequest](attempt, false)
	if err != nil {
		return err
	}
	if err := attempt.bindAdaptor(); err != nil {
		return err
	}
	body, err := attempt.convertedBody(false, func() (any, error) { return attempt.adaptor.ConvertEmbeddingRequest(c, info, *request) }, nil)
	if err != nil {
		return err
	}
	defer body.Close()
	usage, err := attempt.perform(body.reader)
	if err != nil {
		return err
	}
	service.PostTextConsumeQuota(c, info, usage, nil)
	return nil
}

func RerankHelper(c *gin.Context, info *relaycommon.RelayInfo) *types.GatewayError {
	attempt, err := beginProtocolAttempt(c, info)
	if err != nil {
		return err
	}
	request, err := cloneProtocolRequest[*dto.RerankRequest](attempt, false)
	if err != nil {
		return err
	}
	if err := attempt.bindAdaptor(); err != nil {
		return err
	}
	body, err := attempt.convertedBody(false, func() (any, error) { return attempt.adaptor.ConvertRerankRequest(c, info.RelayMode, *request) }, nil)
	if err != nil {
		return err
	}
	defer body.Close()
	usage, err := attempt.perform(body.reader)
	if err != nil {
		return err
	}
	service.PostTextConsumeQuota(c, info, usage, nil)
	return nil
}

func AudioHelper(c *gin.Context, info *relaycommon.RelayInfo) *types.GatewayError {
	attempt, err := beginProtocolAttempt(c, info)
	if err != nil {
		return err
	}
	request, err := cloneProtocolRequest[*dto.AudioRequest](attempt, false)
	if err != nil {
		return err
	}
	if err := attempt.bindAdaptor(); err != nil {
		return err
	}
	body, err := attempt.convertedBody(false, func() (any, error) {
		return attempt.adaptor.ConvertAudioRequest(c, info, *request)
	}, nil)
	if err != nil {
		return err
	}
	defer body.Close()
	usage, err := attempt.perform(body.reader)
	if err != nil {
		return err
	}
	settleConversationalUsage(c, info, usage, false)
	return nil
}

func imageQuantityError(err error) *types.GatewayError {
	return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

func ImageHelper(c *gin.Context, info *relaycommon.RelayInfo) *types.GatewayError {
	attempt, err := beginProtocolAttempt(c, info)
	if err != nil {
		return err
	}
	info.BillingImageCount, info.ImageRequestCount = nil, 0
	request, err := cloneProtocolRequest[*dto.ImageRequest](attempt, false)
	if err != nil {
		return err
	}
	quantity, countErr := request.ImageCount(false)
	if countErr != nil {
		return imageQuantityError(countErr)
	}
	if err := attempt.bindAdaptor(); err != nil {
		return err
	}
	validateQuantity := func(data []byte) *types.GatewayError {
		var final struct{ N *uint }
		if err := common.Unmarshal(data, &final); err != nil {
			return imageQuantityError(fmt.Errorf("生成数量参数无效：%w", err))
		}
		if final.N == nil {
			return nil
		}
		resolved, err := (&dto.ImageRequest{N: final.N}).ImageCount(false)
		if err != nil {
			return imageQuantityError(err)
		}
		quantity = resolved
		return nil
	}
	var body attemptBody
	if attempt.passThrough {
		storage, readErr := common.GetBodyStorage(c)
		if readErr != nil {
			return imageQuantityError(readErr)
		}
		if !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "multipart/form-data") {
			data, readErr := storage.Bytes()
			if readErr != nil {
				return imageQuantityError(readErr)
			}
			if err := validateQuantity(data); err != nil {
				return err
			}
		}
		body = attemptBody{reader: common.NewReplayableBodyReader(storage)}
	} else {
		body, err = attempt.convertedBody(false, func() (any, error) { return attempt.adaptor.ConvertImageRequest(c, info, *request) }, validateQuantity)
		if err != nil {
			return err
		}
	}
	defer body.Close()
	if err := service.PrepareImageBillingForRequest(c, info, quantity); err != nil {
		return err
	}
	attempt.acceptCreated = true
	usage, err := attempt.perform(body.reader)
	if err != nil {
		return err
	}
	usage.TotalTokens, usage.PromptTokens = max(1, usage.TotalTokens), max(1, usage.PromptTokens)
	count := info.RequestedImageCount()
	if info.BillingImageCount != nil {
		count = *info.BillingImageCount
	}
	quality := request.Quality
	if quality == "" {
		quality = "standard"
	}
	details := []string{fmt.Sprintf("品质 %s", quality), fmt.Sprintf("生成数量 %d", count)}
	if request.Size != "" {
		details = append([]string{fmt.Sprintf("大小 %s", request.Size)}, details...)
	}
	service.PostTextConsumeQuota(c, info, usage, details)
	return nil
}

func GeminiEmbeddingHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.GatewayError {
	attempt, err := beginProtocolAttempt(c, info)
	if err != nil {
		return err
	}
	info.IsGeminiBatchEmbedding = strings.HasSuffix(c.Request.URL.Path, "batchEmbedContents")
	var request dto.Request
	if info.IsGeminiBatchEmbedding {
		request = &dto.GeminiBatchEmbeddingRequest{}
	} else {
		request = &dto.GeminiEmbeddingRequest{}
	}
	if err := common.UnmarshalBodyReusable(c, request); err != nil {
		return imageQuantityError(err)
	}
	if err := helper.ModelMappedHelper(c, info, request); err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	if err := helper.ApplyReasoningModelSuffix(c, info, request); err != nil {
		return newConvertRequestFailedError(c, info, err)
	}
	request.SetModelName("models/" + info.UpstreamModelName)
	if err := attempt.bindAdaptor(); err != nil {
		return err
	}
	body, err := attempt.jsonBody(request, false, nil)
	if err != nil {
		return err
	}
	defer body.Close()
	usage, err := attempt.perform(body.reader)
	if err != nil {
		return err
	}
	service.PostTextConsumeQuota(c, info, usage, nil)
	return nil
}

func ResponsesHelper(c *gin.Context, info *relaycommon.RelayInfo) *types.GatewayError {
	if err := protocolCancellation(c); err != nil {
		return err
	}
	if info == nil {
		return imageQuantityError(errors.New("Responses 请求信息缺失。"))
	}
	var request *dto.OpenAIResponsesRequest
	switch source := info.Request.(type) {
	case *dto.OpenAIResponsesRequest:
		request = source
	case *dto.OpenAIResponsesCompactionRequest:
		if source != nil {
			request = &dto.OpenAIResponsesRequest{
				Model: source.Model, Input: source.Input, Instructions: source.Instructions,
				PreviousResponseID: source.PreviousResponseID, ParallelToolCalls: source.ParallelToolCalls,
				ServiceTier: source.ServiceTier, PromptCacheKey: source.PromptCacheKey,
				PromptCacheOptions: source.PromptCacheOptions, PromptCacheRetention: source.PromptCacheRetention,
			}
		}
	}
	if request == nil {
		return imageQuantityError(fmt.Errorf("Responses 请求类型无效：%T。", info.Request))
	}
	adaptor, reader, owner, err := PrepareResponsesRequest(c, info, request)
	if err != nil {
		return err
	}
	defer owner.Close()
	attempt := &protocolAttempt{ctx: c, info: info, adaptor: adaptor}
	usage, err := attempt.perform(reader)
	if err != nil {
		return err
	}
	if info.RelayMode != relayconstant.RelayModeResponsesCompact {
		ConsumeResponsesQuota(c, info, usage)
		return nil
	}
	model, price := info.OriginModelName, info.PriceData
	defer func() { info.OriginModelName, info.PriceData = model, price }()
	if _, err := helper.ModelPriceHelper(c, info, info.GetEstimatePromptTokens(), &types.TokenCountMeta{}); err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	service.PostTextConsumeQuota(c, info, usage, nil)
	return nil
}

func ConsumeResponsesQuota(c *gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage) {
	if usage == nil {
		usage = &dto.Usage{}
	}
	if strings.HasPrefix(info.OriginModelName, "gpt-4o-audio") {
		service.PostAudioConsumeQuota(c, info, usage, "")
	} else {
		service.PostTextConsumeQuota(c, info, usage, nil)
	}
}
