package aws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/claude"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/setting/model_setting"
	"github.com/gin-gonic/gin"
)

func getAwsErrorStatusCode(err error) int {
	var upstream interface{ HTTPStatusCode() int }
	if errors.As(err, &upstream) && upstream.HTTPStatusCode() > 0 {
		return upstream.HTTPStatusCode()
	}
	return http.StatusInternalServerError
}
func newAwsInvokeContext(parent context.Context) (context.Context, context.CancelFunc) {
	if common.RelayTimeout > 0 {
		return context.WithTimeout(parent, time.Duration(common.RelayTimeout)*time.Second)
	}
	return context.WithCancel(parent)
}
func newAwsInvokeError(requestContext context.Context, err error, operation string) *types.GatewayError {
	var existing *types.GatewayError
	if errors.As(err, &existing) {
		return existing
	}
	var options []types.GatewayErrorOptions
	if requestContext.Err() != nil {
		options = append(options, types.ErrOptionWithSkipRetry())
	}
	return types.NewOpenAIError(fmt.Errorf("%s: %w", operation, err), types.ErrorCodeAwsInvokeError, getAwsErrorStatusCode(err), options...)
}
func newAwsClient(_ *gin.Context, info *relaycommon.RelayInfo) (*bedrockruntime.Client, error) {
	return bedrockClient(info)
}

// Preparation owns no network request. The exact SDK operation is committed
// only after credentials, header overrides and the payload have all validated.
func doAwsClientRequest(c *gin.Context, info *relaycommon.RelayInfo, a *Adaptor, body io.Reader) (any, error) {
	if a == nil || c == nil || c.Request == nil || info == nil || !info.HasChannelMeta() || body == nil {
		return nil, types.NewError(errors.New("Bedrock request configuration is incomplete"), types.ErrorCodeBadRequestBody)
	}
	a.AwsReq, a.AwsClient, a.AwsModelId = nil, nil, ""
	client, err := newAwsClient(c, info)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeChannelAwsClientError)
	}
	model := getAwsModelID(info.UpstreamModelName)
	prefix := getAwsRegionPrefix(client.Options().Region)
	if awsModelCanCrossRegion(model, prefix) {
		model = awsModelCrossRegion(model, prefix)
	}
	headers := make(http.Header)
	if err = a.SetupRequestHeader(c, &headers, info); err != nil {
		return nil, err
	}
	overrides, err := channel.ResolveHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	for name, value := range overrides {
		headers.Set(name, value)
	}
	nova := isNovaModel(model)
	var payload []byte
	if nova {
		var request NovaRequest
		if err = json.NewDecoder(body).Decode(&request); err == nil && len(request.Messages) == 0 {
			err = errors.New("Nova request requires messages")
		}
		if err == nil {
			payload, err = json.Marshal(request)
		}
	} else {
		var request *AwsClaudeRequest
		request, err = formatRequest(body, headers)
		if err == nil {
			payload, err = buildAwsRequestBody(c, info, request)
		}
	}
	if err != nil {
		return nil, types.NewError(fmt.Errorf("prepare Bedrock payload: %w", err), types.ErrorCodeBadRequestBody)
	}
	var operation any
	if info.IsStream && !nova {
		operation = &bedrockruntime.InvokeModelWithResponseStreamInput{ModelId: sdkaws.String(model), Accept: sdkaws.String("application/json"), ContentType: sdkaws.String("application/json"), Body: payload}
	} else {
		operation = &bedrockruntime.InvokeModelInput{ModelId: sdkaws.String(model), Accept: sdkaws.String("application/json"), ContentType: sdkaws.String("application/json"), Body: payload}
	}
	a.AwsClient, a.AwsReq, a.AwsModelId, a.IsNova = client, operation, model, nova
	return nil, nil
}

func buildAwsRequestBody(c *gin.Context, info *relaycommon.RelayInfo, request any) ([]byte, error) {
	if info == nil || !info.HasChannelMeta() {
		return nil, errors.New("Bedrock provider configuration is missing")
	}
	if !model_setting.GetGlobalSettings().PassThroughRequestEnabled && !info.ChannelSetting.PassThroughBodyEnabled {
		return json.Marshal(request)
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, fmt.Errorf("read Bedrock pass-through body: %w", err)
	}
	body, err := storage.Bytes()
	if err != nil {
		return nil, err
	}
	var object map[string]json.RawMessage
	if err = json.Unmarshal(body, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, errors.New("Bedrock pass-through body must be an object")
	}
	delete(object, "model")
	delete(object, "stream")
	return json.Marshal(object)
}

