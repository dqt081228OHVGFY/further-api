package ollama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

type ollamaChatStreamChunk struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	// chat
	Message *struct {
		Role      string           `json:"role"`
		Content   string           `json:"content"`
		Thinking  json.RawMessage  `json:"thinking"`
		ToolCalls []OllamaToolCall `json:"tool_calls"`
	} `json:"message"`
	// generate
	Response           string `json:"response"`
	Done               bool   `json:"done"`
	DoneReason         string `json:"done_reason"`
	TotalDuration      int64  `json:"total_duration"`
	LoadDuration       int64  `json:"load_duration"`
	PromptEvalCount    int    `json:"prompt_eval_count"`
	EvalCount          int    `json:"eval_count"`
	PromptEvalDuration int64  `json:"prompt_eval_duration"`
	EvalDuration       int64  `json:"eval_duration"`
}

func ollamaToolCallsToOpenAI(toolCalls []OllamaToolCall, startIndex int, includeIndex bool) ([]dto.ToolCallResponse, int) {
	if len(toolCalls) == 0 {
		return nil, startIndex
	}
	result := make([]dto.ToolCallResponse, 0, len(toolCalls))
	for _, tc := range toolCalls {
		var argBytes []byte
		var err error
		if tc.Function.Arguments == nil {
			argBytes = []byte("{}")
		} else {
			argBytes, err = common.Marshal(tc.Function.Arguments)
			if err != nil || len(argBytes) == 0 {
				argBytes = []byte("{}")
			}
		}
		toolCallID := tc.ID
		if toolCallID == "" {
			toolCallID = fmt.Sprintf("call_%d", startIndex)
		}
		tr := dto.ToolCallResponse{
			ID:   toolCallID,
			Type: "function",
			Function: dto.FunctionResponse{
				Name:      tc.Function.Name,
				Arguments: string(argBytes),
			},
		}
		if includeIndex {
			tr.SetIndex(startIndex)
		}
		startIndex++
		result = append(result, tr)
	}
	return result, startIndex
}

func buildOllamaStreamDelta(chunk *ollamaChatStreamChunk, responseID string, created int64, model string, toolCallIndex *int) (dto.ChatCompletionsStreamResponse, bool) {
	delta := dto.ChatCompletionsStreamResponse{
		Id:      responseID,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Index: 0,
			Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Role: "assistant"},
		}},
	}

	var content string
	if chunk.Message != nil {
		content = chunk.Message.Content
	} else {
		content = chunk.Response
	}
	if content != "" {
		delta.Choices[0].Delta.SetContentString(content)
	}

	hasPayload := content != ""
	if chunk.Message != nil && len(chunk.Message.Thinking) > 0 {
		raw := strings.TrimSpace(string(chunk.Message.Thinking))
		if raw != "" && raw != "null" {
			// Unmarshal the JSON string to get the actual content without quotes
			var thinkingContent string
			if err := common.Unmarshal(chunk.Message.Thinking, &thinkingContent); err == nil {
				delta.Choices[0].Delta.SetReasoningContent(thinkingContent)
			} else {
				// Fallback to raw string if it's not a JSON string
				delta.Choices[0].Delta.SetReasoningContent(raw)
			}
			hasPayload = true
		}
	}
	if chunk.Message != nil && len(chunk.Message.ToolCalls) > 0 {
		delta.Choices[0].Delta.ToolCalls, *toolCallIndex = ollamaToolCallsToOpenAI(chunk.Message.ToolCalls, *toolCallIndex, true)
		hasPayload = true
	}

	return delta, hasPayload
}

func toUnix(ts string) int64 {
	if ts == "" {
		return time.Now().Unix()
	}
	// try time.RFC3339 or with nanoseconds
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		t2, err2 := time.Parse(time.RFC3339, ts)
		if err2 == nil {
			return t2.Unix()
		}
		return time.Now().Unix()
	}
	return t.Unix()
}

func ollamaStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if response == nil || response.Body == nil {
		return nil, types.NewError(fmt.Errorf("Ollama response body is missing"), types.ErrorCodeBadResponseBody)
	}
	adapted := *response
	adapted.Body = openai.JSONLinesAsSSE(response.Body)
	id, created, model := helper.GetResponseID(c), common.GetTimestamp(), info.UpstreamModelName
	index := 0
	return openai.NativeTextStream(c, info, &adapted, func(data []byte) (*dto.ChatCompletionsStreamResponse, *types.GatewayError) {
		var chunk *ollamaChatStreamChunk
		if err := common.Unmarshal(data, &chunk); err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		if chunk == nil {
			return nil, types.NewError(fmt.Errorf("Ollama stream frame must be an object"), types.ErrorCodeBadResponseBody)
		}
		if chunk.Model != "" {
			model = chunk.Model
		}
		if chunk.CreatedAt != "" {
			created = toUnix(chunk.CreatedAt)
		}
		delta, _ := buildOllamaStreamDelta(chunk, id, created, model, &index)
		if chunk.Done {
			reason := chunk.DoneReason
			if reason == "" {
				reason = "stop"
			}
			if index > 0 {
				reason = constant.FinishReasonToolCalls
			}
			delta.Choices[0].FinishReason = &reason
			delta.Usage = &dto.Usage{PromptTokens: chunk.PromptEvalCount, CompletionTokens: chunk.EvalCount, TotalTokens: chunk.PromptEvalCount + chunk.EvalCount}
		}
		return &delta, nil
	})
}

// non-stream handler for chat/generate
func ollamaChatHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	return openai.NativeTextResponse(c, info, response, func(data []byte) (*dto.OpenAITextResponse, *types.GatewayError) {
		decoder := json.NewDecoder(bytes.NewReader(data))
		var text, thinking strings.Builder
		var last *ollamaChatStreamChunk
		index := 0
		var calls []dto.ToolCallResponse
		for {
			var chunk *ollamaChatStreamChunk
			if err := decoder.Decode(&chunk); err == io.EOF {
				break
			} else if err != nil {
				return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
			}
			if chunk == nil {
				return nil, types.NewError(fmt.Errorf("Ollama response must contain objects"), types.ErrorCodeBadResponseBody)
			}
			last = chunk
			if chunk.Message == nil {
				text.WriteString(chunk.Response)
			} else {
				text.WriteString(chunk.Message.Content)
				raw := strings.TrimSpace(string(chunk.Message.Thinking))
				if raw != "" && raw != "null" {
					var value string
					if json.Unmarshal(chunk.Message.Thinking, &value) == nil {
						thinking.WriteString(value)
					} else {
						thinking.WriteString(raw)
					}
				}
				converted, next := ollamaToolCallsToOpenAI(chunk.Message.ToolCalls, index, false)
				calls = append(calls, converted...)
				index = next
			}
		}
		if last == nil {
			return nil, types.NewError(fmt.Errorf("Ollama returned no response objects"), types.ErrorCodeBadResponseBody)
		}
		model := last.Model
		if model == "" {
			model = info.UpstreamModelName
		}
		reason := last.DoneReason
		if reason == "" {
			reason = "stop"
		}
		if len(calls) > 0 {
			reason = constant.FinishReasonToolCalls
		}
		message := dto.Message{Role: "assistant", Content: contentPtr(text.String())}
		if len(calls) > 0 {
			encoded, err := json.Marshal(calls)
			if err != nil {
				return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
			}
			message.ToolCalls = encoded
		}
		if value := thinking.String(); value != "" {
			message.ReasoningContent = &value
		}
		return &dto.OpenAITextResponse{Id: helper.GetResponseID(c), Object: "chat.completion", Created: toUnix(last.CreatedAt), Model: model, Choices: []dto.OpenAITextResponseChoice{{Index: 0, Message: message, FinishReason: reason}}, Usage: dto.Usage{PromptTokens: last.PromptEvalCount, CompletionTokens: last.EvalCount, TotalTokens: last.PromptEvalCount + last.EvalCount}}, nil
	})
}

func contentPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
