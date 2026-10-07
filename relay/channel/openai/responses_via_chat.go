package openai

import (
	"encoding/json"
	"errors"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

func OaiChatToResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if err := responsesContext(c, info, response); err != nil {
		return nil, chatResponseError(err)
	}
	defer service.CloseResponseBodyGracefully(response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, chatResponseError(err)
	}
	var chat *dto.OpenAITextResponse
	if err = json.Unmarshal(body, &chat); err != nil {
		return nil, chatResponseError(err)
	}
	if chat == nil {
		return nil, chatResponseError(errors.New("chat response must be an object"))
	}
	if failure := chat.GetOpenAIError(); failure != nil && failure.Type != "" {
		return nil, types.WithOpenAIError(*failure, http.StatusBadGateway)
	}
	info.ObserveResponseModel(chat.Model)
	if id := helper.GetResponseID(c); id != "" {
		chat.Id = id
	}
	converted, err := service.ConvertResponse(c, info, types.RelayFormatOpenAIResponses, chat)
	if err != nil {
		return nil, chatResponseError(err)
	}
	wire, ok := converted.Value.(*dto.OpenAIResponsesResponse)
	if !ok || wire == nil {
		return nil, chatResponseError(errors.New("Responses codec returned no response"))
	}
	usage := converted.Usage
	if usage == nil || usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, service.ExtractOutputTextFromResponses(wire), info.UpstreamModelName, info.GetEstimatePromptTokens())
		wire.Usage = relayconvert.UsageFromChatUsage(usage)
	}
	body, err = json.Marshal(wire)
	if err != nil {
		return usage, chatResponseError(err)
	}
	service.IOCopyBytesGracefully(c, response, body)
	return usage, nil
}
func OaiChatToResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if err := responsesContext(c, info, response); err != nil {
		return nil, chatResponseError(err)
	}
	defer service.CloseResponseBodyGracefully(response)
	codec, err := relayconvert.NewResponseStreamState(types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, relayconvert.ResponseStreamOptions{ID: helper.GetResponseID(c), Model: info.UpstreamModelName, EmitSequenceNumber: true})
	if err != nil {
		return nil, chatResponseError(err)
	}
	writer := &ResponsesEventWriter{Context: c, Info: info, State: codec}
	helper.StreamScannerHandler(c, response, info, func(data string, result *helper.StreamResult) {
		stop := func(err error) {
			if !writer.Fail(err) {
				writer.Failure = chatResponseError(err)
			}
			result.Stop(writer.Failure)
		}
		var envelope dto.OpenAITextResponse
		if json.Unmarshal([]byte(data), &envelope) == nil {
			if failure := envelope.GetOpenAIError(); failure != nil && failure.Type != "" {
				stop(errors.New(failure.Message))
				return
			}
		}
		var frame *dto.ChatCompletionsStreamResponse
		if err := json.Unmarshal([]byte(data), &frame); err != nil {
			stop(err)
			return
		}
		if frame == nil {
			stop(errors.New("chat stream frame must be an object"))
			return
		}
		info.ObserveResponseModel(frame.Model)
		results, err := service.ConvertStreamResponseChunk(c, info, codec, frame)
		if err != nil {
			stop(err)
			return
		}
		if !writer.Results(results) {
			result.Stop(writer.Failure)
		}
	})
	usage := codec.Usage()
	if usage == nil || usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, codec.UsageText(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		codec.SetUsage(usage)
	}
	if writer.Failure != nil {
		return usage, writer.Failure
	}
	if writer.NativeFailureDelivered {
		return usage, nil
	}
	if c.Request != nil && c.Request.Context().Err() != nil {
		return usage, nil
	}
	if info.StreamStatus != nil && !info.StreamStatus.IsNormalEnd() {
		return usage, nil
	}
	results, err := service.FinalizeStreamResponse(c, info, codec)
	if err != nil {
		if !writer.Fail(err) {
			writer.Failure = chatResponseError(err)
		}
		return usage, writer.Failure
	}
	writer.Results(results)
	return usage, writer.Failure
}
