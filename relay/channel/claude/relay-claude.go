package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting/model_setting"
	"github.com/gin-gonic/gin"
)

type ClaudeResponseInfo = relayconvert.ClaudeResponseInfo

func stopReasonClaude2OpenAI(reason string) string {
	return relayconvert.StopReasonClaudeToOpenAI(reason)
}
func StreamResponseClaude2OpenAI(frame *dto.ClaudeResponse) *dto.ChatCompletionsStreamResponse {
	return relayconvert.StreamResponseClaude2OpenAI(frame)
}
func ResponseClaude2OpenAI(response *dto.ClaudeResponse) *dto.OpenAITextResponse {
	return relayconvert.ResponseClaude2OpenAI(response)
}
func FormatClaudeResponseInfo(frame *dto.ClaudeResponse, chat *dto.ChatCompletionsStreamResponse, state *ClaudeResponseInfo) bool {
	return relayconvert.FormatClaudeResponseInfo(frame, chat, state)
}
func buildMessageDeltaPatchUsage(frame *dto.ClaudeResponse, state *ClaudeResponseInfo) *dto.ClaudeUsage {
	return relayconvert.BuildMessageDeltaPatchUsage(frame, state)
}
func patchClaudeMessageDeltaUsageData(data string, usage *dto.ClaudeUsage) string {
	return relayconvert.PatchClaudeMessageDeltaUsageData(data, usage)
}
func cacheCreationTokensForOpenAIUsage(usage *dto.Usage) int {
	if usage == nil {
		return 0
	}
	mapped := relayconvert.UsageFromClaudeUsage(usage)
	if mapped == nil {
		return 0
	}
	return mapped.PromptTokens - usage.PromptTokens - usage.PromptTokensDetails.CachedTokens
}
func buildOpenAIStyleUsageFromClaudeUsage(usage *dto.Usage) dto.Usage {
	if mapped := relayconvert.UsageFromClaudeUsage(usage); mapped != nil {
		return *mapped
	}
	return dto.Usage{}
}
func shouldSkipClaudeMessageDeltaUsagePatch(info *relaycommon.RelayInfo) bool {
	return model_setting.GetGlobalSettings().PassThroughRequestEnabled || info != nil && info.HasChannelMeta() && info.ChannelSetting.PassThroughBodyEnabled
}
func messagesResponseError(err error) *types.GatewayError {
	return types.NewError(err, types.ErrorCodeBadResponseBody)
}

