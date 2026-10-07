package openai

import (
	"encoding/json"
	"fmt"
	"net/http"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// ResponsesEventWriter owns output validation and failure delivery for native
// provider bridges. The codec owns sequence numbers and terminal uniqueness.
type ResponsesEventWriter struct {
	Context                *gin.Context
	Info                   *relaycommon.RelayInfo
	State                  *relayconvert.ResponseStreamState
	Failure                *types.GatewayError
	NativeFailureDelivered bool
}

func (writer *ResponsesEventWriter) Send(event relayconvert.ChatToResponsesStreamEvent) bool {
	event.Payload.Type = event.Type
	data, err := json.Marshal(event.Payload)
	if err == nil {
		err = helper.ResponseChunkData(writer.Context, dto.ResponsesStreamResponse{Type: event.Type}, string(data))
	}
	if err != nil {
		if writer.Context.Request != nil && writer.Context.Request.Context().Err() != nil {
			writer.Info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, err)
			return false
		}
		writer.Failure = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusBadGateway)
		return false
	}
	return true
}
func (writer *ResponsesEventWriter) Results(results []relayconvert.ResponseResult) bool {
	for _, result := range results {
		event, ok := result.Value.(relayconvert.ChatToResponsesStreamEvent)
		if !ok {
			writer.Failure = types.NewOpenAIError(fmt.Errorf("Responses stream codec returned %T", result.Value), types.ErrorCodeBadResponse, http.StatusBadGateway)
			return false
		}
		if !writer.Send(event) {
			return false
		}
	}
	return true
}
func (writer *ResponsesEventWriter) Fail(err error) bool {
	if writer.NativeFailureDelivered {
		return true
	}
	results, handled := writer.State.FailResponsesStream("server_error", err.Error(), "")
	if !handled {
		return false
	}
	if writer.Results(results) {
		writer.NativeFailureDelivered = true
	}
	return true
}
