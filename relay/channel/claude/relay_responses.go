package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

func ClaudeResponsesStreamHandler(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.GatewayError) {
	if c == nil || info == nil || response == nil || response.Body == nil {
		return nil, messagesResponseError(errors.New("Messages Responses bridge context is missing"))
	}
	defer service.CloseResponseBodyGracefully(response)
	model := info.OriginModelName
	if info.HasChannelMeta() {
		model = info.UpstreamModelName
	}
	id, created := helper.GetResponseID(c), common.GetTimestamp()
	codec, err := relayconvert.NewResponseStreamState(types.RelayFormatClaude, types.RelayFormatOpenAIResponses, relayconvert.ResponseStreamOptions{ID: id, Model: model, Created: created, EmitSequenceNumber: true})
	if err != nil {
		return nil, messagesResponseError(err)
	}
	emitter := &openai.ResponsesEventWriter{Context: c, Info: info, State: codec}
	hosted := relayconvert.NewClaudeHostedStreamBridge()
	usageState := &ClaudeResponseInfo{ResponseId: id, Created: created, Model: model, Usage: &dto.Usage{}}
	stopWithFailure := func(err error, result *helper.StreamResult) {
		if !emitter.Fail(err) {
			emitter.Failure = messagesResponseError(err)
		}
		result.Stop(emitter.Failure)
	}
	helper.StreamScannerHandler(c, response, info, func(data string, result *helper.StreamResult) {
		var frame *dto.ClaudeResponse
		if err := json.Unmarshal([]byte(data), &frame); err != nil {
			stopWithFailure(err, result)
			return
		}
		if frame == nil {
			stopWithFailure(errors.New("Messages stream frame must be an object"), result)
			return
		}
		if failure := frame.GetClaudeError(); failure != nil && failure.Type != "" {
			stopWithFailure(errors.New(failure.Message), result)
			return
		}
		observeMessagesFrame(c, info, frame)
		if frame.Type == "message_start" && frame.Message != nil && info.HasChannelMeta() {
			info.UpstreamModelName = frame.Message.Model
		}
		FormatClaudeResponseInfo(frame, nil, usageState)
		countClaudeStreamBillableTools(c, info, frame)
		events, consumed, err := hosted.Convert(frame, codec)
		if err != nil {
			stopWithFailure(err, result)
			return
		}
		for _, event := range events {
			if !emitter.Send(event) {
				result.Stop(emitter.Failure)
				return
			}
		}
		if consumed {
			return
		}
		results, err := service.ConvertStreamResponseChunk(c, info, codec, frame)
		if err != nil {
			stopWithFailure(err, result)
			return
		}
		if !emitter.Results(results) {
			result.Stop(emitter.Failure)
		}
	})
	info.StreamStatus.RequireTerminal()
	settleMessagesStream(c, info, usageState)
	if emitter.Failure != nil {
		return usageState.Usage, emitter.Failure
	}
	if emitter.NativeFailureDelivered {
		return usageState.Usage, nil
	}
	if c.Request != nil && c.Request.Context().Err() != nil {
		return usageState.Usage, nil
	}
	if info.StreamStatus != nil && !info.StreamStatus.IsNormalEnd() {
		emitter.Fail(fmt.Errorf("Messages stream ended before completion: %s", info.StreamStatus.Summary()))
		return usageState.Usage, emitter.Failure
	}
	mapped := buildOpenAIStyleUsageFromClaudeUsage(usageState.Usage)
	codec.SetUsage(&mapped)
	results, err := service.FinalizeStreamResponse(c, info, codec)
	if err != nil {
		if !emitter.Fail(err) {
			emitter.Failure = messagesResponseError(err)
		}
		return usageState.Usage, emitter.Failure
	}
	emitter.Results(results)
	return usageState.Usage, emitter.Failure
}