func maybeMarkClaudeRefusal(c *gin.Context, info *relaycommon.RelayInfo, reason string) {
	if info == nil || !strings.EqualFold(reason, "refusal") {
		return
	}
	info.PerformanceBusinessRejection = true
	if c != nil {
		common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "claude_stop_reason=refusal")
	}
}
func observeMessagesFrame(c *gin.Context, info *relaycommon.RelayInfo, frame *dto.ClaudeResponse) {
	if frame.Type == "message_start" && frame.Message != nil {
		info.ObserveResponseModel(frame.Message.Model)
	}
	maybeMarkClaudeRefusal(c, info, frame.StopReason)
	if frame.Delta != nil && frame.Delta.StopReason != nil {
		maybeMarkClaudeRefusal(c, info, *frame.Delta.StopReason)
	}
	if frame.Type == "message_stop" {
		info.StreamStatus.MarkCompleted()
	}
}
func countClaudeStreamBillableTools(c *gin.Context, info *relaycommon.RelayInfo, frame *dto.ClaudeResponse) {
	if frame == nil || info == nil {
		return
	}
	if frame.Type == "content_block_start" && frame.ContentBlock != nil && frame.ContentBlock.Type == "tool_use" {
		info.CountBillableToolCall(dto.BuildInCallToolUse, frame.ContentBlock.Name)
	}
	if c != nil && frame.Type == "message_delta" && frame.Usage != nil && frame.Usage.ServerToolUse != nil && frame.Usage.ServerToolUse.WebSearchRequests > 0 {
		c.Set("claude_web_search_requests", frame.Usage.ServerToolUse.WebSearchRequests)
	}
}
func claudeToChatStreamState(info *relaycommon.RelayInfo) (*relayconvert.ClaudeToChatStreamState, error) {
	if info != nil && info.ClaudeToChatStreamState != nil {
		state, ok := info.ClaudeToChatStreamState.(*relayconvert.ClaudeToChatStreamState)
		if !ok || state == nil {
			return nil, fmt.Errorf("invalid Messages-to-chat state %T", info.ClaudeToChatStreamState)
		}
		return state, nil
	}
	state := relayconvert.NewClaudeToChatStreamState()
	if info != nil {
		info.ClaudeToChatStreamState = state
	}
	return state, nil
}
func claudeToGeminiStreamState(info *relaycommon.RelayInfo) (*relayconvert.ResponseStreamState, error) {
	if info != nil && info.ChatToGeminiStreamState != nil {
		state, ok := info.ChatToGeminiStreamState.(*relayconvert.ResponseStreamState)
		if !ok || state == nil {
			return nil, fmt.Errorf("invalid Messages-to-content state %T", info.ChatToGeminiStreamState)
		}
		return state, nil
	}
	state, err := relayconvert.NewResponseStreamState(types.RelayFormatClaude, types.RelayFormatGemini, relayconvert.ResponseStreamOptions{})
	if err == nil && info != nil {
		info.ChatToGeminiStreamState = state
	}
	return state, err
}
func sendGeminiStreamResults(c *gin.Context, results []relayconvert.ResponseResult) *types.GatewayError {
	for _, result := range results {
		frame, ok := result.Value.(*dto.GeminiChatResponse)
		if !ok {
			return messagesResponseError(fmt.Errorf("content stream codec returned %T", result.Value))
		}
		if frame == nil {
			continue
		}
		data, err := json.Marshal(frame)
		if err != nil {
			return messagesResponseError(err)
		}
		if err = helper.StringData(c, string(data)); err != nil {
			return messagesResponseError(err)
		}
	}
	return nil
}

func HandleStreamResponseData(c *gin.Context, info *relaycommon.RelayInfo, state *ClaudeResponseInfo, data string) *types.GatewayError {
	if c == nil || info == nil || state == nil {
		return messagesResponseError(errors.New("Messages stream context is missing"))
	}
	var frame dto.ClaudeResponse
	if err := json.Unmarshal([]byte(data), &frame); err != nil {
		return messagesResponseError(err)
	}
	if failure := frame.GetClaudeError(); failure != nil && failure.Type != "" {
		return types.WithClaudeError(*failure, http.StatusBadGateway)
	}
	observeMessagesFrame(c, info, &frame)
	switch info.RelayFormat {
	case types.RelayFormatClaude, "":
		FormatClaudeResponseInfo(&frame, nil, state)
		if frame.Type == "message_start" && frame.Message != nil && info.HasChannelMeta() {
			info.UpstreamModelName = frame.Message.Model
		}
		if frame.Type == "message_delta" && !shouldSkipClaudeMessageDeltaUsagePatch(info) {
			data = patchClaudeMessageDeltaUsageData(data, buildMessageDeltaPatchUsage(&frame, state))
		}
		countClaudeStreamBillableTools(c, info, &frame)
		helper.ClaudeChunkData(c, frame, data)
	case types.RelayFormatOpenAI:
		codec, err := claudeToChatStreamState(info)
		if err != nil {
			return messagesResponseError(err)
		}
		converted, err := codec.ConvertChunk(&frame)
		if err != nil {
			return messagesResponseError(err)
		}
		if !FormatClaudeResponseInfo(&frame, converted, state) {
			return nil
		}
		countClaudeStreamBillableTools(c, info, &frame)
		if converted != nil {
			if err = helper.ObjectData(c, converted); err != nil {
				return messagesResponseError(err)
			}
		}
	case types.RelayFormatGemini:
		codec, err := claudeToGeminiStreamState(info)
		if err != nil {
			return messagesResponseError(err)
		}
		results, err := service.ConvertStreamResponseChunk(c, info, codec, &frame)
		if err != nil {
			return messagesResponseError(err)
		}
		if !FormatClaudeResponseInfo(&frame, nil, state) {
			return nil
		}
		countClaudeStreamBillableTools(c, info, &frame)
		return sendGeminiStreamResults(c, results)
	default:
		return messagesResponseError(fmt.Errorf("Messages stream cannot emit protocol %q", info.RelayFormat))
	}
	return nil
}

