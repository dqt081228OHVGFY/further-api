package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	kitreasoning "github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert/reasoning"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting/model_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/reasoning"
	"github.com/gin-gonic/gin"
)

func requestConfiguration(info *relaycommon.RelayInfo) error {
	if info == nil || !info.HasChannelMeta() {
		return errors.New("OpenAI provider configuration is missing")
	}
	return nil
}

func InitializeProtocol(info *relaycommon.RelayInfo) {
	if info == nil || !info.HasChannelMeta() {
		return
	}
	if info.ChannelSetting.ThinkingToContent {
		info.ThinkingContentInfo = relaycommon.ThinkingContentInfo{IsFirstThinkingContent: true}
	}
}

func ChatFrom(c *gin.Context, info *relaycommon.RelayInfo, source any) (*dto.GeneralOpenAIRequest, error) {
	if err := requestConfiguration(info); err != nil {
		return nil, err
	}
	switch request := source.(type) {
	case *dto.ClaudeRequest:
		if request == nil {
			return nil, errors.New("Anthropic request is missing")
		}
	case *dto.GeminiChatRequest:
		if request == nil {
			return nil, errors.New("Gemini request is missing")
		}
	case *dto.OpenAIResponsesRequest:
		if request == nil {
			return nil, errors.New("Responses request is missing")
		}
	case *dto.GeneralOpenAIRequest:
		return NativeChat(c, info, request)
	}
	converted, err := service.ConvertRequest(c, info, types.RelayFormatOpenAI, source)
	if err != nil {
		return nil, err
	}
	request, ok := converted.Value.(*dto.GeneralOpenAIRequest)
	if !ok || request == nil {
		return nil, fmt.Errorf("OpenAI conversion returned %T instead of chat completions", converted.Value)
	}
	if info.SupportStreamOptions && info.IsStream {
		request.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
	}
	return NativeChat(c, info, request)
}

func preservedModelNames(models ...string) bool {
	for _, model := range models {
		if model_setting.ShouldPreserveThinkingSuffix(model) {
			return true
		}
	}
	return false
}

// Chat Completions and Responses use the same merge order for model modifiers.
func suffixIntent(c *gin.Context, info *relaycommon.RelayInfo, intent kitreasoning.Intent, preserved bool, models ...string) (kitreasoning.Intent, error) {
	if preserved {
		return intent, nil
	}
	seen := make(map[string]bool, len(models))
	for _, model := range models {
		if seen[model] {
			continue
		}
		seen[model] = true
		raw, _ := reasoning.ParseOpenAIReasoningEffortFromModelSuffix(model)
		if raw == "" {
			continue
		}
		effort, err := kitreasoning.ParseEffort(raw)
		if err != nil {
			return intent, kitreasoning.AsClientError(err)
		}
		mode := kitreasoning.ModeEnabled
		if effort == kitreasoning.EffortNone {
			mode = kitreasoning.ModeDisabled
		}
		merged, diagnostics, err := kitreasoning.MergeExplicitAndSuffix(intent, kitreasoning.Intent{Mode: mode, Effort: effort, Source: kitreasoning.SourceSuffix}, model)
		recordReasoningDiagnostics(c, info, diagnostics)
		if err != nil {
			return intent, kitreasoning.AsClientError(err)
		}
		intent = merged
	}
	return intent, nil
}

