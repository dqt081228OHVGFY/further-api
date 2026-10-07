package zhipu

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
	jwt "github.com/golang-jwt/jwt/v5"
)

var expSeconds int64 = 24 * 3600

func getZhipuToken(key string) (string, error) {
	id, secret, valid := strings.Cut(key, ".")
	if !valid || id == "" || secret == "" || strings.Contains(secret, ".") {
		return "", errors.New("Zhipu credentials require id.secret")
	}
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"api_key": id, "exp": now.Add(time.Duration(expSeconds) * time.Second).UnixMilli(), "timestamp": now.UnixMilli()})
	token.Header["sign_type"] = "SIGN"
	return token.SignedString([]byte(secret))
}
func requestOpenAI2Zhipu(request dto.GeneralOpenAIRequest) *ZhipuRequest {
	output := &ZhipuRequest{Prompt: make([]ZhipuMessage, 0, len(request.Messages)), Temperature: request.Temperature}
	if request.TopP != nil {
		output.TopP = *request.TopP
	}
	for _, message := range request.Messages {
		role := message.Role
		if role == "developer" {
			role = "system"
		}
		output.Prompt = append(output.Prompt, ZhipuMessage{Role: role, Content: message.StringContent()})
		if role == "system" {
			output.Prompt = append(output.Prompt, ZhipuMessage{Role: "user", Content: "Okay"})
		}
	}
	return output
}
func responseZhipu2OpenAI(wire *ZhipuResponse) *dto.OpenAITextResponse {
	output := &dto.OpenAITextResponse{Id: wire.Data.TaskId, Object: "chat.completion", Created: common.GetTimestamp(), Usage: wire.Data.Usage, Choices: make([]dto.OpenAITextResponseChoice, 0, len(wire.Data.Choices))}
	for index, choice := range wire.Data.Choices {
		output.Choices = append(output.Choices, dto.OpenAITextResponseChoice{Index: index, Message: dto.Message{Role: choice.Role, Content: strings.Trim(choice.Content, "\"")}, FinishReason: "stop"})
	}
	return output
}
func streamResponseZhipu2OpenAI(text string) *dto.ChatCompletionsStreamResponse {
	frame := &dto.ChatCompletionsStreamResponse{Object: "chat.completion.chunk", Created: common.GetTimestamp(), Choices: []dto.ChatCompletionsStreamResponseChoice{{Index: 0}}}
	frame.Choices[0].Delta.SetContentString(text)
	return frame
}
func streamMetaResponseZhipu2OpenAI(wire *ZhipuStreamMetaResponse) (*dto.ChatCompletionsStreamResponse, *dto.Usage) {
	reason := "stop"
	usage := wire.Usage
	frame := &dto.ChatCompletionsStreamResponse{Id: wire.RequestId, Object: "chat.completion.chunk", Created: common.GetTimestamp(), Choices: []dto.ChatCompletionsStreamResponseChoice{{Index: 0, FinishReason: &reason}}, Usage: &usage}
	return frame, &usage
}

type zhipuEventBody struct {
	source  io.ReadCloser
	scanner *bufio.Scanner
	pending []byte
}

func (body *zhipuEventBody) Read(target []byte) (int, error) {
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
		line := strings.TrimSuffix(body.scanner.Text(), "\r")
		var envelope any
		if value, found := strings.CutPrefix(line, "meta:"); found {
			var meta ZhipuStreamMetaResponse
			if err := json.Unmarshal([]byte(value), &meta); err != nil {
				return 0, err
			}
			envelope = map[string]any{"meta": meta}
		} else if value, found := strings.CutPrefix(line, "data:"); found {
			value = strings.TrimPrefix(value, " ")
			if value == "[DONE]" {
				body.pending = []byte("data: [DONE]\n\n")
				break
			}
			envelope = map[string]string{"text": value}
		} else {
			continue
		}
		encoded, err := json.Marshal(envelope)
		if err != nil {
			return 0, err
		}
		body.pending = append([]byte("data: "), encoded...)
		body.pending = append(body.pending, '\n', '\n')
	}
	n := copy(target, body.pending)
	body.pending = body.pending[n:]
	return n, nil
}
func (body *zhipuEventBody) Close() error { return body.source.Close() }
func zhipuStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if response == nil || response.Body == nil {
		return nil, types.NewError(errors.New("Zhipu stream body is missing"), types.ErrorCodeBadResponseBody)
	}
	adapted := *response
	adapted.Body = &zhipuEventBody{source: response.Body, scanner: helper.NewStreamScanner(response.Body)}
	return openai.NativeTextStream(c, info, &adapted, func(data []byte) (*dto.ChatCompletionsStreamResponse, *types.GatewayError) {
		var wire struct {
			Text string                   `json:"text"`
			Meta *ZhipuStreamMetaResponse `json:"meta"`
		}
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		var frame *dto.ChatCompletionsStreamResponse
		if wire.Meta != nil {
			frame, _ = streamMetaResponseZhipu2OpenAI(wire.Meta)
		} else {
			frame = streamResponseZhipu2OpenAI(wire.Text)
		}
		frame.Model = info.UpstreamModelName
		if frame.Id == "" {
			frame.Id = helper.GetResponseID(c)
		}
		return frame, nil
	})
}
func zhipuHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	return openai.NativeTextResponse(c, info, response, func(data []byte) (*dto.OpenAITextResponse, *types.GatewayError) {
		var wire *ZhipuResponse
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		if wire == nil {
			return nil, types.NewError(errors.New("Zhipu response must be an object"), types.ErrorCodeBadResponseBody)
		}
		if !wire.Success {
			return nil, types.NewError(errors.New(wire.Msg), types.ErrorCodeBadResponseBody)
		}
		output := responseZhipu2OpenAI(wire)
		output.Model = info.UpstreamModelName
		return output, nil
	})
}
