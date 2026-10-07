package baidu

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"

	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
)

// https://cloud.baidu.com/doc/WENXINWORKSHOP/s/flfmc9do2

func requestOpenAI2Baidu(request dto.GeneralOpenAIRequest) *BaiduChatRequest {
	baiduRequest := BaiduChatRequest{
		Temperature:    request.Temperature,
		TopP:           lo.FromPtrOr(request.TopP, 0),
		PenaltyScore:   lo.FromPtrOr(request.FrequencyPenalty, 0),
		Stream:         lo.FromPtrOr(request.Stream, false),
		DisableSearch:  false,
		EnableCitation: false,
		UserId:         request.User,
	}
	if request.GetMaxTokens() != 0 {
		maxTokens := int(request.GetMaxTokens())
		if request.GetMaxTokens() == 1 {
			maxTokens = 2
		}
		baiduRequest.MaxOutputTokens = &maxTokens
	}
	for _, message := range request.Messages {
		if message.Role == "system" {
			baiduRequest.System = message.StringContent()
		} else {
			baiduRequest.Messages = append(baiduRequest.Messages, BaiduMessage{
				Role:    message.Role,
				Content: message.StringContent(),
			})
		}
	}
	return &baiduRequest
}

func responseBaidu2OpenAI(response *BaiduChatResponse) *dto.OpenAITextResponse {
	choice := dto.OpenAITextResponseChoice{
		Index: 0,
		Message: dto.Message{
			Role:    "assistant",
			Content: response.Result,
		},
		FinishReason: "stop",
	}
	fullTextResponse := dto.OpenAITextResponse{
		Id:      response.Id,
		Object:  "chat.completion",
		Created: response.Created,
		Choices: []dto.OpenAITextResponseChoice{choice},
		Usage:   response.Usage,
	}
	return &fullTextResponse
}

func streamResponseBaidu2OpenAI(baiduResponse *BaiduChatStreamResponse) *dto.ChatCompletionsStreamResponse {
	var choice dto.ChatCompletionsStreamResponseChoice
	choice.Delta.SetContentString(baiduResponse.Result)
	if baiduResponse.IsEnd {
		choice.FinishReason = &constant.FinishReasonStop
	}
	response := dto.ChatCompletionsStreamResponse{
		Id:      baiduResponse.Id,
		Object:  "chat.completion.chunk",
		Created: baiduResponse.Created,
		Model:   "ernie-bot",
		Choices: []dto.ChatCompletionsStreamResponseChoice{choice},
	}
	return &response
}

func embeddingRequestOpenAI2Baidu(request dto.EmbeddingRequest) *BaiduEmbeddingRequest {
	return &BaiduEmbeddingRequest{
		Input: request.ParseInput(),
	}
}

func embeddingResponseBaidu2OpenAI(response *BaiduEmbeddingResponse) *dto.OpenAIEmbeddingResponse {
	openAIEmbeddingResponse := dto.OpenAIEmbeddingResponse{
		Object: "list",
		Data:   make([]dto.OpenAIEmbeddingResponseItem, 0, len(response.Data)),
		Model:  "baidu-embedding",
		Usage:  response.Usage,
	}
	for _, item := range response.Data {
		openAIEmbeddingResponse.Data = append(openAIEmbeddingResponse.Data, dto.OpenAIEmbeddingResponseItem{
			Object:    item.Object,
			Index:     item.Index,
			Embedding: item.Embedding,
		})
	}
	return &openAIEmbeddingResponse
}

func baiduStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*types.GatewayError, *dto.Usage) {
	usage, failure := openai.NativeTextStream(c, info, response, func(data []byte) (*dto.ChatCompletionsStreamResponse, *types.GatewayError) {
		var wire BaiduChatStreamResponse
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		if wire.ErrorCode != 0 {
			return nil, types.NewOpenAIError(errors.New(wire.ErrorMsg), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
		}
		frame := streamResponseBaidu2OpenAI(&wire)
		frame.Model = info.UpstreamModelName
		if wire.Usage.TotalTokens > 0 {
			snapshot := wire.Usage
			if snapshot.CompletionTokens == 0 {
				snapshot.CompletionTokens = max(0, snapshot.TotalTokens-snapshot.PromptTokens)
			}
			frame.Usage = &snapshot
		}
		return frame, nil
	})
	return failure, usage
}

func baiduHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*types.GatewayError, *dto.Usage) {
	usage, failure := openai.NativeTextResponse(c, info, response, func(data []byte) (*dto.OpenAITextResponse, *types.GatewayError) {
		var wire BaiduChatResponse
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		if wire.ErrorCode != 0 {
			return nil, types.NewOpenAIError(errors.New(wire.ErrorMsg), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
		}
		output := responseBaidu2OpenAI(&wire)
		output.Model = info.UpstreamModelName
		if wire.IsTruncated && len(output.Choices) > 0 {
			output.Choices[0].FinishReason = "length"
		}
		return output, nil
	})
	return failure, usage
}

func baiduEmbeddingHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*types.GatewayError, *dto.Usage) {
	if c == nil || info == nil || !info.HasChannelMeta() || response == nil || response.Body == nil {
		return types.NewError(errors.New("Baidu embedding context is missing"), types.ErrorCodeBadResponseBody), nil
	}
	defer service.CloseResponseBodyGracefully(response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return types.NewError(err, types.ErrorCodeReadResponseBodyFailed), nil
	}
	var wire *BaiduEmbeddingResponse
	if err = json.Unmarshal(body, &wire); err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	if wire == nil {
		return types.NewError(errors.New("Baidu embedding response must be an object"), types.ErrorCodeBadResponseBody), nil
	}
	if wire.ErrorCode != 0 {
		return types.NewError(errors.New(wire.ErrorMsg), types.ErrorCodeBadResponseBody), nil
	}
	output := embeddingResponseBaidu2OpenAI(wire)
	output.Model = info.UpstreamModelName
	body, err = json.Marshal(output)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	service.IOCopyBytesGracefully(c, response, body)
	return nil, &output.Usage
}
