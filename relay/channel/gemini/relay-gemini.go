package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func contentResponseError(err error) *types.GatewayError {
	return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
}
func contentResponseContext(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) error {
	if c == nil || info == nil || !info.HasChannelMeta() || response == nil || response.Body == nil {
		return errors.New("Gemini response context or body is missing")
	}
	return nil
}
func geminiWireError(data []byte) *types.GatewayError {
	var envelope struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.Error == nil {
		return nil
	}
	status := envelope.Error.Code
	if status < 400 || status > 599 {
		status = http.StatusBadGateway
	}
	return types.NewOpenAIError(errors.New(envelope.Error.Message), types.ErrorCodeBadResponseBody, status)
}
func buildUsageFromGeminiMetadata(metadata *dto.GeminiUsageMetadata, prompt int) dto.Usage {
	if usage := relayconvert.UsageFromGeminiMetadata(metadata, prompt); usage != nil {
		return *usage
	}
	return dto.Usage{}
}
func attachEstimatedGeminiBillingUsage(usage *dto.Usage) *dto.Usage {
	if usage != nil && usage.BillingUsage == nil {
		usage.BillingUsage = dto.NewEstimatedGeminiChatBillingUsage(usage)
	}
	return usage
}
func patchGeminiZeroCompletionUsage(c *gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage, text string, images int) {
	if usage == nil || usage.CompletionTokens > 0 || text == "" && images == 0 {
		return
	}
	estimated := service.ResponseText2Usage(c, text, info.UpstreamModelName, usage.PromptTokens)
	usage.CompletionTokens = estimated.CompletionTokens
	if usage.CompletionTokens == 0 && images > 0 {
		usage.CompletionTokens = images * 1400
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	if usage.BillingUsage != nil {
		usage.BillingUsage = dto.CloneBillingUsageWithEstimatedCompletion(usage.BillingUsage, usage.CompletionTokens)
	} else {
		attachEstimatedGeminiBillingUsage(usage)
	}
}
func geminiResponseUsageText(response *dto.GeminiChatResponse) string {
	if response == nil {
		return ""
	}
	var text strings.Builder
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			text.WriteString(part.Text)
		}
	}
	return text.String()
}
func geminiResponseInlineImageCount(response *dto.GeminiChatResponse) int {
	if response == nil {
		return 0
	}
	images := 0
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData != nil && part.InlineData.MimeType != "" {
				images++
			}
		}
	}
	return images
}
func markGeminiGoogleSearchCall(c *gin.Context, response *dto.GeminiChatResponse) {
	if c == nil || response == nil {
		return
	}
	for _, candidate := range response.Candidates {
		if candidate.GroundingMetadata != nil && len(candidate.GroundingMetadata.WebSearchQueries) > 0 {
			c.Set("gemini_google_search_call", true)
			return
		}
	}
}
func countGeminiBillableFunctionCalls(info *relaycommon.RelayInfo, response *dto.GeminiChatResponse) {
	if info == nil || response == nil {
		return
	}
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.FunctionCall != nil {
				info.CountBillableToolCall(dto.BuildInCallFunctionCall, part.FunctionCall.FunctionName)
			}
		}
	}
}
func observeGeminiResponse(c *gin.Context, info *relaycommon.RelayInfo, response *dto.GeminiChatResponse, data string) {
	info.ObserveResponseModel(gjson.Get(data, "modelVersion").String())
	if len(response.Candidates) == 0 && response.PromptFeedback != nil && response.PromptFeedback.BlockReason != nil {
		info.PerformanceBusinessRejection = true
		common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "gemini_block_reason="+*response.PromptFeedback.BlockReason)
	}
	for _, candidate := range response.Candidates {
		if candidate.FinishReason == nil || *candidate.FinishReason == "" || *candidate.FinishReason == "FINISH_REASON_UNSPECIFIED" {
			continue
		}
		switch *candidate.FinishReason {
		case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII", "IMAGE_SAFETY", "IMAGE_PROHIBITED_CONTENT", "IMAGE_RECITATION":
			info.PerformanceBusinessRejection = true
		}
		info.StreamStatus.MarkCompleted()
	}
	markGeminiGoogleSearchCall(c, response)
	countGeminiBillableFunctionCalls(info, response)
}
func buildUsageFromGeminiResponse(c *gin.Context, info *relaycommon.RelayInfo, response *dto.GeminiChatResponse) dto.Usage {
	metadata := response.GetUsageMetadata()
	if dto.HasGeminiUsageMetadataTokens(metadata) {
		usage := buildUsageFromGeminiMetadata(metadata, info.GetEstimatePromptTokens())
		patchGeminiZeroCompletionUsage(c, info, &usage, geminiResponseUsageText(response), geminiResponseInlineImageCount(response))
		return usage
	}
	usage := service.ResponseText2Usage(c, geminiResponseUsageText(response), info.UpstreamModelName, info.GetEstimatePromptTokens())
	attachEstimatedGeminiBillingUsage(usage)
	return *usage
}
func responseGeminiChat2OpenAI(c *gin.Context, response *dto.GeminiChatResponse) *dto.OpenAITextResponse {
	return relayconvert.ResponseGeminiChat2OpenAI(helper.GetResponseID(c), common.GetTimestamp(), response)
}
func streamResponseGeminiChat2OpenAI(response *dto.GeminiChatResponse) (*dto.ChatCompletionsStreamResponse, bool) {
	return relayconvert.StreamResponseGeminiChat2OpenAI(response)
}
func handleStream(c *gin.Context, info *relaycommon.RelayInfo, frame *dto.ChatCompletionsStreamResponse) error {
	body, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	return openai.HandleStreamFormat(c, info, string(body), info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent)
}
func handleFinalStream(c *gin.Context, info *relaycommon.RelayInfo, frame *dto.ChatCompletionsStreamResponse) error {
	body, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	return openai.FinalizeChatStream(c, info, string(body), frame.Id, frame.Created, frame.Model, frame.GetSystemFingerprint(), frame.Usage, false)
}
func isGeminiDownstreamStop(c *gin.Context, info *relaycommon.RelayInfo) bool {
	return c != nil && c.Request != nil && c.Request.Context().Err() != nil || info != nil && info.StreamStatus != nil && info.StreamStatus.EndReason == relaycommon.StreamEndReasonClientGone
}

