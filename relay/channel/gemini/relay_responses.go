package gemini

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

func GeminiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	_, wire, failure := readGeminiChat(c, info, response)
	if failure != nil {
		return nil, failure
	}
	usage := buildUsageFromGeminiResponse(c, info, wire)
	if len(wire.Candidates) == 0 {
		return &usage, emptyGeminiCandidates(c, info, wire)
	}
	result, err := service.ConvertResponse(c, info, types.RelayFormatOpenAIResponses, wire)
	if err != nil {
		return &usage, contentResponseError(err)
	}
	output, ok := result.Value.(*dto.OpenAIResponsesResponse)
	if !ok || output == nil {
		return &usage, contentResponseError(fmt.Errorf("Responses codec returned %T", result.Value))
	}
	if id := helper.GetResponseID(c); id != "" {
		output.ID = id
	}
	output.Model = info.UpstreamModelName
	output.Usage = relayconvert.UsageFromChatUsage(&usage)
	body, err := json.Marshal(output)
	if err != nil {
		return &usage, contentResponseError(err)
	}
	service.IOCopyBytesGracefully(c, response, body)
	return &usage, nil
}
func GeminiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if err := contentResponseContext(c, info, response); err != nil {
		return nil, contentResponseError(err)
	}
	codec, err := relayconvert.NewResponseStreamState(types.RelayFormatGemini, types.RelayFormatOpenAIResponses, relayconvert.ResponseStreamOptions{ID: helper.GetResponseID(c), Model: info.UpstreamModelName, Created: common.GetTimestamp(), EmitSequenceNumber: true})
	if err != nil {
		return nil, contentResponseError(err)
	}
	emitter := &openai.ResponsesEventWriter{Context: c, Info: info, State: codec}
	hosted := relayconvert.NewGeminiHostedStreamBridge()
	usage, failure := geminiStreamHandler(c, info, response, func(_ string, frame *dto.GeminiChatResponse) bool {
		hosted.Observe(frame)
		results, err := service.ConvertStreamResponseChunk(c, info, codec, frame)
		if err != nil {
			if !emitter.Fail(err) {
				emitter.Failure = contentResponseError(err)
			}
			return false
		}
		return emitter.Results(results)
	})
	if emitter.Failure != nil {
		return usage, emitter.Failure
	}
	if emitter.NativeFailureDelivered || isGeminiDownstreamStop(c, info) {
		return usage, nil
	}
	if failure != nil {
		if emitter.Fail(failure) {
			return usage, emitter.Failure
		}
		return usage, failure
	}
	if info.StreamStatus != nil && !info.StreamStatus.IsNormalEnd() {
		emitter.Fail(fmt.Errorf("Gemini stream ended before completion: %s", info.StreamStatus.Summary()))
		return usage, emitter.Failure
	}
	events, err := hosted.Finalize(codec)
	if err != nil {
		if !emitter.Fail(err) {
			emitter.Failure = contentResponseError(err)
		}
		return usage, emitter.Failure
	}
	for _, event := range events {
		if !emitter.Send(event) {
			return usage, emitter.Failure
		}
	}
	if usage == nil {
		return nil, contentResponseError(errors.New("Gemini stream usage is missing"))
	}
	codec.SetUsage(usage)
	results, err := service.FinalizeStreamResponse(c, info, codec)
	if err != nil {
		if !emitter.Fail(err) {
			emitter.Failure = contentResponseError(err)
		}
		return usage, emitter.Failure
	}
	emitter.Results(results)
	return usage, emitter.Failure
}
