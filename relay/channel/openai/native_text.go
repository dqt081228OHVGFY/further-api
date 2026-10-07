package openai

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

type nativeJSONLines struct {
	source  io.ReadCloser
	scanner *bufio.Scanner
	pending []byte
}

func JSONLinesAsSSE(source io.ReadCloser) io.ReadCloser {
	return &nativeJSONLines{source: source, scanner: helper.NewStreamScanner(source)}
}
func (body *nativeJSONLines) Read(target []byte) (int, error) {
	if len(target) == 0 {
		return 0, nil
	}
	for len(body.pending) == 0 {
		if !body.scanner.Scan() {
			if err := body.scanner.Err(); err != nil {
				return 0, err
			}
			return 0, io.EOF
		}
		line := strings.TrimSpace(body.scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			body.pending = []byte(line + "\n\n")
		} else {
			body.pending = []byte("data: " + line + "\n\n")
		}
	}
	n := copy(target, body.pending)
	body.pending = body.pending[n:]
	return n, nil
}
func (body *nativeJSONLines) Close() error { return body.source.Close() }

type NativeTextDecoder func([]byte) (*dto.OpenAITextResponse, *types.GatewayError)
type NativeStreamDecoder func([]byte) (*dto.ChatCompletionsStreamResponse, *types.GatewayError)

// NativeTextResponse and NativeTextStream are the execution boundaries for
// native providers whose official DTOs project into the chat protocol. Decode
// callbacks contain provider wire facts; lifecycle, usage and writes live here.
func NativeTextResponse(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response, decode NativeTextDecoder) (*dto.Usage, *types.GatewayError) {
	if err := responseContext(c, info, response); err != nil {
		return nil, chatResponseError(err)
	}
	if decode == nil {
		return nil, chatResponseError(errors.New("native response decoder is missing"))
	}
	defer service.CloseResponseBodyGracefully(response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, chatResponseError(err)
	}
	wire, failure := decode(body)
	if failure != nil {
		return nil, failure
	}
	if wire == nil {
		return nil, chatResponseError(errors.New("native provider returned no response"))
	}
	if wire.Model == "" {
		wire.Model = info.UpstreamModelName
	}
	info.ObserveResponseModel(wire.Model)
	if wire.Usage.PromptTokens == 0 {
		wire.Usage.PromptTokens = info.GetEstimatePromptTokens()
	}
	if wire.Usage.CompletionTokens == 0 {
		for _, choice := range wire.Choices {
			wire.Usage.CompletionTokens += service.CountTextToken(choice.Message.StringContent()+choice.Message.GetReasoningContent(), info.UpstreamModelName)
		}
	}
	if wire.Usage.TotalTokens == 0 {
		wire.Usage.TotalTokens = wire.Usage.PromptTokens + wire.Usage.CompletionTokens
	}
	var output any = wire
	if info.RelayFormat == types.RelayFormatClaude || info.RelayFormat == types.RelayFormatGemini || info.RelayFormat == types.RelayFormatOpenAIResponses {
		converted, err := service.ConvertResponse(c, info, info.RelayFormat, wire)
		if err != nil {
			return &wire.Usage, chatResponseError(err)
		}
		output = converted.Value
	}
	body, err = json.Marshal(output)
	if err != nil {
		return &wire.Usage, chatResponseError(err)
	}
	service.IOCopyBytesGracefully(c, response, body)
	return &wire.Usage, nil
}

func NativeTextStream(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response, decode NativeStreamDecoder) (*dto.Usage, *types.GatewayError) {
	if err := responseContext(c, info, response); err != nil {
		return nil, chatResponseError(err)
	}
	if decode == nil {
		return nil, chatResponseError(errors.New("native stream decoder is missing"))
	}
	defer service.CloseResponseBodyGracefully(response)
	usage := &dto.Usage{}
	id, created, model := "", int64(0), info.UpstreamModelName
	var text strings.Builder
	toolCount := 0
	upstreamUsage := false
	var failure *types.GatewayError
	helper.StreamScannerHandler(c, response, info, func(data string, result *helper.StreamResult) {
		frame, err := decode([]byte(data))
		if err != nil {
			failure = err
			result.Stop(err)
			return
		}
		if frame == nil {
			return
		}
		if frame.Id != "" {
			id = frame.Id
		}
		if frame.Created != 0 {
			created = frame.Created
		}
		if frame.Model != "" {
			model = frame.Model
			info.ObserveResponseModel(model)
		} else {
			frame.Model = model
		}
		if frame.Usage != nil {
			usage = dto.MergeUsageNonZero(usage, frame.Usage)
			upstreamUsage = upstreamUsage || service.ValidUsage(frame.Usage)
		}
		for _, choice := range frame.Choices {
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				info.StreamStatus.MarkCompleted()
			}
		}
		if err := ProcessStreamResponse(*frame, &text, &toolCount); err != nil {
			failure = chatResponseError(err)
			result.Stop(failure)
			return
		}
		payload, marshalErr := json.Marshal(frame)
		if marshalErr != nil {
			failure = chatResponseError(marshalErr)
			result.Stop(failure)
			return
		}
		var terminalWithPayload bool
		for _, choice := range frame.Choices {
			if choice.FinishReason != nil && (choice.Delta.GetContentString() != "" || choice.Delta.GetReasoningContent() != "" || len(choice.Delta.ToolCalls) > 0) {
				terminalWithPayload = true
			}
		}
		if terminalWithPayload {
			content := frame.Copy()
			for index := range content.Choices {
				content.Choices[index].FinishReason = nil
			}
			content.Usage = nil
			contentData, err := json.Marshal(content)
			if err != nil {
				failure = chatResponseError(err)
				result.Stop(failure)
				return
			}
			if err = HandleStreamFormat(c, info, string(contentData), info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent); err != nil {
				failure = chatResponseError(err)
				result.Stop(failure)
				return
			}
			for index := range frame.Choices {
				frame.Choices[index].Delta = dto.ChatCompletionsStreamResponseChoiceDelta{}
			}
			payload, marshalErr = json.Marshal(frame)
			if marshalErr != nil {
				failure = chatResponseError(marshalErr)
				result.Stop(failure)
				return
			}
		}
		if err := HandleStreamFormat(c, info, string(payload), info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent); err != nil {
			failure = chatResponseError(err)
			result.Stop(failure)
		}
	})
	if !upstreamUsage {
		estimated := service.ResponseText2Usage(c, text.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		if usage.PromptTokens == 0 {
			usage.PromptTokens = estimated.PromptTokens
		}
		if usage.CompletionTokens == 0 {
			usage.CompletionTokens = estimated.CompletionTokens + toolCount*7
		}
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if failure != nil {
		return usage, failure
	}
	if c.Request != nil && c.Request.Context().Err() != nil {
		return usage, nil
	}
	if info.StreamStatus != nil && !info.StreamStatus.IsNormalEnd() {
		return usage, nil
	}
	final := helper.GenerateFinalUsageResponse(id, created, model, *usage)
	body, err := json.Marshal(final)
	if err != nil {
		return usage, chatResponseError(err)
	}
	if info.RelayFormat == types.RelayFormatClaude {
		info.EnsureClaudeConvertInfo().Usage = usage
	}
	if err = FinalizeChatStream(c, info, string(body), id, created, model, "", usage, false); err != nil {
		return usage, chatResponseError(err)
	}
	return usage, nil
}