// A Gemini usageMetadata frame is a cumulative snapshot. Merge non-zero fields
// across every frame and keep its native billing dialect on partial streams.
func geminiStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response, callback func(string, *dto.GeminiChatResponse) bool) (*dto.Usage, *types.GatewayError) {
	if err := contentResponseContext(c, info, response); err != nil {
		return nil, contentResponseError(err)
	}
	if callback == nil {
		return nil, contentResponseError(errors.New("Gemini stream callback is missing"))
	}
	defer service.CloseResponseBodyGracefully(response)
	usage := &dto.Usage{}
	var metadata *dto.GeminiUsageMetadata
	var text strings.Builder
	images, frames := 0, 0
	var failure *types.GatewayError
	helper.StreamScannerHandler(c, response, info, func(data string, result *helper.StreamResult) {
		if wireError := geminiWireError([]byte(data)); wireError != nil {
			failure = wireError
			result.Stop(failure)
			return
		}
		var frame *dto.GeminiChatResponse
		if err := json.Unmarshal([]byte(data), &frame); err != nil {
			failure = contentResponseError(err)
			result.Stop(failure)
			return
		}
		if frame == nil {
			failure = contentResponseError(errors.New("Gemini stream frame must be an object"))
			result.Stop(failure)
			return
		}
		frames++
		observeGeminiResponse(c, info, frame, data)
		text.WriteString(geminiResponseUsageText(frame))
		images += geminiResponseInlineImageCount(frame)
		if current := frame.GetUsageMetadata(); dto.HasGeminiUsageMetadataTokens(current) {
			metadata = dto.MergeGeminiUsageMetadataNonZero(metadata, current)
			mapped := buildUsageFromGeminiMetadata(metadata, info.GetEstimatePromptTokens())
			*usage = mapped
		}
		if !callback(data, frame) {
			if isGeminiDownstreamStop(c, info) {
				result.Stop(nil)
			} else {
				failure = contentResponseError(errors.New("Gemini stream output stopped"))
				result.Stop(failure)
			}
		}
	})
	info.StreamStatus.RequireTerminal()
	if metadata == nil {
		if frames > 0 {
			usage = service.ResponseText2Usage(c, text.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		}
		if images > 0 && usage.CompletionTokens == 0 {
			usage.CompletionTokens = images * 1400
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
			common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
		}
		attachEstimatedGeminiBillingUsage(usage)
	} else {
		patchGeminiZeroCompletionUsage(c, info, usage, text.String(), images)
	}
	return usage, failure
}

func GeminiChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if err := contentResponseContext(c, info, response); err != nil {
		return nil, contentResponseError(err)
	}
	id, created := helper.GetResponseID(c), common.GetTimestamp()
	finish := constant.FinishReasonStop
	indices := make(map[int]map[string]int)
	var sendFailure error
	usage, failure := geminiStreamHandler(c, info, response, func(_ string, native *dto.GeminiChatResponse) bool {
		frame, stopped := streamResponseGeminiChat2OpenAI(native)
		if frame == nil {
			sendFailure = errors.New("Gemini chat codec returned no frame")
			return false
		}
		frame.Id, frame.Created, frame.Model = id, created, info.UpstreamModelName
		if frame.IsToolCall() {
			finish = constant.FinishReasonToolCalls
		}
		for choiceIndex := range frame.Choices {
			choice := &frame.Choices[choiceIndex]
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				finish = *choice.FinishReason
			}
			if info.RelayFormat == types.RelayFormatClaude {
				choice.FinishReason = nil
			}
			for callIndex := range choice.Delta.ToolCalls {
				call := &choice.Delta.ToolCalls[callIndex]
				if call.ID == "" {
					continue
				}
				slots := indices[choice.Index]
				if slots == nil {
					slots = make(map[string]int)
					indices[choice.Index] = slots
				}
				index, known := slots[call.ID]
				if !known {
					index = len(slots)
					slots[call.ID] = index
				}
				call.SetIndex(index)
			}
		}
		if info.SendResponseCount == 0 {
			opening := helper.GenerateStartEmptyResponse(id, created, info.UpstreamModelName, nil)
			opening.Usage = frame.Usage
			if sendFailure = handleStream(c, info, opening); sendFailure != nil {
				return false
			}
		}
		if sendFailure = handleStream(c, info, frame); sendFailure != nil {
			return false
		}
		if stopped && info.RelayFormat != types.RelayFormatClaude {
			sendFailure = handleStream(c, info, helper.GenerateStopResponse(id, created, info.UpstreamModelName, finish))
		}
		return sendFailure == nil
	})
	if sendFailure != nil && !isGeminiDownstreamStop(c, info) {
		return usage, contentResponseError(sendFailure)
	}
	if failure != nil {
		return usage, failure
	}
	if isGeminiDownstreamStop(c, info) || info.StreamStatus != nil && !info.StreamStatus.IsNormalEnd() {
		return usage, nil
	}
	final := helper.GenerateFinalUsageResponse(id, created, info.UpstreamModelName, *usage)
	if info.RelayFormat == types.RelayFormatClaude && !info.EnsureClaudeConvertInfo().Done {
		final = helper.GenerateStopResponse(id, created, info.UpstreamModelName, finish)
		final.Usage = usage
	}
	if err := handleFinalStream(c, info, final); err != nil {
		return usage, contentResponseError(err)
	}
	return usage, nil
}

