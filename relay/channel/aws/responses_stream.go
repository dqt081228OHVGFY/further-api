package aws

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/claude"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// Bedrock's binary event reader exposes Anthropic JSON frames. This reader
// adapts those frames to the shared SSE codec without another HTTP request or
// background producer that could outlive cancellation.
type bedrockEventBody struct {
	ctx     context.Context
	stream  *bedrockruntime.InvokeModelWithResponseStreamEventStream
	pending []byte
}

func (body *bedrockEventBody) Read(target []byte) (int, error) {
	if len(target) == 0 {
		return 0, nil
	}
	for len(body.pending) == 0 {
		select {
		case <-body.ctx.Done():
			return 0, body.ctx.Err()
		case event, open := <-body.stream.Events():
			if !open {
				if err := body.stream.Err(); err != nil {
					return 0, err
				}
				return 0, io.EOF
			}
			chunk, ok := event.(*bedrocktypes.ResponseStreamMemberChunk)
			if !ok || chunk == nil {
				return 0, errors.New("Bedrock returned an unsupported stream event")
			}
			body.pending = append([]byte("data: "), chunk.Value.Bytes...)
			body.pending = append(body.pending, '\n', '\n')
		}
	}
	n := copy(target, body.pending)
	body.pending = body.pending[n:]
	return n, nil
}
func (body *bedrockEventBody) Close() error { return body.stream.Close() }

func awsResponsesStream(c *gin.Context, info *relaycommon.RelayInfo, a *Adaptor) (*types.GatewayError, *dto.Usage) {
	if err := validateAwsOperation(c, info, a); err != nil {
		return invalidAwsOperation(err), nil
	}
	input, ok := a.AwsReq.(*bedrockruntime.InvokeModelWithResponseStreamInput)
	if !ok || input == nil {
		return invalidAwsOperation(errors.New("Bedrock stream payload has the wrong type")), nil
	}
	parent := c.Request.Context()
	ctx, cancel := newAwsInvokeContext(parent)
	defer cancel()
	output, err := a.AwsClient.InvokeModelWithResponseStream(ctx, input)
	if err != nil {
		return newAwsInvokeError(parent, err, "InvokeModelWithResponseStream"), nil
	}
	if output == nil || output.GetStream() == nil {
		return types.NewError(errors.New("Bedrock returned no response stream"), types.ErrorCodeBadResponseBody), nil
	}
	body := &bedrockEventBody{ctx: ctx, stream: output.GetStream()}
	defer body.Close()
	response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}
	info.FinalRequestRelayFormat = types.RelayFormatClaude
	usage, failure := claude.ClaudeResponsesStreamHandler(c, response, info)
	return failure, usage
}
