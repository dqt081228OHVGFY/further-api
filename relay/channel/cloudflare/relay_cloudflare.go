package cloudflare

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

func convertCf2CompletionsRequest(request dto.GeneralOpenAIRequest) *CfRequest {
	prompt, _ := request.Prompt.(string)
	stream := request.Stream != nil && *request.Stream
	return &CfRequest{Prompt: prompt, MaxTokens: request.GetMaxTokens(), Stream: stream, Temperature: request.Temperature}
}
func cloudflareError(data []byte) *types.GatewayError {
	var wire struct {
		Success *bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Error any `json:"error"`
	}
	if json.Unmarshal(data, &wire) != nil {
		return nil
	}
	if failure := dto.GetOpenAIError(wire.Error); failure != nil && failure.Type != "" {
		return types.WithOpenAIError(*failure, http.StatusBadGateway)
	}
	if wire.Success != nil && !*wire.Success {
		message := "Cloudflare request failed"
		if len(wire.Errors) > 0 && wire.Errors[0].Message != "" {
			message = wire.Errors[0].Message
		}
		return types.NewError(errors.New(message), types.ErrorCodeBadResponseBody)
	}
	return nil
}
func cfStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*types.GatewayError, *dto.Usage) {
	usage, failure := openai.NativeTextStream(c, info, response, func(data []byte) (*dto.ChatCompletionsStreamResponse, *types.GatewayError) {
		if failure := cloudflareError(data); failure != nil {
			return nil, failure
		}
		var frame *dto.ChatCompletionsStreamResponse
		if err := json.Unmarshal(data, &frame); err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		if frame == nil {
			return nil, types.NewError(errors.New("Cloudflare stream frame must be an object"), types.ErrorCodeBadResponseBody)
		}
		if len(frame.Choices) == 0 {
			var native struct {
				Response string `json:"response"`
			}
			if err := json.Unmarshal(data, &native); err != nil {
				return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
			}
			if native.Response != "" {
				frame.Choices = []dto.ChatCompletionsStreamResponseChoice{{Index: 0, Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Role: "assistant", Content: &native.Response}}}
			}
		}
		if frame.Id == "" {
			frame.Id = helper.GetResponseID(c)
		}
		frame.Model = info.UpstreamModelName
		for index := range frame.Choices {
			frame.Choices[index].Delta.Role = "assistant"
		}
		return frame, nil
	})
	return failure, usage
}
func cfHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*types.GatewayError, *dto.Usage) {
	if info != nil && info.RelayMode == constant.RelayModeEmbeddings {
		usage, failure := openai.OpenaiHandler(c, info, response)
		return failure, usage
	}
	usage, failure := openai.NativeTextResponse(c, info, response, func(data []byte) (*dto.OpenAITextResponse, *types.GatewayError) {
		if failure := cloudflareError(data); failure != nil {
			return nil, failure
		}
		var wire *dto.OpenAITextResponse
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		if wire == nil {
			return nil, types.NewError(errors.New("Cloudflare response must be an object"), types.ErrorCodeBadResponseBody)
		}
		if len(wire.Choices) == 0 {
			var native struct {
				Result struct {
					Response string `json:"response"`
				} `json:"result"`
			}
			if err := json.Unmarshal(data, &native); err != nil {
				return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
			}
			wire.Choices = []dto.OpenAITextResponseChoice{{Index: 0, Message: dto.Message{Role: "assistant", Content: native.Result.Response}, FinishReason: "stop"}}
		}
		if wire.Id == "" {
			wire.Id = helper.GetResponseID(c)
		}
		wire.Model = info.UpstreamModelName
		return wire, nil
	})
	return failure, usage
}
func cfSTTHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*types.GatewayError, *dto.Usage) {
	if c == nil || info == nil || !info.HasChannelMeta() || response == nil || response.Body == nil {
		return types.NewError(errors.New("Cloudflare audio response context is missing"), types.ErrorCodeBadResponseBody), nil
	}
	defer service.CloseResponseBodyGracefully(response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	if failure := cloudflareError(body); failure != nil {
		return failure, nil
	}
	var wire *CfAudioResponse
	if err = json.Unmarshal(body, &wire); err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	if wire == nil {
		return types.NewError(errors.New("Cloudflare audio response must be an object"), types.ErrorCodeBadResponseBody), nil
	}
	output, err := json.Marshal(&dto.AudioResponse{Text: wire.Result.Text})
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	service.IOCopyBytesGracefully(c, response, output)
	return nil, service.ResponseText2Usage(c, wire.Result.Text, info.UpstreamModelName, info.GetEstimatePromptTokens())
}