func emptyGeminiCandidates(c *gin.Context, info *relaycommon.RelayInfo, response *dto.GeminiChatResponse) *types.GatewayError {
	if response.PromptFeedback != nil && response.PromptFeedback.BlockReason != nil {
		info.PerformanceBusinessRejection = true
		common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "gemini_block_reason="+*response.PromptFeedback.BlockReason)
		return types.NewOpenAIError(errors.New("request blocked by Gemini API: "+*response.PromptFeedback.BlockReason), types.ErrorCodePromptBlocked, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "gemini_empty_candidates")
	return types.NewOpenAIError(errors.New("Gemini returned no candidates"), types.ErrorCodeEmptyResponse, http.StatusBadGateway)
}
func readGeminiChat(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) ([]byte, *dto.GeminiChatResponse, *types.GatewayError) {
	if err := contentResponseContext(c, info, response); err != nil {
		return nil, nil, contentResponseError(err)
	}
	defer service.CloseResponseBodyGracefully(response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, nil, contentResponseError(err)
	}
	if failure := geminiWireError(body); failure != nil {
		return nil, nil, failure
	}
	var wire *dto.GeminiChatResponse
	if err = json.Unmarshal(body, &wire); err != nil {
		return nil, nil, contentResponseError(err)
	}
	if wire == nil {
		return nil, nil, contentResponseError(errors.New("Gemini response must be an object"))
	}
	observeGeminiResponse(c, info, wire, string(body))
	return body, wire, nil
}
func GeminiChatHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	body, wire, failure := readGeminiChat(c, info, response)
	if failure != nil {
		return nil, failure
	}
	usage := buildUsageFromGeminiResponse(c, info, wire)
	if len(wire.Candidates) == 0 {
		return &usage, emptyGeminiCandidates(c, info, wire)
	}
	if info.RelayFormat != types.RelayFormatGemini {
		chat := responseGeminiChat2OpenAI(c, wire)
		chat.Model = info.UpstreamModelName
		chat.Usage = usage
		var output any = chat
		if info.RelayFormat != types.RelayFormatOpenAI && info.RelayFormat != "" {
			converted, err := service.ConvertResponse(c, info, info.RelayFormat, chat)
			if err != nil {
				return &usage, contentResponseError(err)
			}
			output = converted.Value
		}
		var err error
		body, err = json.Marshal(output)
		if err != nil {
			return &usage, contentResponseError(err)
		}
	}
	service.IOCopyBytesGracefully(c, response, body)
	return &usage, nil
}
func GeminiEmbeddingHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if err := contentResponseContext(c, info, response); err != nil {
		return nil, contentResponseError(err)
	}
	defer service.CloseResponseBodyGracefully(response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, contentResponseError(err)
	}
	if failure := geminiWireError(body); failure != nil {
		return nil, failure
	}
	var wire struct {
		Embedding  *dto.ContentEmbedding   `json:"embedding"`
		Embeddings []*dto.ContentEmbedding `json:"embeddings"`
	}
	if err = json.Unmarshal(body, &wire); err != nil {
		return nil, contentResponseError(err)
	}
	if wire.Embedding != nil {
		wire.Embeddings = []*dto.ContentEmbedding{wire.Embedding}
	}
	if len(wire.Embeddings) == 0 {
		return nil, contentResponseError(errors.New("Gemini returned no embeddings"))
	}
	usage := service.ResponseText2Usage(c, "", info.UpstreamModelName, info.GetEstimatePromptTokens())
	output := dto.OpenAIEmbeddingResponse{Object: "list", Model: info.UpstreamModelName, Usage: *usage, Data: make([]dto.OpenAIEmbeddingResponseItem, 0, len(wire.Embeddings))}
	for index, embedding := range wire.Embeddings {
		if embedding == nil {
			return nil, contentResponseError(errors.New("Gemini returned an empty embedding"))
		}
		output.Data = append(output.Data, dto.OpenAIEmbeddingResponseItem{Object: "embedding", Index: index, Embedding: embedding.Values})
	}
	body, err = json.Marshal(output)
	if err != nil {
		return nil, contentResponseError(err)
	}
	service.IOCopyBytesGracefully(c, response, body)
	return usage, nil
}
func GeminiImageHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if err := contentResponseContext(c, info, response); err != nil {
		return nil, contentResponseError(err)
	}
	defer service.CloseResponseBodyGracefully(response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, contentResponseError(err)
	}
	if failure := geminiWireError(body); failure != nil {
		return nil, failure
	}
	var wire dto.GeminiImageResponse
	if err = json.Unmarshal(body, &wire); err != nil {
		return nil, contentResponseError(err)
	}
	output := dto.ImageResponse{Created: common.GetTimestamp(), Data: make([]dto.ImageData, 0, len(wire.Predictions))}
	for _, image := range wire.Predictions {
		if image.RaiFilteredReason == "" && image.BytesBase64Encoded != "" {
			output.Data = append(output.Data, dto.ImageData{B64Json: image.BytesBase64Encoded})
		}
	}
	if len(output.Data) == 0 {
		return nil, contentResponseError(errors.New("Imagen returned no generated images"))
	}
	body, err = json.Marshal(output)
	if err != nil {
		return nil, contentResponseError(err)
	}
	info.UpdateImageCount(int64(len(output.Data)))
	service.IOCopyBytesGracefully(c, response, body)
	tokens := 258 * len(output.Data)
	return &dto.Usage{PromptTokens: tokens, TotalTokens: tokens}, nil
}