func messagesModel(info *relaycommon.RelayInfo, state *ClaudeResponseInfo) string {
	if info != nil && info.HasChannelMeta() && info.UpstreamModelName != "" {
		return info.UpstreamModelName
	}
	return state.Model
}
func settleMessagesStream(c *gin.Context, info *relaycommon.RelayInfo, state *ClaudeResponseInfo) {
	if state.Usage == nil {
		state.Usage = &dto.Usage{}
	}
	if state.Usage.CompletionTokens == 0 || !state.Done {
		estimate := service.ResponseText2Usage(c, state.ResponseText.String(), messagesModel(info, state), info.GetEstimatePromptTokens())
		if state.Usage.CompletionTokens == 0 || !state.Done && estimate.CompletionTokens > state.Usage.CompletionTokens {
			state.Usage.CompletionTokens = estimate.CompletionTokens
		}
		if state.Usage.PromptTokens == 0 {
			state.Usage.PromptTokens = estimate.PromptTokens
		}
		state.Usage.TotalTokens = state.Usage.PromptTokens + state.Usage.CompletionTokens
	}
	state.Usage.UsageSemantic = "anthropic"
	relayconvert.FinalizeClaudeStreamBillingUsage(state)
}
func finalizeMessagesStream(c *gin.Context, info *relaycommon.RelayInfo, state *ClaudeResponseInfo) *types.GatewayError {
	settleMessagesStream(c, info, state)
	if c.Request != nil && c.Request.Context().Err() != nil {
		return nil
	}
	if info.StreamStatus != nil && !info.StreamStatus.IsNormalEnd() {
		return nil
	}
	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		if info.ShouldIncludeUsage {
			usage := buildOpenAIStyleUsageFromClaudeUsage(state.Usage)
			if err := helper.ObjectData(c, helper.GenerateFinalUsageResponse(state.ResponseId, state.Created, messagesModel(info, state), usage)); err != nil {
				return messagesResponseError(err)
			}
		}
		helper.Done(c)
	case types.RelayFormatGemini:
		codec, err := claudeToGeminiStreamState(info)
		if err != nil {
			return messagesResponseError(err)
		}
		results, err := service.FinalizeStreamResponse(c, info, codec)
		if err != nil {
			return messagesResponseError(err)
		}
		return sendGeminiStreamResults(c, results)
	}
	return nil
}
func HandleStreamFinalResponse(c *gin.Context, info *relaycommon.RelayInfo, state *ClaudeResponseInfo) {
	if c == nil || info == nil || state == nil {
		return
	}
	if err := finalizeMessagesStream(c, info, state); err != nil {
		info.StreamStatus.RecordError(err.Error())
		info.StreamStatus.MarkFailed("messages_finalize", "server_error", 500)
	}
}
func messagesState(c *gin.Context, info *relaycommon.RelayInfo) *ClaudeResponseInfo {
	model := info.OriginModelName
	if info.HasChannelMeta() {
		model = info.UpstreamModelName
	}
	return &ClaudeResponseInfo{ResponseId: helper.GetResponseID(c), Created: common.GetTimestamp(), Model: model, Usage: &dto.Usage{}}
}
func ClaudeStreamHandler(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.GatewayError) {
	if c == nil || info == nil || response == nil || response.Body == nil {
		return nil, messagesResponseError(errors.New("Messages response context or body is missing"))
	}
	defer service.CloseResponseBodyGracefully(response)
	state := messagesState(c, info)
	var failure *types.GatewayError
	helper.StreamScannerHandler(c, response, info, func(data string, result *helper.StreamResult) {
		if failure = HandleStreamResponseData(c, info, state, data); failure != nil {
			result.Stop(failure)
		}
	})
	info.StreamStatus.RequireTerminal()
	settleMessagesStream(c, info, state)
	if failure != nil {
		return state.Usage, failure
	}
	failure = finalizeMessagesStream(c, info, state)
	return state.Usage, failure
}

