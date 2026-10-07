package openai

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func responsesContext(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) error {
	if c == nil || info == nil || response == nil || response.Body == nil {
		return errors.New("Responses context or body is missing")
	}
	return nil
}
func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if err := responsesContext(c, info, response); err != nil {
		return nil, chatResponseError(err)
	}
	defer service.CloseResponseBodyGracefully(response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusBadGateway)
	}
	var wire *dto.OpenAIResponsesResponse
	if err = json.Unmarshal(body, &wire); err != nil {
		return nil, chatResponseError(err)
	}
	if wire == nil {
		return nil, chatResponseError(errors.New("Responses response must be an object"))
	}
	if failure := wire.GetOpenAIError(); failure != nil && failure.Type != "" {
		status := response.StatusCode
		if status < 400 {
			status = http.StatusBadGateway
		}
		return nil, types.WithOpenAIError(*failure, status)
	}
	info.ObserveResponseModel(wire.Model)
	usage := &dto.Usage{}
	service.ApplyResponsesUsage(usage, wire.Usage)
	images := &relaycommon.ImageGenerationCallCounter{}
	for index := range wire.Output {
		output := &wire.Output[index]
		switch output.Type {
		case dto.BuildInCallWebSearchCall, dto.BuildInCallFileSearchCall:
			info.CountBillableToolCall(output.Type, "")
		case dto.BuildInCallFunctionCall:
			info.CountBillableToolCall(output.Type, output.Name)
		}
		if !relaycommon.IsNonBillableResponsesStatus(wire.Status) {
			position := index
			images.Observe(output, &position)
		}
	}
	images.Commit(info)
	body = rewriteSGLangResponsesCreatedAt(info, body, "created_at", wire.CreatedAt)
	service.IOCopyBytesGracefully(c, response, body)
	return usage, nil
}
func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if err := responsesContext(c, info, response); err != nil {
		return nil, chatResponseError(err)
	}
	defer service.CloseResponseBodyGracefully(response)
	accumulator := service.NewResponsesUsageAccumulator(info)
	var failure *types.GatewayError
	helper.StreamScannerHandler(c, response, info, func(data string, result *helper.StreamResult) {
		var frame *dto.ResponsesStreamResponse
		if err := json.Unmarshal([]byte(data), &frame); err != nil {
			failure = chatResponseError(err)
			result.Stop(failure)
			return
		}
		if frame == nil {
			failure = chatResponseError(errors.New("Responses stream frame must be an object"))
			result.Stop(failure)
			return
		}
		accumulator.Observe(frame)
		if frame.Response != nil {
			data = string(rewriteSGLangResponsesCreatedAt(info, []byte(data), "response.created_at", frame.Response.CreatedAt))
		}
		if err := helper.ResponseChunkData(c, *frame, data); err != nil {
			if c.Request != nil && c.Request.Context().Err() != nil {
				result.Stop(nil)
				return
			}
			failure = chatResponseError(err)
			result.Stop(failure)
		}
	})
	common.SetContextKey(c, constant.ContextKeyResponseStreamStatus, info.StreamStatus)
	info.StreamStatus.RequireTerminal()
	return accumulator.Finish(), failure
}
func rewriteSGLangResponsesCreatedAt(info *relaycommon.RelayInfo, payload []byte, path string, created dto.IntValue) []byte {
	if info.GetChannelType() != constant.ChannelTypeSGLang || !gjson.GetBytes(payload, path).Exists() {
		return payload
	}
	encoded, err := sjson.SetBytes(payload, path, int(created))
	if err != nil {
		return payload
	}
	return encoded
}
