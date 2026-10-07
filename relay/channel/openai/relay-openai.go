package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openrouter"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

func chatResponseError(err error) *types.GatewayError {
	return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
}
func responseContext(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) error {
	if c == nil || info == nil || !info.HasChannelMeta() || response == nil || response.Body == nil {
		return errors.New("OpenAI response context or body is missing")
	}
	return nil
}

// Thinking-to-content operates on each choice's own text. A multi-choice
// response must never copy one choice's reasoning into the other choices.
func sendStreamData(c *gin.Context, info *relaycommon.RelayInfo, data string, force, thinking bool) error {
	if data == "" {
		return nil
	}
	if !force && !thinking {
		return helper.StringData(c, data)
	}
	var frame dto.ChatCompletionsStreamResponse
	if err := json.Unmarshal([]byte(data), &frame); err != nil {
		return err
	}
	if !thinking {
		return helper.ObjectData(c, frame)
	}
	state := &info.ThinkingContentInfo
	hasReasoning, hasContent := false, false
	for _, choice := range frame.Choices {
		hasReasoning = hasReasoning || choice.Delta.GetReasoningContent() != ""
		hasContent = hasContent || choice.Delta.GetContentString() != ""
	}
	if hasContent && state.HasSentThinkingContent && !state.SendLastThinkingContent {
		closing := frame.Copy()
		for i := range closing.Choices {
			closing.Choices[i].Delta.SetContentString("\n</think>\n")
			closing.Choices[i].Delta.ReasoningContent = nil
			closing.Choices[i].Delta.Reasoning = nil
			closing.Choices[i].Delta.ToolCalls = nil
			closing.Choices[i].FinishReason = nil
		}
		if err := helper.ObjectData(c, closing); err != nil {
			return err
		}
		state.SendLastThinkingContent = true
	}
	opening := hasReasoning && !state.HasSentThinkingContent
	for i := range frame.Choices {
		delta := &frame.Choices[i].Delta
		if reason := delta.GetReasoningContent(); reason != "" {
			text := reason
			if opening {
				text = "<think>\n" + text
			}
			if content := delta.GetContentString(); content != "" {
				text += "\n</think>\n" + content
				state.SendLastThinkingContent = true
			}
			delta.SetContentString(text)
		}
		delta.ReasoningContent, delta.Reasoning = nil, nil
	}
	if hasReasoning {
		state.HasSentThinkingContent = true
		state.IsFirstThinkingContent = false
	}
	return helper.ObjectData(c, frame)
}

type chatStreamAccumulator struct {
	tail          string
	frame         dto.ChatCompletionsStreamResponse
	usage         *dto.Usage
	upstreamUsage bool
	usageFrame    []byte
	text          strings.Builder
	toolCount     int
	tools         map[string]struct{}
	names         []string
}

func (state *chatStreamAccumulator) receive(info *relaycommon.RelayInfo, data string) error {
	var frame dto.ChatCompletionsStreamResponse
	if err := json.Unmarshal([]byte(data), &frame); err != nil {
		return err
	}
	var errorEnvelope dto.OpenAITextResponse
	if err := json.Unmarshal([]byte(data), &errorEnvelope); err == nil {
		if failure := errorEnvelope.GetOpenAIError(); failure != nil && failure.Type != "" {
			return types.WithOpenAIError(*failure, http.StatusBadGateway)
		}
	}
	if frame.Id != "" {
		state.frame.Id = frame.Id
	}
	if frame.Created != 0 {
		state.frame.Created = frame.Created
	}
	if frame.Model != "" {
		state.frame.Model = frame.Model
		info.ObserveResponseModel(frame.Model)
	}
	if fingerprint := frame.GetSystemFingerprint(); fingerprint != "" {
		state.frame.SetSystemFingerprint(fingerprint)
	}
	if frame.Usage != nil {
		state.usage = dto.MergeUsageNonZero(state.usage, frame.Usage)
		state.upstreamUsage = state.upstreamUsage || service.ValidUsage(frame.Usage)
		state.usageFrame = []byte(data)
	}
	applyUsagePostProcessing(info, state.usage, []byte(data))
	observeStreamChoices(info, data, state.tools, &state.names)
	if info.RelayMode == relayconstant.RelayModeCompletions {
		var completion dto.CompletionsStreamResponse
		if err := json.Unmarshal([]byte(data), &completion); err != nil {
			return err
		}
		for _, choice := range completion.Choices {
			state.text.WriteString(choice.Text)
		}
	} else {
		if err := ProcessStreamResponse(frame, &state.text, &state.toolCount); err != nil {
			return err
		}
	}
	state.tail = data
	return nil
}

func OaiStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if err := responseContext(c, info, response); err != nil {
		return nil, chatResponseError(err)
	}
	defer service.CloseResponseBodyGracefully(response)
	state := &chatStreamAccumulator{usage: &dto.Usage{}, tools: make(map[string]struct{})}
	state.frame.Model = info.UpstreamModelName
	var failure *types.GatewayError
	helper.StreamScannerHandler(c, response, info, func(data string, result *helper.StreamResult) {
		if data == "" {
			return
		}
		previous := state.tail
		if err := state.receive(info, data); err != nil {
			var typed *types.GatewayError
			if errors.As(err, &typed) {
				failure = typed
			} else {
				failure = chatResponseError(err)
			}
			result.Stop(failure)
			return
		}
		if previous != "" {
			if err := HandleStreamFormat(c, info, previous, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent); err != nil {
				failure = chatResponseError(err)
				result.Stop(failure)
			}
		}
	})
	info.StreamStatus.RequireTerminal()
	if !state.upstreamUsage {
		estimate := service.ResponseText2Usage(c, state.text.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		if state.usage.PromptTokens == 0 {
			state.usage.PromptTokens = estimate.PromptTokens
		}
		if state.usage.CompletionTokens == 0 {
			state.usage.CompletionTokens = estimate.CompletionTokens + state.toolCount*7
		}
		state.usage.TotalTokens = state.usage.PromptTokens + state.usage.CompletionTokens
	}
	applyUsagePostProcessing(info, state.usage, state.usageFrame)
	for _, name := range state.names {
		info.CountBillableToolCall(dto.BuildInCallFunctionCall, name)
	}
	if failure != nil {
		return state.usage, failure
	}
	if c.Request != nil && c.Request.Context().Err() != nil {
		return state.usage, nil
	}
	if info.StreamStatus != nil && !info.StreamStatus.IsNormalEnd() {
		return state.usage, nil
	}
	if state.tail == "" {
		return state.usage, chatResponseError(errors.New("OpenAI stream contained no response frames"))
	}
	if info.RelayFormat == types.RelayFormatOpenAI {
		var tail dto.ChatCompletionsStreamResponse
		if err := json.Unmarshal([]byte(state.tail), &tail); err != nil {
			return state.usage, chatResponseError(err)
		}
		output := !service.ValidUsage(tail.Usage) || info.ShouldIncludeUsage || len(tail.Choices) > 0
		if output {
			if err := sendStreamData(c, info, state.tail, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent); err != nil {
				return state.usage, chatResponseError(err)
			}
		}
	}
	if err := FinalizeChatStream(c, info, state.tail, state.frame.Id, state.frame.Created, state.frame.Model, state.frame.GetSystemFingerprint(), state.usage, state.upstreamUsage); err != nil {
		return state.usage, chatResponseError(err)
	}
	return state.usage, nil
}