func getAwsRegionPrefix(region string) string {
	prefix, _, _ := strings.Cut(region, "-")
	return prefix
}
func awsModelCanCrossRegion(model, prefix string) bool {
	return awsModelCanCrossRegionMap[model][prefix]
}
func awsModelCrossRegion(model, prefix string) string {
	if cross, ok := awsRegionCrossModelPrefixMap[prefix]; ok {
		return cross + "." + model
	}
	return model
}
func getAwsModelID(model string) string {
	if mapped, ok := awsModelIDMap[model]; ok {
		return mapped
	}
	return model
}

func awsResponseState(c *gin.Context, info *relaycommon.RelayInfo) *claude.ClaudeResponseInfo {
	return &claude.ClaudeResponseInfo{ResponseId: helper.GetResponseID(c), Created: common.GetTimestamp(), Model: info.UpstreamModelName, Usage: &dto.Usage{}}
}
func validateAwsOperation(c *gin.Context, info *relaycommon.RelayInfo, a *Adaptor) error {
	if c == nil || c.Request == nil || info == nil || !info.HasChannelMeta() || a == nil || a.AwsClient == nil || a.AwsReq == nil {
		return errors.New("Bedrock operation was not prepared")
	}
	return nil
}
func invalidAwsOperation(err error) *types.GatewayError {
	return types.NewError(err, types.ErrorCodeBadRequestBody)
}

func awsHandler(c *gin.Context, info *relaycommon.RelayInfo, a *Adaptor) (*types.GatewayError, *dto.Usage) {
	if err := validateAwsOperation(c, info, a); err != nil {
		return invalidAwsOperation(err), nil
	}
	input, ok := a.AwsReq.(*bedrockruntime.InvokeModelInput)
	if !ok || input == nil {
		return invalidAwsOperation(errors.New("Bedrock invoke payload has the wrong type")), nil
	}
	parent := c.Request.Context()
	ctx, cancel := newAwsInvokeContext(parent)
	defer cancel()
	output, err := a.AwsClient.InvokeModel(ctx, input)
	if err != nil {
		return newAwsInvokeError(parent, err, "InvokeModel"), nil
	}
	if output == nil {
		return types.NewError(errors.New("Bedrock returned no invoke output"), types.ErrorCodeBadResponseBody), nil
	}
	state := awsResponseState(c, info)
	if output.ContentType != nil && *output.ContentType != "" {
		c.Writer.Header().Set("Content-Type", *output.ContentType)
	}
	info.FinalRequestRelayFormat = types.RelayFormatClaude
	if err := claude.HandleClaudeResponseData(c, info, state, nil, output.Body); err != nil {
		return err, nil
	}
	return nil, state.Usage
}

func awsStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, a *Adaptor) (*types.GatewayError, *dto.Usage) {
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
	stream := output.GetStream()
	defer stream.Close()
	state := awsResponseState(c, info)
	info.FinalRequestRelayFormat = types.RelayFormatClaude
	events := stream.Events()
	for {
		select {
		case <-ctx.Done():
			// Keep received usage after a disconnect, but never emit a final frame.
			if parent.Err() != nil {
				return nil, state.Usage
			}
			return newAwsInvokeError(parent, ctx.Err(), "read Bedrock response stream"), state.Usage
		case event, open := <-events:
			if ctx.Err() != nil {
				if parent.Err() != nil {
					return nil, state.Usage
				}
				return newAwsInvokeError(parent, ctx.Err(), "read Bedrock response stream"), state.Usage
			}
			if !open {
				if err := stream.Err(); err != nil {
					return newAwsInvokeError(parent, err, "read Bedrock response stream"), state.Usage
				}
				claude.HandleStreamFinalResponse(c, info, state)
				return nil, state.Usage
			}
			chunk, ok := event.(*bedrocktypes.ResponseStreamMemberChunk)
			if !ok || chunk == nil {
				return types.NewError(errors.New("Bedrock returned an unsupported stream event"), types.ErrorCodeBadResponseBody), state.Usage
			}
			info.SetFirstResponseTime()
			if err := claude.HandleStreamResponseData(c, info, state, string(chunk.Value.Bytes)); err != nil {
				return err, state.Usage
			}
		}
	}
}

