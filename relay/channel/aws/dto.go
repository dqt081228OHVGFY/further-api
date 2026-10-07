package aws

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

type AwsClaudeRequest struct {
	AnthropicVersion  string              `json:"anthropic_version"`
	AnthropicBeta     json.RawMessage     `json:"anthropic_beta,omitempty"`
	System            any                 `json:"system,omitempty"`
	Messages          []dto.ClaudeMessage `json:"messages"`
	MaxTokens         *uint               `json:"max_tokens,omitempty"`
	Temperature       *float64            `json:"temperature,omitempty"`
	TopP              *float64            `json:"top_p,omitempty"`
	TopK              *int                `json:"top_k,omitempty"`
	StopSequences     []string            `json:"stop_sequences,omitempty"`
	Tools             any                 `json:"tools,omitempty"`
	ToolChoice        any                 `json:"tool_choice,omitempty"`
	ContextManagement json.RawMessage     `json:"context_management,omitempty"`
	Thinking          *dto.Thinking       `json:"thinking,omitempty"`
	OutputConfig      json.RawMessage     `json:"output_config,omitempty"`
}

func formatRequest(body io.Reader, headers http.Header) (*AwsClaudeRequest, error) {
	if body == nil {
		return nil, errors.New("Bedrock messages body is missing")
	}
	var request *AwsClaudeRequest
	if err := json.NewDecoder(body).Decode(&request); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, errors.New("Bedrock messages body must be an object")
	}
	request.AnthropicVersion = "bedrock-2023-05-31"
	var flags []string
	seen := make(map[string]bool)
	for _, flag := range strings.Split(headers.Get("Anthropic-Beta"), ",") {
		flag = strings.TrimSpace(flag)
		if flag != "" && !seen[flag] {
			seen[flag] = true
			flags = append(flags, flag)
		}
	}
	if len(flags) != 0 {
		encoded, err := json.Marshal(flags)
		if err != nil {
			return nil, err
		}
		request.AnthropicBeta = encoded
	}
	return request, nil
}

type NovaMessage struct {
	Role    string        `json:"role"`
	Content []NovaContent `json:"content"`
}
type NovaContent struct {
	Text       string         `json:"text,omitempty"`
	Image      map[string]any `json:"image,omitempty"`
	ToolUse    map[string]any `json:"toolUse,omitempty"`
	ToolResult map[string]any `json:"toolResult,omitempty"`
}
type NovaRequest struct {
	SchemaVersion   string               `json:"schemaVersion"`
	System          []NovaContent        `json:"system,omitempty"`
	Messages        []NovaMessage        `json:"messages"`
	InferenceConfig *NovaInferenceConfig `json:"inferenceConfig,omitempty"`
	ToolConfig      map[string]any       `json:"toolConfig,omitempty"`
}
type NovaInferenceConfig struct {
	MaxTokens     *uint    `json:"maxTokens,omitempty"`
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"topP,omitempty"`
	TopK          *int     `json:"topK,omitempty"`
	StopSequences []string `json:"stopSequences,omitempty"`
}

func novaRequest(c *gin.Context, request *dto.GeneralOpenAIRequest) (*NovaRequest, error) {
	if request == nil {
		return nil, errors.New("Nova chat request is missing")
	}
	result := &NovaRequest{SchemaVersion: "messages-v1", Messages: make([]NovaMessage, 0, len(request.Messages))}
	for _, message := range request.Messages {
		blocks := make([]NovaContent, 0)
		if message.Role == "tool" {
			if message.ToolCallId == "" {
				return nil, errors.New("Nova tool result requires a tool_call_id")
			}
			blocks = append(blocks, NovaContent{ToolResult: map[string]any{"toolUseId": message.ToolCallId, "content": []map[string]string{{"text": message.StringContent()}}}})
		} else {
			for _, part := range message.ParseContent() {
				switch part.Type {
				case dto.ContentTypeText:
					blocks = append(blocks, NovaContent{Text: part.Text})
				case dto.ContentTypeImageURL:
					if c == nil {
						return nil, errors.New("Nova image conversion requires a request context")
					}
					source := part.ToFileSource()
					if source == nil {
						return nil, errors.New("Nova image source is missing")
					}
					data, mediaType, err := service.GetBase64Data(c, source, "Nova image conversion")
					if err != nil {
						return nil, err
					}
					format := strings.TrimPrefix(mediaType, "image/")
					if format == "jpg" {
						format = "jpeg"
					}
					blocks = append(blocks, NovaContent{Image: map[string]any{"format": format, "source": map[string]string{"bytes": data}}})
				default:
					return nil, fmt.Errorf("Nova messages do not support content type %q", part.Type)
				}
			}
			if len(message.ToolCalls) != 0 {
				var calls []dto.ToolCallRequest
				if err := json.Unmarshal(message.ToolCalls, &calls); err != nil {
					return nil, fmt.Errorf("decode Nova tool calls: %w", err)
				}
				for _, call := range calls {
					var arguments any
					if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
						return nil, fmt.Errorf("decode Nova tool arguments: %w", err)
					}
					blocks = append(blocks, NovaContent{ToolUse: map[string]any{"toolUseId": call.ID, "name": call.Function.Name, "input": arguments}})
				}
			}
		}
		if message.Role == "system" || message.Role == "developer" {
			result.System = append(result.System, blocks...)
			continue
		}
		role := message.Role
		if role == "tool" {
			role = "user"
		}
		if role != "user" && role != "assistant" {
			return nil, fmt.Errorf("Nova messages do not support role %q", message.Role)
		}
		if len(blocks) == 0 {
			continue
		}
		if n := len(result.Messages); n > 0 && result.Messages[n-1].Role == role {
			result.Messages[n-1].Content = append(result.Messages[n-1].Content, blocks...)
		} else {
			result.Messages = append(result.Messages, NovaMessage{Role: role, Content: blocks})
		}
	}
	limit := request.MaxCompletionTokens
	if limit == nil || *limit == 0 {
		limit = request.MaxTokens
	}
	if limit != nil || request.Temperature != nil || request.TopP != nil || request.TopK != nil || request.Stop != nil {
		result.InferenceConfig = &NovaInferenceConfig{MaxTokens: limit, Temperature: request.Temperature, TopP: request.TopP, TopK: request.TopK, StopSequences: parseStopSequences(request.Stop)}
	}
	if len(request.Tools) != 0 {
		tools := make([]any, 0, len(request.Tools))
		for _, tool := range request.Tools {
			if tool.Type != "function" {
				return nil, errors.New("Nova supports function tools only")
			}
			tools = append(tools, map[string]any{"toolSpec": map[string]any{"name": tool.Function.Name, "description": tool.Function.Description, "inputSchema": map[string]any{"json": tool.Function.Parameters}}})
		}
		result.ToolConfig = map[string]any{"tools": tools}
	}
	return result, nil
}
func parseStopSequences(value any) []string {
	result := make([]string, 0)
	add := func(text string) {
		if text != "" {
			result = append(result, text)
		}
	}
	switch values := value.(type) {
	case string:
		add(values)
	case []string:
		for _, text := range values {
			add(text)
		}
	case []any:
		for _, item := range values {
			if text, ok := item.(string); ok {
				add(text)
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
