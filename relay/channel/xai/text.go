package xai

import (
	"encoding/json"
	"errors"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"net/http"
)

func normalizeXAIUsage(usage *dto.Usage) {
	if usage == nil {
		return
	}
	if usage.CompletionTokens == 0 {
		usage.CompletionTokens = max(0, usage.TotalTokens-usage.PromptTokens)
	}
	usage.CompletionTokenDetails.TextTokens = max(0, usage.CompletionTokens-usage.CompletionTokenDetails.ReasoningTokens)
}
func streamResponseXAI2OpenAI(frame *dto.ChatCompletionsStreamResponse, usage *dto.Usage) *dto.ChatCompletionsStreamResponse {
	if frame == nil {
		return nil
	}
	if frame.Usage == nil && usage != nil {
		frame.Usage = usage
	}
	normalizeXAIUsage(frame.Usage)
	return frame
}
func xAIStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	return openai.NativeTextStream(c, info, response, func(data []byte) (*dto.ChatCompletionsStreamResponse, *types.GatewayError) {
		var frame *dto.ChatCompletionsStreamResponse
		if err := json.Unmarshal(data, &frame); err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		if frame == nil {
			return nil, types.NewError(errors.New("xAI stream frame must be an object"), types.ErrorCodeBadResponseBody)
		}
		normalizeXAIUsage(frame.Usage)
		return frame, nil
	})
}
func xAIHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	return openai.NativeTextResponse(c, info, response, func(data []byte) (*dto.OpenAITextResponse, *types.GatewayError) {
		var wire *dto.OpenAITextResponse
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		if wire == nil {
			return nil, types.NewError(errors.New("xAI response must be an object"), types.ErrorCodeBadResponseBody)
		}
		if failure := wire.GetOpenAIError(); failure != nil && failure.Type != "" {
			return nil, types.WithOpenAIError(*failure, http.StatusBadGateway)
		}
		normalizeXAIUsage(&wire.Usage)
		return wire, nil
	})
}