func handleNovaRequest(c *gin.Context, info *relaycommon.RelayInfo, a *Adaptor) (*types.GatewayError, *dto.Usage) {
	if err := validateAwsOperation(c, info, a); err != nil {
		return invalidAwsOperation(err), nil
	}
	input, ok := a.AwsReq.(*bedrockruntime.InvokeModelInput)
	if !ok || input == nil {
		return invalidAwsOperation(errors.New("Nova invoke payload has the wrong type")), nil
	}
	parent := c.Request.Context()
	ctx, cancel := newAwsInvokeContext(parent)
	defer cancel()
	output, err := a.AwsClient.InvokeModel(ctx, input)
	if err != nil {
		return newAwsInvokeError(parent, err, "InvokeModel"), nil
	}
	if output == nil {
		return types.NewError(errors.New("Nova returned no invoke output"), types.ErrorCodeBadResponseBody), nil
	}
	var wire struct {
		Output struct {
			Message struct {
				Content []NovaContent `json:"content"`
			} `json:"message"`
		} `json:"output"`
		Usage struct {
			InputTokens  int `json:"inputTokens"`
			OutputTokens int `json:"outputTokens"`
			TotalTokens  int `json:"totalTokens"`
		} `json:"usage"`
		StopReason string `json:"stopReason"`
	}
	if err = json.Unmarshal(output.Body, &wire); err != nil {
		return types.NewError(fmt.Errorf("decode Nova response: %w", err), types.ErrorCodeBadResponseBody), nil
	}
	if len(wire.Output.Message.Content) == 0 {
		return types.NewError(errors.New("Nova response contains no content"), types.ErrorCodeBadResponseBody), nil
	}
	var text strings.Builder
	var calls []dto.ToolCallRequest
	for _, block := range wire.Output.Message.Content {
		text.WriteString(block.Text)
		if block.ToolUse != nil {
			id, _ := block.ToolUse["toolUseId"].(string)
			name, _ := block.ToolUse["name"].(string)
			arguments, err := json.Marshal(block.ToolUse["input"])
			if err != nil {
				return types.NewError(err, types.ErrorCodeBadResponseBody), nil
			}
			calls = append(calls, dto.ToolCallRequest{ID: id, Type: "function", Function: dto.FunctionRequest{Name: name, Arguments: string(arguments)}})
		}
	}
	finish := "stop"
	if wire.StopReason == "max_tokens" {
		finish = "length"
	}
	if len(calls) > 0 {
		finish = "tool_calls"
	}
	usage := dto.Usage{PromptTokens: wire.Usage.InputTokens, CompletionTokens: wire.Usage.OutputTokens, TotalTokens: wire.Usage.TotalTokens}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	response := dto.OpenAITextResponse{Id: helper.GetResponseID(c), Object: "chat.completion", Created: common.GetTimestamp(), Model: info.UpstreamModelName, Choices: []dto.OpenAITextResponseChoice{{Index: 0, Message: dto.Message{Role: "assistant", Content: text.String()}, FinishReason: finish}}, Usage: usage}
	if len(calls) > 0 {
		response.Choices[0].Message.SetToolCalls(calls)
	}
	encoded, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		return types.NewError(marshalErr, types.ErrorCodeBadResponseBody), nil
	}
	if info.IsStream {
		created := common.GetTimestamp()
		content := helper.GenerateStartEmptyResponse(response.Id, created, response.Model, nil)
		content.Choices[0].Delta.SetContentString(text.String())
		for index, call := range calls {
			tool := dto.ToolCallResponse{ID: call.ID, Type: "function", Function: dto.FunctionResponse{Name: call.Function.Name, Arguments: call.Function.Arguments}}
			tool.SetIndex(index)
			content.Choices[0].Delta.ToolCalls = append(content.Choices[0].Delta.ToolCalls, tool)
		}
		stop := helper.GenerateStopResponse(response.Id, created, response.Model, finish)
		stop.Usage = &response.Usage
		first, err := json.Marshal(content)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody), nil
		}
		last, err := json.Marshal(stop)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody), nil
		}
		stream := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + string(first) + "\n\ndata: " + string(last) + "\n\ndata: [DONE]\n\n"))}
		var usage *dto.Usage
		var failure *types.GatewayError
		if info.RelayFormat == types.RelayFormatOpenAIResponses {
			usage, failure = openai.OaiChatToResponsesStreamHandler(c, info, stream)
		} else {
			usage, failure = openai.OaiStreamHandler(c, info, stream)
		}
		return failure, usage
	}
	httpResponse := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(encoded))}
	usageResult, failure := openai.NativeTextResponse(c, info, httpResponse, func([]byte) (*dto.OpenAITextResponse, *types.GatewayError) { return &response, nil })
	return failure, usageResult
}
