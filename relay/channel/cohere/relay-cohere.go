package cohere

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

func requestOpenAI2Cohere(request dto.GeneralOpenAIRequest) *CohereRequest {
	output := &CohereRequest{Model: request.Model, MaxTokens: request.GetMaxTokens(), Stream: request.Stream != nil && *request.Stream, ChatHistory: make([]ChatHistory, 0, len(request.Messages))}
	if output.MaxTokens == 0 {
		output.MaxTokens = 4000
	}
	if common.CohereSafetySetting != "NONE" {
		output.SafetyMode = common.CohereSafetySetting
	}
	latest := -1
	for index, message := range request.Messages {
		if message.Role == "user" {
			latest = index
		}
	}
	for index, message := range request.Messages {
		if index == latest {
			output.Message = message.StringContent()
			continue
		}
		role := "USER"
		switch message.Role {
		case "system", "developer":
			role = "SYSTEM"
		case "assistant":
			role = "CHATBOT"
		}
		output.ChatHistory = append(output.ChatHistory, ChatHistory{Role: role, Message: message.StringContent()})
	}
	return output
}
func requestConvertRerank2Cohere(request dto.RerankRequest) *CohereRerankRequest {
	count := 1
	if request.TopN != nil && *request.TopN > 0 {
		count = *request.TopN
	}
	return &CohereRerankRequest{Query: request.Query, Documents: request.Documents, Model: request.Model, TopN: count, ReturnDocuments: true}
}
func stopReasonCohere2OpenAI(reason string) string {
	switch reason {
	case "COMPLETE":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	default:
		return reason
	}
}
func cohereUsage(meta CohereMeta) *dto.Usage {
	return &dto.Usage{PromptTokens: meta.BilledUnits.InputTokens, CompletionTokens: meta.BilledUnits.OutputTokens, TotalTokens: meta.BilledUnits.InputTokens + meta.BilledUnits.OutputTokens}
}
func cohereStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if response == nil || response.Body == nil {
		return nil, types.NewError(errors.New("Cohere stream body is missing"), types.ErrorCodeBadResponseBody)
	}
	adapted := *response
	adapted.Body = openai.JSONLinesAsSSE(response.Body)
	return openai.NativeTextStream(c, info, &adapted, func(data []byte) (*dto.ChatCompletionsStreamResponse, *types.GatewayError) {
		var wire *CohereResponse
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		if wire == nil {
			return nil, types.NewError(errors.New("Cohere stream frame must be an object"), types.ErrorCodeBadResponseBody)
		}
		frame := &dto.ChatCompletionsStreamResponse{Id: helper.GetResponseID(c), Created: common.GetTimestamp(), Model: info.UpstreamModelName, Object: "chat.completion.chunk"}
		choice := dto.ChatCompletionsStreamResponseChoice{Index: 0}
		if wire.IsFinished {
			finish := stopReasonCohere2OpenAI(wire.FinishReason)
			choice.FinishReason = &finish
			if wire.Response != nil {
				frame.Usage = cohereUsage(wire.Response.Meta)
			}
		} else {
			choice.Delta.Role = "assistant"
			choice.Delta.SetContentString(wire.Text)
		}
		frame.Choices = []dto.ChatCompletionsStreamResponseChoice{choice}
		return frame, nil
	})
}
func cohereHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	return openai.NativeTextResponse(c, info, response, func(data []byte) (*dto.OpenAITextResponse, *types.GatewayError) {
		var wire *CohereResponseResult
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		if wire == nil {
			return nil, types.NewError(errors.New("Cohere response must be an object"), types.ErrorCodeBadResponseBody)
		}
		return &dto.OpenAITextResponse{Id: wire.ResponseId, Created: common.GetTimestamp(), Model: info.UpstreamModelName, Object: "chat.completion", Usage: *cohereUsage(wire.Meta), Choices: []dto.OpenAITextResponseChoice{{Index: 0, Message: dto.Message{Role: "assistant", Content: wire.Text}, FinishReason: stopReasonCohere2OpenAI(wire.FinishReason)}}}, nil
	})
}
func cohereRerankHandler(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.GatewayError) {
	if c == nil || response == nil || response.Body == nil || info == nil {
		return nil, types.NewError(errors.New("Cohere rerank response context is missing"), types.ErrorCodeBadResponseBody)
	}
	defer service.CloseResponseBodyGracefully(response)
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	var wire *CohereRerankResponseResult
	if err = json.Unmarshal(data, &wire); err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if wire == nil {
		return nil, types.NewError(errors.New("Cohere rerank response must be an object"), types.ErrorCodeBadResponseBody)
	}
	usage := cohereUsage(wire.Meta)
	if usage.PromptTokens == 0 {
		usage.PromptTokens = info.GetEstimatePromptTokens()
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	body, err := json.Marshal(dto.RerankResponse{Results: wire.Results, Usage: *usage})
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	service.IOCopyBytesGracefully(c, response, body)
	return usage, nil
}
