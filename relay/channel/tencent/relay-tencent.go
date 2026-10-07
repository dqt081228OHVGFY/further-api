package tencent

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"

	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

// https://cloud.tencent.com/document/product/1729/97732

func requestOpenAI2Tencent(a *Adaptor, request dto.GeneralOpenAIRequest) *TencentChatRequest {
	messages := make([]*TencentMessage, 0, len(request.Messages))
	for i := 0; i < len(request.Messages); i++ {
		message := request.Messages[i]
		messages = append(messages, &TencentMessage{
			Content: message.StringContent(),
			Role:    message.Role,
		})
	}
	var req = TencentChatRequest{
		Stream:   request.Stream,
		Messages: messages,
		Model:    &request.Model,
	}
	if request.TopP != nil {
		req.TopP = request.TopP
	}
	req.Temperature = request.Temperature
	return &req
}

func responseTencent2OpenAI(response *TencentChatResponse) *dto.OpenAITextResponse {
	fullTextResponse := dto.OpenAITextResponse{
		Id:      response.Id,
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
		Usage: dto.Usage{
			PromptTokens:     response.Usage.PromptTokens,
			CompletionTokens: response.Usage.CompletionTokens,
			TotalTokens:      response.Usage.TotalTokens,
		},
	}
	if len(response.Choices) > 0 {
		choice := dto.OpenAITextResponseChoice{
			Index: 0,
			Message: dto.Message{
				Role:    "assistant",
				Content: response.Choices[0].Messages.Content,
			},
			FinishReason: response.Choices[0].FinishReason,
		}
		fullTextResponse.Choices = append(fullTextResponse.Choices, choice)
	}
	return &fullTextResponse
}

func streamResponseTencent2OpenAI(TencentResponse *TencentChatResponse) *dto.ChatCompletionsStreamResponse {
	response := dto.ChatCompletionsStreamResponse{
		Object:  "chat.completion.chunk",
		Created: common.GetTimestamp(),
		Model:   "tencent-hunyuan",
	}
	if len(TencentResponse.Choices) > 0 {
		var choice dto.ChatCompletionsStreamResponseChoice
		choice.Delta.SetContentString(TencentResponse.Choices[0].Delta.Content)
		if TencentResponse.Choices[0].FinishReason == "stop" {
			choice.FinishReason = &constant.FinishReasonStop
		}
		response.Choices = append(response.Choices, choice)
	}
	return &response
}

func tencentStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	return openai.NativeTextStream(c, info, response, func(data []byte) (*dto.ChatCompletionsStreamResponse, *types.GatewayError) {
		var wire TencentChatResponse
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		if wire.Error.Message != "" {
			return nil, types.NewError(errors.New(wire.Error.Message), types.ErrorCodeBadResponseBody)
		}
		frame := streamResponseTencent2OpenAI(&wire)
		frame.Id = wire.Id
		frame.Model = info.UpstreamModelName
		if wire.Usage.TotalTokens > 0 {
			frame.Usage = &dto.Usage{PromptTokens: wire.Usage.PromptTokens, CompletionTokens: wire.Usage.CompletionTokens, TotalTokens: wire.Usage.TotalTokens}
		}
		return frame, nil
	})
}

func tencentHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	return openai.NativeTextResponse(c, info, response, func(data []byte) (*dto.OpenAITextResponse, *types.GatewayError) {
		var wire *TencentChatResponseSB
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		if wire == nil {
			return nil, types.NewError(errors.New("Tencent response must be an object"), types.ErrorCodeBadResponseBody)
		}
		if wire.Response.Error.Message != "" {
			return nil, types.NewError(errors.New(wire.Response.Error.Message), types.ErrorCodeBadResponseBody)
		}
		output := responseTencent2OpenAI(&wire.Response)
		output.Model = info.UpstreamModelName
		return output, nil
	})
}

func parseTencentConfig(config string) (appId int64, secretId string, secretKey string, err error) {
	parts := strings.Split(config, "|")
	if len(parts) != 3 {
		err = errors.New("invalid tencent config")
		return
	}
	appId, err = strconv.ParseInt(parts[0], 10, 64)
	secretId = parts[1]
	secretKey = parts[2]
	return
}

func sha256hex(s string) string {
	b := sha256.Sum256([]byte(s))
	return hex.EncodeToString(b[:])
}

func hmacSha256(s, key string) string {
	hashed := hmac.New(sha256.New, []byte(key))
	hashed.Write([]byte(s))
	return string(hashed.Sum(nil))
}

func getTencentSign(req TencentChatRequest, adaptor *Adaptor, secId, secKey string) string {
	// build canonical request string
	host := "hunyuan.tencentcloudapi.com"
	httpRequestMethod := "POST"
	canonicalURI := "/"
	canonicalQueryString := ""
	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\nx-tc-action:%s\n",
		"application/json", host, strings.ToLower(adaptor.Action))
	signedHeaders := "content-type;host;x-tc-action"
	payload, _ := json.Marshal(req)
	hashedRequestPayload := sha256hex(string(payload))
	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		httpRequestMethod,
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		hashedRequestPayload)
	// build string to sign
	algorithm := "TC3-HMAC-SHA256"
	requestTimestamp := strconv.FormatInt(adaptor.Timestamp, 10)
	timestamp, _ := strconv.ParseInt(requestTimestamp, 10, 64)
	t := time.Unix(timestamp, 0).UTC()
	// must be the format 2006-01-02, ref to package time for more info
	date := t.Format("2006-01-02")
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, "hunyuan")
	hashedCanonicalRequest := sha256hex(canonicalRequest)
	string2sign := fmt.Sprintf("%s\n%s\n%s\n%s",
		algorithm,
		requestTimestamp,
		credentialScope,
		hashedCanonicalRequest)

	// sign string
	secretDate := hmacSha256(date, "TC3"+secKey)
	secretService := hmacSha256("hunyuan", secretDate)
	secretKey := hmacSha256("tc3_request", secretService)
	signature := hex.EncodeToString([]byte(hmacSha256(string2sign, secretKey)))

	// build authorization
	authorization := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm,
		secId,
		credentialScope,
		signedHeaders,
		signature)
	return authorization
}