type GeminiModelsResponse struct {
	Models        []dto.GeminiModel `json:"models"`
	NextPageToken string            `json:"nextPageToken"`
}

func FetchGeminiModels(base, key, proxy string) ([]string, error) {
	return FetchGeminiModelsContext(common.BackgroundContext(), base, key, proxy)
}
func FetchGeminiModelsContext(parent context.Context, base, key, proxy string) ([]string, error) {
	if parent == nil {
		parent = common.BackgroundContext()
	}
	endpoint, err := openai.ProviderEndpoint(base, "/v1beta/models")
	if err != nil {
		return nil, err
	}
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, err
	}
	models := make([]string, 0)
	seenModels, seenPages := make(map[string]bool), make(map[string]bool)
	pageToken := ""
	for page := 0; page < 100; page++ {
		if err := parent.Err(); err != nil {
			return nil, err
		}
		target, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		query := target.Query()
		if pageToken != "" {
			query.Set("pageToken", pageToken)
		}
		target.RawQuery = query.Encode()
		ctx, cancel := context.WithTimeout(parent, 30*time.Second)
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
		if err != nil {
			cancel()
			return nil, err
		}
		request.Header.Set("X-Goog-Api-Key", key)
		response, err := client.Do(request)
		if err != nil {
			cancel()
			return nil, err
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 16<<20))
		response.Body.Close()
		cancel()
		if readErr != nil {
			return nil, readErr
		}
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Gemini model listing failed (HTTP %d)", response.StatusCode)
		}
		var wire GeminiModelsResponse
		if err = json.Unmarshal(body, &wire); err != nil {
			return nil, err
		}
		for _, item := range wire.Models {
			if name, ok := item.Name.(string); ok {
				name = strings.TrimPrefix(name, "models/")
				if name != "" && !seenModels[name] {
					seenModels[name] = true
					models = append(models, name)
				}
			}
		}
		pageToken = wire.NextPageToken
		if pageToken == "" {
			return models, nil
		}
		if seenPages[pageToken] {
			return nil, errors.New("Gemini model listing repeated its pagination token")
		}
		seenPages[pageToken] = true
	}
	return nil, errors.New("Gemini model listing exceeded its pagination limit")
}
