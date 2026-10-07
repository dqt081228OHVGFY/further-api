package palm

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

func decodePaLM(data []byte) (*PaLMChatResponse, *types.GatewayError) {
	var wire *PaLMChatResponse
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if wire == nil {
		return nil, types.NewError(errors.New("PaLM response must be an object"), types.ErrorCodeBadResponseBody)
	}
	if wire.Error.Code != 0 {
		return nil, types.WithOpenAIError(types.OpenAIError{Message: wire.Error.Message, Type: wire.Error.Status, Code: wire.Error.Code}, wire.Error.Code)
	}
	if len(wire.Candidates) == 0 {
		return nil, types.NewError(errors.New("PaLM returned no candidates"), types.ErrorCodeEmptyResponse)
	}
	return wire, nil
}
func responsePaLM2OpenAI(wire *PaLMChatResponse) *dto.OpenAITextResponse {
	response := &dto.OpenAITextResponse{Object: "chat.completion", Created: common.GetTimestamp(), Choices: make([]dto.OpenAITextResponseChoice, 0, len(wire.Candidates))}
	for index, candidate := range wire.Candidates {
		response.Choices = append(response.Choices, dto.OpenAITextResponseChoice{Index: index, Message: dto.Message{Role: "assistant", Content: candidate.Content}, FinishReason: "stop"})
	}
	return response
}
func streamResponsePaLM2OpenAI(wire *PaLMChatResponse) *dto.ChatCompletionsStreamResponse {
	response := &dto.ChatCompletionsStreamResponse{Object: "chat.completion.chunk", Created: common.GetTimestamp(), Choices: make([]dto.ChatCompletionsStreamResponseChoice, 0, len(wire.Candidates))}
	for index, candidate := range wire.Candidates {
		choice := dto.ChatCompletionsStreamResponseChoice{Index: index}
		choice.Delta.SetContentString(candidate.Content)
		response.Choices = append(response.Choices, choice)
	}
	return response
}
func palmStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*types.GatewayError, string) {
	if c == nil || info == nil || !info.HasChannelMeta() || response == nil || response.Body == nil {
		return types.NewError(errors.New("PaLM stream context is missing"), types.ErrorCodeBadResponseBody), ""
	}
	defer service.CloseResponseBodyGracefully(response)
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), ""
	}
	wire, failure := decodePaLM(data)
	if failure != nil {
		return failure, ""
	}
	frame := streamResponsePaLM2OpenAI(wire)
	frame.Id = helper.GetResponseID(c)
	frame.Model = info.UpstreamModelName
	if c.Request != nil && c.Request.Context().Err() != nil {
		return nil, ""
	}
	encoded, err := json.Marshal(frame)
	if err == nil {
		err = openai.HandleStreamFormat(c, info, string(encoded), info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent)
	}
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), ""
	}
	reason := "stop"
	stop := helper.GenerateStopResponse(frame.Id, frame.Created, frame.Model, reason)
	encoded, err = json.Marshal(stop)
	if err == nil {
		err = openai.HandleStreamFormat(c, info, string(encoded), false, false)
	}
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), ""
	}
	text := ""
	for _, candidate := range wire.Candidates {
		text += candidate.Content
	}
	usage := service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
	if err = openai.FinalizeChatStream(c, info, string(encoded), frame.Id, frame.Created, frame.Model, "", usage, false); err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), text
	}
	return nil, text
}
func palmHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	return openai.NativeTextResponse(c, info, response, func(data []byte) (*dto.OpenAITextResponse, *types.GatewayError) {
		wire, failure := decodePaLM(data)
		if failure != nil {
			return nil, failure
		}
		frame := responsePaLM2OpenAI(wire)
		frame.Id = helper.GetResponseID(c)
		frame.Model = info.UpstreamModelName
		return frame, nil
	})
}
