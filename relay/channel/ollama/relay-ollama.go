package ollama

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

func toOllamaResponseFormat(format *dto.ResponseFormat) (any, error) {
	if format == nil {
		return nil, nil
	}
	switch format.Type {
	case "json", "json_object":
		return "json", nil
	case "json_schema":
		if len(format.JsonSchema) == 0 {
			return nil, nil
		}
		var schema dto.FormatJsonSchema
		if err := json.Unmarshal(format.JsonSchema, &schema); err != nil {
			return nil, fmt.Errorf("invalid Ollama output schema: %w", err)
		}
		return schema.Schema, nil
	default:
		return nil, nil
	}
}
func ollamaThinking(request *dto.GeneralOpenAIRequest) (json.RawMessage, error) {
	if len(request.Think) > 0 {
		return request.Think, nil
	}
	effort := request.ReasoningEffort
	if len(request.Reasoning) > 0 {
		var nested dto.Reasoning
		if err := json.Unmarshal(request.Reasoning, &nested); err != nil {
			return nil, err
		}
		if nested.Effort != "" {
			effort = nested.Effort
		}
	}
	if effort == "" {
		return nil, nil
	}
	var value any
	switch effort {
	case "none":
		value = false
	case "low", "medium", "high", "max":
		value = effort
	default:
		return nil, fmt.Errorf("Ollama does not support reasoning effort %q", effort)
	}
	return json.Marshal(value)
}
func ollamaOptions(request *dto.GeneralOpenAIRequest) map[string]any {
	options := make(map[string]any)
	if request.Temperature != nil {
		options["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		options["top_p"] = *request.TopP
	}
	if request.TopK != nil {
		options["top_k"] = *request.TopK
	}
	if request.FrequencyPenalty != nil {
		options["frequency_penalty"] = *request.FrequencyPenalty
	}
	if request.PresencePenalty != nil {
		options["presence_penalty"] = *request.PresencePenalty
	}
	if request.Seed != nil {
		options["seed"] = int(*request.Seed)
	}
	if tokens := request.GetMaxTokens(); tokens > 0 {
		options["num_predict"] = int(tokens)
	}
	switch stop := request.Stop.(type) {
	case string:
		options["stop"] = []string{stop}
	case []string:
		options["stop"] = append([]string(nil), stop...)
	case []any:
		values := make([]string, 0, len(stop))
		for _, item := range stop {
			if text, ok := item.(string); ok {
				values = append(values, text)
			}
		}
		options["stop"] = values
	}
	return options
}
func openAIChatToOllamaChat(c *gin.Context, request *dto.GeneralOpenAIRequest) (*OllamaChatRequest, error) {
	if request == nil {
		return nil, errors.New("Ollama chat request is missing")
	}
	thinking, err := ollamaThinking(request)
	if err != nil {
		return nil, err
	}
	format, err := toOllamaResponseFormat(request.ResponseFormat)
	if err != nil {
		return nil, err
	}
	output := &OllamaChatRequest{Model: request.Model, Stream: request.Stream != nil && *request.Stream, Options: ollamaOptions(request), Think: thinking, Format: format, Messages: make([]OllamaChatMessage, 0, len(request.Messages))}
	tools := make([]OllamaTool, 0, len(request.Tools))
	for _, tool := range request.Tools {
		tools = append(tools, OllamaTool{Type: "function", Function: OllamaToolFunction{Name: tool.Function.Name, Description: tool.Function.Description, Parameters: tool.Function.Parameters}})
	}
	if len(tools) > 0 {
		output.Tools = tools
	}
	names := make(map[string]string)
	for _, message := range request.Messages {
		converted := OllamaChatMessage{Role: message.Role}
		var text strings.Builder
		for _, part := range message.ParseContent() {
			if part.Type == dto.ContentTypeText {
				text.WriteString(part.Text)
				continue
			}
			if part.Type != dto.ContentTypeImageURL {
				return nil, fmt.Errorf("Ollama chat does not support content type %q", part.Type)
			}
			source := part.ToFileSource()
			if source == nil {
				return nil, errors.New("Ollama image source is missing")
			}
			image, _, err := service.GetBase64Data(c, source, "Ollama chat image")
			if err != nil {
				return nil, err
			}
			if image != "" {
				converted.Images = append(converted.Images, image)
			}
		}
		converted.Content = text.String()
		reason := message.GetReasoningContent()
		if message.Role == "assistant" && reason != "" {
			converted.Thinking, err = json.Marshal(reason)
			if err != nil {
				return nil, err
			}
		}
		if message.Role == "tool" {
			converted.ToolCallID = message.ToolCallId
			converted.ToolName = names[message.ToolCallId]
			if message.Name != nil && *message.Name != "" {
				converted.ToolName = *message.Name
			}
		}
		if len(message.ToolCalls) > 0 {
			var calls []dto.ToolCallRequest
			if err = json.Unmarshal(message.ToolCalls, &calls); err != nil {
				return nil, fmt.Errorf("invalid Ollama tool call list: %w", err)
			}
			for _, call := range calls {
				var arguments any
				if call.Function.Arguments != "" {
					if err = json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
						return nil, fmt.Errorf("invalid Ollama tool arguments: %w", err)
					}
				}
				if arguments == nil {
					arguments = map[string]any{}
				}
				native := OllamaToolCall{ID: call.ID}
				native.Function.Name, native.Function.Arguments = call.Function.Name, arguments
				converted.ToolCalls = append(converted.ToolCalls, native)
				if call.ID != "" {
					names[call.ID] = call.Function.Name
				}
			}
		}
		output.Messages = append(output.Messages, converted)
	}
	return output, nil
}
func openAIToGenerate(_ *gin.Context, request *dto.GeneralOpenAIRequest) (*OllamaGenerateRequest, error) {
	if request == nil {
		return nil, errors.New("Ollama completion request is missing")
	}
	format, err := toOllamaResponseFormat(request.ResponseFormat)
	if err != nil {
		return nil, err
	}
	output := &OllamaGenerateRequest{Model: request.Model, Stream: request.Stream != nil && *request.Stream, Options: ollamaOptions(request), Format: format, Think: request.Think}
	switch prompt := request.Prompt.(type) {
	case string:
		output.Prompt = prompt
	case []string:
		output.Prompt = strings.Join(prompt, "")
	case []any:
		var text strings.Builder
		for _, item := range prompt {
			if value, ok := item.(string); ok {
				text.WriteString(value)
			}
		}
		output.Prompt = text.String()
	case nil:
	default:
		output.Prompt = fmt.Sprint(prompt)
	}
	output.Suffix, _ = request.Suffix.(string)
	return output, nil
}
func requestOpenAI2Embeddings(request dto.EmbeddingRequest) *OllamaEmbeddingRequest {
	base := &dto.GeneralOpenAIRequest{Temperature: request.Temperature, TopP: request.TopP, FrequencyPenalty: request.FrequencyPenalty, PresencePenalty: request.PresencePenalty, Seed: request.Seed}
	options := ollamaOptions(base)
	dimensions := 0
	if request.Dimensions != nil {
		dimensions = *request.Dimensions
		options["dimensions"] = dimensions
	}
	values := request.ParseInput()
	if strings, ok := request.Input.([]string); ok {
		values = append([]string(nil), strings...)
	}
	var input any = values
	if len(values) == 1 {
		input = values[0]
	}
	return &OllamaEmbeddingRequest{Model: request.Model, Input: input, Options: options, Dimensions: dimensions}
}
func ollamaEmbeddingHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if c == nil || info == nil || !info.HasChannelMeta() || response == nil || response.Body == nil {
		return nil, types.NewError(errors.New("Ollama embedding context is missing"), types.ErrorCodeBadResponseBody)
	}
	defer service.CloseResponseBodyGracefully(response)
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	var wire *OllamaEmbeddingResponse
	if err = json.Unmarshal(data, &wire); err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if wire == nil {
		return nil, types.NewError(errors.New("Ollama embedding response must be an object"), types.ErrorCodeBadResponseBody)
	}
	if wire.Error != "" {
		return nil, types.NewError(errors.New(wire.Error), types.ErrorCodeBadResponseBody)
	}
	usage := dto.Usage{PromptTokens: wire.PromptEvalCount, TotalTokens: wire.PromptEvalCount}
	output := dto.OpenAIEmbeddingResponse{Object: "list", Model: info.UpstreamModelName, Usage: usage, Data: make([]dto.OpenAIEmbeddingResponseItem, 0, len(wire.Embeddings))}
	for index, embedding := range wire.Embeddings {
		output.Data = append(output.Data, dto.OpenAIEmbeddingResponseItem{Object: "embedding", Index: index, Embedding: embedding})
	}
	body, err := json.Marshal(output)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	service.IOCopyBytesGracefully(c, response, body)
	return &output.Usage, nil
}
