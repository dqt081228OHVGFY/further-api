package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dqt081228OHVGFY/further-api/common"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"strings"
)

func convertResponsesResponseForClient(c *gin.Context, info *relaycommon.RelayInfo, wire *dto.OpenAIResponsesResponse) (any, *dto.Usage, error) {
	if wire == nil {
		return nil, nil, errors.New("Responses response is missing")
	}
	if id := helper.GetResponseID(c); id != "" {
		wire.ID = id
	}
	usage := relayconvert.UsageFromResponsesUsage(wire.Usage)
	if usage == nil || usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, service.ExtractOutputTextFromResponses(wire), info.UpstreamModelName, info.GetEstimatePromptTokens())
		wire.Usage = relayconvert.UsageFromChatUsage(usage)
	}
	result, err := service.ConvertResponse(c, info, info.RelayFormat, wire)
	if err != nil {
		return nil, usage, err
	}
	if result.Usage != nil && result.Usage.TotalTokens > 0 {
		usage = result.Usage
	}
	return result.Value, usage, nil
}
func writeConvertedResponses(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response, wire *dto.OpenAIResponsesResponse) (*dto.Usage, *types.GatewayError) {
	value, usage, err := convertResponsesResponseForClient(c, info, wire)
	if err != nil {
		return usage, chatResponseError(err)
	}
	body, err := json.Marshal(value)
	if err != nil {
		return usage, chatResponseError(err)
	}
	service.IOCopyBytesGracefully(c, response, body)
	return usage, nil
}
func OaiResponsesToChatHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if err := responsesContext(c, info, response); err != nil {
		return nil, chatResponseError(err)
	}
	defer service.CloseResponseBodyGracefully(response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, chatResponseError(err)
	}
	var wire *dto.OpenAIResponsesResponse
	if err = json.Unmarshal(body, &wire); err != nil {
		return nil, chatResponseError(err)
	}
	if wire == nil {
		return nil, chatResponseError(errors.New("Responses response must be an object"))
	}
	if failure := wire.GetOpenAIError(); failure != nil && failure.Type != "" {
		return nil, types.WithOpenAIError(*failure, http.StatusBadGateway)
	}
	info.ObserveResponseModel(wire.Model)
	return writeConvertedResponses(c, info, response, wire)
}
func OaiResponsesToChatBufferedStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if err := responsesContext(c, info, response); err != nil {
		return nil, chatResponseError(err)
	}
	defer service.CloseResponseBodyGracefully(response)
	if c.Request == nil {
		return nil, chatResponseError(errors.New("buffered Responses request context is missing"))
	}
	stop := context.AfterFunc(c.Request.Context(), func() { response.Body.Close() })
	defer stop()
	info.StreamStatus = relaycommon.NewStreamStatus()
	info.StreamStatus.RequireTerminal()
	buffer := relayconvert.NewResponsesBufferedAccumulator()
	usageState := service.NewResponsesUsageAccumulator(info)
	scanner := helper.NewStreamScanner(response.Body)
	var terminal *dto.OpenAIResponsesResponse
	for scanner.Scan() {
		if err := c.Request.Context().Err(); err != nil {
			return usageState.Finish(), chatResponseError(err)
		}
		line := strings.TrimSpace(scanner.Text())
		data, found := strings.CutPrefix(line, "data:")
		if !found {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			break
		}
		if data == "" {
			continue
		}
		var event *dto.ResponsesStreamResponse
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return usageState.Finish(), chatResponseError(err)
		}
		if event == nil {
			return usageState.Finish(), chatResponseError(errors.New("Responses stream event must be an object"))
		}
		usageState.Observe(event)
		if event.Response != nil {
			info.ObserveResponseModel(event.Response.Model)
		}
		buffer.ProcessEvent(event)
		switch event.Type {
		case "response.failed", "response.error":
			if event.Response != nil {
				if failure := event.Response.GetOpenAIError(); failure != nil && failure.Type != "" {
					return usageState.Finish(), types.WithOpenAIError(*failure, http.StatusBadGateway)
				}
			}
			return usageState.Finish(), chatResponseError(fmt.Errorf("Responses stream ended with %s", event.Type))
		case "response.completed", "response.done", "response.incomplete":
			terminal = event.Response
			if terminal == nil {
				terminal = &dto.OpenAIResponsesResponse{ID: helper.GetResponseID(c), Model: info.UpstreamModelName, CreatedAt: dto.IntValue(common.GetTimestamp())}
			}
			if len(terminal.Status) == 0 {
				status := "completed"
				if event.Type == "response.incomplete" {
					status = "incomplete"
				}
				terminal.Status, _ = json.Marshal(status)
			}
		}
		if terminal != nil {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return usageState.Finish(), chatResponseError(err)
	}
	if terminal == nil {
		info.StreamStatus.MarkIncomplete("missing_terminal")
		terminal = &dto.OpenAIResponsesResponse{ID: helper.GetResponseID(c), Model: info.UpstreamModelName, CreatedAt: dto.IntValue(common.GetTimestamp()), Status: json.RawMessage(`"incomplete"`)}
	}
	buffer.SupplementResponseOutput(terminal)
	return writeConvertedResponses(c, info, response, terminal)
}
func sendConvertedStreamResults(c *gin.Context, results []relayconvert.ResponseResult) error {
	for _, result := range results {
		switch value := result.Value.(type) {
		case dto.ChatCompletionsStreamResponse:
			if len(value.Choices) > 0 || value.Usage != nil {
				if err := helper.ObjectData(c, &value); err != nil {
					return err
				}
			}
		case *dto.ChatCompletionsStreamResponse:
			if value != nil && (len(value.Choices) > 0 || value.Usage != nil) {
				if err := helper.ObjectData(c, value); err != nil {
					return err
				}
			}
		case dto.ClaudeResponse:
			if err := helper.ClaudeData(c, value); err != nil {
				return err
			}
		case *dto.ClaudeResponse:
			if value != nil {
				if err := helper.ClaudeData(c, *value); err != nil {
					return err
				}
			}
		case dto.GeminiChatResponse:
			body, err := json.Marshal(value)
			if err != nil {
				return err
			}
			if err = helper.StringData(c, string(body)); err != nil {
				return err
			}
		case *dto.GeminiChatResponse:
			if value != nil {
				body, err := json.Marshal(value)
				if err != nil {
					return err
				}
				if err = helper.StringData(c, string(body)); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("stream codec returned unsupported value %T", result.Value)
		}
	}
	return nil
}
func OaiResponsesToChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if err := responsesContext(c, info, response); err != nil {
		return nil, chatResponseError(err)
	}
	defer service.CloseResponseBodyGracefully(response)
	id, created := helper.GetResponseID(c), common.GetTimestamp()
	state, err := relayconvert.NewResponseStreamState(types.RelayFormatOpenAIResponses, info.RelayFormat, relayconvert.ResponseStreamOptions{ID: id, Model: info.UpstreamModelName, Created: created})
	if err != nil {
		return nil, chatResponseError(err)
	}
	var failure *types.GatewayError
	helper.StreamScannerHandler(c, response, info, func(data string, result *helper.StreamResult) {
		var event *dto.ResponsesStreamResponse
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			failure = chatResponseError(err)
			result.Stop(failure)
			return
		}
		if event == nil {
			failure = chatResponseError(errors.New("Responses stream event must be an object"))
			result.Stop(failure)
			return
		}
		service.ObserveResponsesOutcome(info, event)
		if event.Response != nil {
			info.ObserveResponseModel(event.Response.Model)
		}
		if event.Type == "response.failed" || event.Type == "response.error" {
			failure = chatResponseError(fmt.Errorf("Responses stream ended with %s", event.Type))
			result.Stop(failure)
			return
		}
		results, err := service.ConvertStreamResponseChunk(c, info, state, event)
		if err == nil {
			err = sendConvertedStreamResults(c, results)
		}
		if err != nil {
			failure = chatResponseError(err)
			result.Stop(failure)
		}
	})
	usage := state.Usage()
	if usage == nil || usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, state.UsageText(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		state.SetUsage(usage)
	}
	info.StreamStatus.RequireTerminal()
	if failure != nil {
		return usage, failure
	}
	if c.Request != nil && c.Request.Context().Err() != nil {
		return usage, nil
	}
	if info.StreamStatus != nil && !info.StreamStatus.IsNormalEnd() {
		return usage, nil
	}
	if info.RelayFormat == types.RelayFormatClaude {
		info.EnsureClaudeConvertInfo().Usage = usage
	}
	results, err := service.FinalizeStreamResponse(c, info, state)
	if err == nil {
		err = sendConvertedStreamResults(c, results)
	}
	if err != nil {
		return usage, chatResponseError(err)
	}
	if info.RelayFormat == types.RelayFormatOpenAI {
		if info.ShouldIncludeUsage {
			if err = helper.ObjectData(c, helper.GenerateFinalUsageResponse(id, created, info.UpstreamModelName, *usage)); err != nil {
				return usage, chatResponseError(err)
			}
		}
		helper.Done(c)
	}
	return usage, nil
}
