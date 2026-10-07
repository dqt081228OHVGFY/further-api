package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

func HandleStreamFormat(c *gin.Context, info *relaycommon.RelayInfo, data string, force, thinking bool) error {
	if c == nil || info == nil {
		return errors.New("chat stream context is missing")
	}
	switch info.RelayFormat {
	case types.RelayFormatOpenAI, "":
		info.IncrSendResponseCount()
		return sendStreamData(c, info, data, force, thinking)
	case types.RelayFormatClaude:
		info.IncrSendResponseCount()
		return handleClaudeFormat(c, data, info)
	case types.RelayFormatGemini:
		return handleGeminiFormat(c, data, info)
	default:
		return fmt.Errorf("chat stream cannot emit protocol %q", info.RelayFormat)
	}
}
func handleClaudeFormat(c *gin.Context, data string, info *relaycommon.RelayInfo) error {
	var frame dto.ChatCompletionsStreamResponse
	if err := json.Unmarshal([]byte(data), &frame); err != nil {
		return err
	}
	if frame.Usage != nil {
		info.EnsureClaudeConvertInfo().Usage = frame.Usage
	}
	result, err := service.ConvertStreamResponse(c, info, types.RelayFormatClaude, &frame)
	if err != nil {
		return err
	}
	frames, ok := result.Value.([]*dto.ClaudeResponse)
	if !ok {
		return fmt.Errorf("Messages stream codec returned %T", result.Value)
	}
	for _, event := range frames {
		if event != nil {
			if err := helper.ClaudeData(c, *event); err != nil {
				return err
			}
		}
	}
	return nil
}
func chatToGeminiStreamState(info *relaycommon.RelayInfo, frame *dto.ChatCompletionsStreamResponse) (*relayconvert.ResponseStreamState, error) {
	if info.ChatToGeminiStreamState != nil {
		state, ok := info.ChatToGeminiStreamState.(*relayconvert.ResponseStreamState)
		if !ok || state == nil {
			return nil, fmt.Errorf("invalid chat-to-content stream state %T", info.ChatToGeminiStreamState)
		}
		return state, nil
	}
	state, err := relayconvert.NewResponseStreamState(types.RelayFormatOpenAI, types.RelayFormatGemini, relayconvert.ResponseStreamOptions{ID: frame.Id, Model: frame.Model, Created: frame.Created})
	if err == nil {
		info.ChatToGeminiStreamState = state
	}
	return state, err
}
func sendGeminiStreamResults(c *gin.Context, results []relayconvert.ResponseResult) error {
	for _, result := range results {
		frame, ok := result.Value.(*dto.GeminiChatResponse)
		if !ok {
			return fmt.Errorf("content stream codec returned %T", result.Value)
		}
		if frame == nil {
			continue
		}
		data, err := json.Marshal(frame)
		if err != nil {
			return err
		}
		if err = helper.StringData(c, string(data)); err != nil {
			return err
		}
	}
	return nil
}
func handleGeminiFormat(c *gin.Context, data string, info *relaycommon.RelayInfo) error {
	var frame dto.ChatCompletionsStreamResponse
	if err := json.Unmarshal([]byte(data), &frame); err != nil {
		return err
	}
	state, err := chatToGeminiStreamState(info, &frame)
	if err != nil {
		return err
	}
	results, err := service.ConvertStreamResponseChunk(c, info, state, &frame)
	if err != nil {
		return err
	}
	return sendGeminiStreamResults(c, results)
}
func ProcessStreamResponse(frame dto.ChatCompletionsStreamResponse, text *strings.Builder, tools *int) error {
	if text == nil || tools == nil {
		return errors.New("chat token accumulator is missing")
	}
	for _, choice := range frame.Choices {
		text.WriteString(choice.Delta.GetContentString())
		text.WriteString(choice.Delta.GetReasoningContent())
		if len(choice.Delta.ToolCalls) > *tools {
			*tools = len(choice.Delta.ToolCalls)
		}
		for _, call := range choice.Delta.ToolCalls {
			text.WriteString(call.Function.Name)
			text.WriteString(call.Function.Arguments)
		}
	}
	return nil
}

// Completion emission is a separate operation: cancelled or broken streams
// retain their partial usage without writing a fabricated terminal frame.
func FinalizeChatStream(c *gin.Context, info *relaycommon.RelayInfo, last string, id string, created int64, model, fingerprint string, usage *dto.Usage, upstreamUsage bool) error {
	if c == nil || info == nil || usage == nil {
		return errors.New("chat finalization context is missing")
	}
	if c.Request != nil && c.Request.Context().Err() != nil {
		return nil
	}
	if info.StreamStatus != nil && !info.StreamStatus.IsNormalEnd() {
		return nil
	}
	switch info.RelayFormat {
	case types.RelayFormatOpenAI, "":
		if info.ShouldIncludeUsage && !upstreamUsage {
			frame := helper.GenerateFinalUsageResponse(id, created, model, *usage)
			frame.SetSystemFingerprint(fingerprint)
			if err := helper.ObjectData(c, frame); err != nil {
				return err
			}
		}
		helper.Done(c)
		return nil
	case types.RelayFormatClaude:
		info.EnsureClaudeConvertInfo().Usage = usage
		if err := handleClaudeFormat(c, last, info); err != nil {
			return err
		}
		info.EnsureClaudeConvertInfo().Done = true
		return nil
	case types.RelayFormatGemini:
		var frame dto.ChatCompletionsStreamResponse
		if err := json.Unmarshal([]byte(last), &frame); err != nil {
			return err
		}
		state, err := chatToGeminiStreamState(info, &frame)
		if err != nil {
			return err
		}
		state.SetUsage(usage)
		results, err := service.ConvertStreamResponseChunk(c, info, state, &frame)
		if err != nil {
			return err
		}
		if err = sendGeminiStreamResults(c, results); err != nil {
			return err
		}
		results, err = service.FinalizeStreamResponse(c, info, state)
		if err != nil {
			return err
		}
		return sendGeminiStreamResults(c, results)
	default:
		return fmt.Errorf("chat stream cannot finalize protocol %q", info.RelayFormat)
	}
}
func HandleFinalResponse(c *gin.Context, info *relaycommon.RelayInfo, last, id string, created int64, model, fingerprint string, usage *dto.Usage, upstreamUsage bool) {
	if err := FinalizeChatStream(c, info, last, id, created, model, fingerprint, usage, upstreamUsage); err != nil {
		info.StreamStatus.RecordError(err.Error())
		info.StreamStatus.MarkFailed("stream_finalize", "server_error", 500)
	}
}
func sendResponsesStreamData(c *gin.Context, frame dto.ResponsesStreamResponse, data string) {
	if data != "" {
		_ = helper.ResponseChunkData(c, frame, data)
	}
}