func observeStreamChoices(info *relaycommon.RelayInfo, data string, seen map[string]struct{}, names *[]string) {
	var frame dto.ChatCompletionsStreamResponse
	if json.Unmarshal([]byte(data), &frame) != nil {
		return
	}
	for _, choice := range frame.Choices {
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			if *choice.FinishReason == constant.FinishReasonContentFilter {
				info.PerformanceBusinessRejection = true
			}
			info.StreamStatus.MarkCompleted()
		}
		for index, call := range choice.Delta.ToolCalls {
			name := strings.TrimSpace(call.Function.Name)
			if name == "" {
				continue
			}
			if call.Index != nil {
				index = *call.Index
			}
			slot := fmt.Sprintf("%d/%d/%s", choice.Index, index, name)
			pending, active := "pending/"+slot, "active/"+slot
			if call.ID != "" {
				identity := fmt.Sprintf("id/%d/%s", choice.Index, call.ID)
				if _, known := seen[identity]; known {
					continue
				}
				seen[identity] = struct{}{}
				seen[active] = struct{}{}
				if _, delayed := seen[pending]; delayed {
					delete(seen, pending)
					continue
				}
			} else {
				if _, known := seen[active]; known {
					continue
				}
				seen[active], seen[pending] = struct{}{}, struct{}{}
			}
			*names = append(*names, name)
		}
	}
}

func OpenaiHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if err := responseContext(c, info, response); err != nil {
		return nil, chatResponseError(err)
	}
	defer service.CloseResponseBodyGracefully(response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusBadGateway)
	}
	if info.ChannelType == constant.ChannelTypeOpenRouter && info.ChannelOtherSettings.IsOpenRouterEnterprise() {
		var enterprise openrouter.OpenRouterEnterpriseResponse
		if err = json.Unmarshal(body, &enterprise); err != nil {
			return nil, chatResponseError(err)
		}
		if !enterprise.Success {
			return nil, chatResponseError(errors.New("OpenRouter enterprise response failed"))
		}
		body = enterprise.Data
	}
	var wire dto.OpenAITextResponse
	if err = json.Unmarshal(body, &wire); err != nil {
		return nil, chatResponseError(err)
	}
	if failure := wire.GetOpenAIError(); failure != nil && failure.Type != "" {
		status := response.StatusCode
		if status < 400 {
			status = http.StatusBadGateway
		}
		return nil, types.WithOpenAIError(*failure, status)
	}
	info.ObserveResponseModel(wire.Model)
	for _, choice := range wire.Choices {
		if choice.FinishReason == constant.FinishReasonContentFilter {
			info.PerformanceBusinessRejection = true
			common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "openai_finish_reason=content_filter")
		}
		for _, call := range choice.Message.ParseToolCalls() {
			info.CountBillableToolCall(dto.BuildInCallFunctionCall, call.Function.Name)
		}
	}
	patched := wire.Usage.PromptTokens == 0
	if patched {
		if wire.Usage.CompletionTokens == 0 {
			for _, choice := range wire.Choices {
				wire.Usage.CompletionTokens += service.CountTextToken(choice.Message.StringContent()+choice.Message.GetReasoningContent(), info.UpstreamModelName)
			}
		}
		wire.Usage.PromptTokens = info.GetEstimatePromptTokens()
		wire.Usage.TotalTokens = wire.Usage.PromptTokens + wire.Usage.CompletionTokens
	}
	applyUsagePostProcessing(info, &wire.Usage, body)
	nativeOutput := true
	switch info.RelayFormat {
	case types.RelayFormatClaude, types.RelayFormatGemini, types.RelayFormatOpenAIResponses:
		nativeOutput = false
		converted, convertErr := service.ConvertResponse(c, info, info.RelayFormat, &wire)
		if convertErr != nil {
			return nil, chatResponseError(convertErr)
		}
		body, err = json.Marshal(converted.Value)
	}
	if nativeOutput {
		if info.ChannelSetting.ForceFormat {
			body, err = json.Marshal(wire)
		} else if patched {
			var fields map[string]json.RawMessage
			if err = json.Unmarshal(body, &fields); err == nil {
				if fields == nil {
					return nil, chatResponseError(errors.New("OpenAI response must be an object"))
				}
				fields["usage"], err = json.Marshal(wire.Usage)
				if err == nil {
					body, err = json.Marshal(fields)
				}
			}
		}
	}
	if err != nil {
		return nil, chatResponseError(err)
	}
	service.IOCopyBytesGracefully(c, response, body)
	return &wire.Usage, nil
}