func NativeChat(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (*dto.GeneralOpenAIRequest, error) {
	if request == nil {
		return nil, errors.New("OpenAI chat request is missing")
	}
	if err := requestConfiguration(info); err != nil {
		return nil, err
	}
	native := info.ChannelType == constant.ChannelTypeOpenAI || info.ChannelType == constant.ChannelTypeAzure
	router := info.ChannelType == constant.ChannelTypeOpenRouter
	if !native {
		request.StreamOptions = nil
	}
	preserved := preservedModelNames(info.OriginModelName, info.UpstreamModelName)
	upstreamEffort, baseModel := reasoning.ParseOpenAIReasoningEffortFromModelSuffix(info.UpstreamModelName)
	originEffort, _ := reasoning.ParseOpenAIReasoningEffortFromModelSuffix(info.OriginModelName)
	project := router || len(request.Reasoning) != 0 || len(info.RequestConversionChain) > 1 || request.ReasoningConversion != nil || info.ReasoningState() != nil || (!preserved && (upstreamEffort != "" || originEffort != ""))
	if !project {
		info.SetReasoningEffort(request.ReasoningEffort)
	} else {
		if request.ReasoningConversion == nil {
			request.ReasoningConversion = info.ReasoningState()
		}
		intent, diagnostics, err := kitreasoning.FromOpenAIChat(request)
		recordReasoningDiagnostics(c, info, diagnostics)
		if err != nil {
			return nil, kitreasoning.AsClientError(err)
		}
		if router && len(request.THINKING) != 0 && strings.HasPrefix(info.UpstreamModelName, "anthropic") {
			var thinking dto.Thinking
			if err := json.Unmarshal(request.THINKING, &thinking); err != nil {
				return nil, kitreasoning.AsClientError(fmt.Errorf("invalid thinking settings: %w", err))
			}
			legacy, diagnostics, err := kitreasoning.FromClaude(&dto.ClaudeRequest{Thinking: &thinking})
			recordReasoningDiagnostics(c, info, diagnostics)
			if err != nil {
				return nil, kitreasoning.AsClientError(err)
			}
			intent, diagnostics, err = kitreasoning.MergeExplicit(intent, legacy, request.Model)
			recordReasoningDiagnostics(c, info, diagnostics)
			if err != nil {
				return nil, kitreasoning.AsClientError(err)
			}
			request.THINKING = nil
		}
		intent, err = suffixIntent(c, info, intent, preserved, info.UpstreamModelName, info.OriginModelName)
		if err != nil {
			return nil, err
		}
		if !preserved && upstreamEffort != "" {
			info.UpstreamModelName, request.Model = baseModel, baseModel
		}
		if router {
			if len(request.Usage) == 0 {
				request.Usage = json.RawMessage(`{"include":true}`)
			}
			if err := routerReasoning(request, intent); err != nil {
				return nil, err
			}
			request.ReasoningEffort = ""
			resolved := kitreasoning.EffectiveEffort(intent)
			if intent.BudgetTokens != nil {
				resolved = kitreasoning.EffortFromBudget(*intent.BudgetTokens)
			}
			info.SetReasoningEffort(string(resolved))
		} else {
			if resolved := kitreasoning.EffectiveEffort(intent); resolved != "" {
				request.ReasoningEffort = string(resolved)
				info.SetReasoningEffort(string(resolved))
			}
			if native {
				request.Reasoning = nil
			}
		}
	}
	capabilities := dto.GetOpenAIChatCapabilities(info.UpstreamModelName, info.ReasoningEffort)
	if capabilities.UseMaxCompletionTokens && (request.MaxCompletionTokens == nil || *request.MaxCompletionTokens == 0) && request.MaxTokens != nil && *request.MaxTokens != 0 {
		request.MaxCompletionTokens, request.MaxTokens = request.MaxTokens, nil
	}
	if !capabilities.SupportsTemperature {
		request.Temperature = nil
	}
	if !capabilities.SupportsTopP {
		request.TopP = nil
	}
	if !capabilities.SupportsLogProbs {
		request.LogProbs, request.TopLogProbs = nil, nil
	}
	if capabilities.UseDeveloperRole && len(request.Messages) > 0 && request.Messages[0].Role == "system" {
		request.Messages[0].Role = "developer"
	}
	return request, nil
}

func routerReasoning(request *dto.GeneralOpenAIRequest, intent kitreasoning.Intent) error {
	if intent.IsEmpty() {
		return nil
	}
	fields := make(map[string]any)
	if len(request.Reasoning) > 0 {
		if err := json.Unmarshal(request.Reasoning, &fields); err != nil {
			return kitreasoning.AsClientError(err)
		}
		if fields == nil {
			fields = make(map[string]any)
		}
	}
	disabled := intent.Mode == kitreasoning.ModeDisabled || intent.Effort == kitreasoning.EffortNone
	if intent.HasStrength() {
		fields["enabled"] = !disabled
	}
	if disabled {
		delete(fields, "effort")
		delete(fields, "max_tokens")
	} else if intent.BudgetTokens != nil {
		fields["max_tokens"] = *intent.BudgetTokens
		delete(fields, "effort")
	} else if intent.Effort != "" {
		fields["effort"] = string(intent.Effort)
		delete(fields, "max_tokens")
	}
	if intent.IncludeThoughts != nil {
		fields["exclude"] = !*intent.IncludeThoughts
	}
	encoded, err := json.Marshal(fields)
	if err == nil {
		request.Reasoning = encoded
	}
	return err
}

func NativeResponses(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (dto.OpenAIResponsesRequest, error) {
	origin, router, crossProtocol := "", false, false
	if info != nil {
		origin, crossProtocol = info.OriginModelName, len(info.RequestConversionChain) > 1
		if info.HasChannelMeta() {
			router = info.ChannelType == constant.ChannelTypeOpenRouter
		}
	}
	preserved := preservedModelNames(request.Model, origin)
	effort, baseModel := reasoning.ParseOpenAIReasoningEffortFromModelSuffix(request.Model)
	originEffort, _ := reasoning.ParseOpenAIReasoningEffortFromModelSuffix(origin)
	if !router && !crossProtocol && request.ReasoningConversion == nil && info.ReasoningState() == nil && (preserved || effort == "" && originEffort == "") {
		if request.Reasoning != nil {
			info.SetReasoningEffort(request.Reasoning.Effort)
		} else {
			info.SetReasoningEffort("")
		}
		return request, nil
	}
	if request.ReasoningConversion == nil {
		request.ReasoningConversion = info.ReasoningState()
	}
	intent, diagnostics, err := kitreasoning.FromOpenAIResponses(&request)
	recordReasoningDiagnostics(c, info, diagnostics)
	if err != nil {
		return request, kitreasoning.AsClientError(err)
	}
	intent, err = suffixIntent(c, info, intent, preserved, request.Model, origin)
	if err != nil {
		return request, err
	}
	if !preserved && effort != "" {
		request.Model = baseModel
		if info != nil && info.HasChannelMeta() {
			info.UpstreamModelName = baseModel
		}
	}
	if resolved := kitreasoning.EffectiveEffort(intent); resolved != "" {
		if request.Reasoning == nil {
			request.Reasoning = &dto.Reasoning{}
		}
		request.Reasoning.Effort = string(resolved)
		info.SetReasoningEffort(string(resolved))
	}
	return request, nil
}

func recordReasoningDiagnostics(c *gin.Context, info *relaycommon.RelayInfo, diagnostics []types.ConversionDiagnostic) {
	if info == nil || len(diagnostics) == 0 {
		return
	}
	for i := range diagnostics {
		diagnostics[i].From = info.RelayFormat
	}
	info.RecordConversionDiagnostics(c, diagnostics)
}