func messagesUsage(wire *dto.ClaudeUsage) *dto.Usage {
	if wire == nil {
		return &dto.Usage{}
	}
	result := &dto.Usage{PromptTokens: wire.InputTokens, CompletionTokens: wire.OutputTokens, TotalTokens: wire.InputTokens + wire.OutputTokens, UsageSemantic: "anthropic", BillingUsage: dto.CloneBillingUsage(wire.BillingUsage)}
	if result.BillingUsage == nil {
		result.BillingUsage = dto.NewClaudeMessagesBillingUsage(wire)
	}
	result.PromptTokensDetails.CachedTokens = wire.CacheReadInputTokens
	result.PromptTokensDetails.CachedCreationTokens = wire.CacheCreationInputTokens
	result.ClaudeCacheCreation5mTokens = wire.GetCacheCreation5mTokens()
	result.ClaudeCacheCreation1hTokens = wire.GetCacheCreation1hTokens()
	return result
}
func HandleClaudeResponseData(c *gin.Context, info *relaycommon.RelayInfo, state *ClaudeResponseInfo, response *http.Response, data []byte) *types.GatewayError {
	if c == nil || info == nil || state == nil {
		return messagesResponseError(errors.New("Messages response context is missing"))
	}
	var wire *dto.ClaudeResponse
	if err := json.Unmarshal(data, &wire); err != nil {
		return messagesResponseError(err)
	}
	if wire == nil {
		return messagesResponseError(errors.New("Messages response must be an object"))
	}
	if failure := wire.GetClaudeError(); failure != nil && failure.Type != "" {
		return types.WithClaudeError(*failure, http.StatusBadGateway)
	}
	info.ObserveResponseModel(wire.Model)
	maybeMarkClaudeRefusal(c, info, wire.StopReason)
	state.Usage = messagesUsage(wire.Usage)
	output := data
	if info.RelayFormat != types.RelayFormatClaude && info.RelayFormat != "" {
		converted, err := service.ConvertResponse(c, info, info.RelayFormat, wire)
		if err != nil {
			return messagesResponseError(err)
		}
		if responses, ok := converted.Value.(*dto.OpenAIResponsesResponse); ok && responses != nil {
			if id := helper.GetResponseID(c); id != "" {
				responses.ID = id
			}
		}
		var err2 error
		output, err2 = json.Marshal(converted.Value)
		if err2 != nil {
			return messagesResponseError(err2)
		}
	}
	if wire.Usage != nil && wire.Usage.ServerToolUse != nil && wire.Usage.ServerToolUse.WebSearchRequests > 0 {
		c.Set("claude_web_search_requests", wire.Usage.ServerToolUse.WebSearchRequests)
	}
	for _, block := range wire.Content {
		if block.Type == "tool_use" {
			info.CountBillableToolCall(dto.BuildInCallToolUse, block.Name)
		}
	}
	service.IOCopyBytesGracefully(c, response, output)
	return nil
}
func ClaudeHandler(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.GatewayError) {
	if c == nil || info == nil || response == nil || response.Body == nil {
		return nil, messagesResponseError(errors.New("Messages response context or body is missing"))
	}
	defer service.CloseResponseBodyGracefully(response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, messagesResponseError(err)
	}
	state := messagesState(c, info)
	if failure := HandleClaudeResponseData(c, info, state, response, body); failure != nil {
		return nil, failure
	}
	return state.Usage, nil
}
