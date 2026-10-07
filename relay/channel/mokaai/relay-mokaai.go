package mokaai

import (
	"encoding/json"
	"errors"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

func embeddingRequestOpenAI2Moka(request dto.GeneralOpenAIRequest) *dto.EmbeddingRequest {
	inputs := make([]string, 0)
	switch value := request.Input.(type) {
	case string:
		inputs = append(inputs, value)
	case []string:
		inputs = append(inputs, value...)
	case []any:
		for _, item := range value {
			if text, ok := item.(string); ok {
				inputs = append(inputs, text)
			}
		}
	}
	return &dto.EmbeddingRequest{Input: inputs, Model: request.Model}
}
func embeddingResponseMoka2OpenAI(wire *dto.EmbeddingResponse) *dto.OpenAIEmbeddingResponse {
	output := &dto.OpenAIEmbeddingResponse{Object: "list", Usage: wire.Usage, Data: make([]dto.OpenAIEmbeddingResponseItem, 0, len(wire.Data))}
	for _, item := range wire.Data {
		output.Data = append(output.Data, dto.OpenAIEmbeddingResponseItem{Object: "embedding", Index: item.Index, Embedding: item.Embedding})
	}
	return output
}
func mokaEmbeddingHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if c == nil || info == nil || !info.HasChannelMeta() || response == nil || response.Body == nil {
		return nil, types.NewError(errors.New("Moka embedding context is missing"), types.ErrorCodeBadResponseBody)
	}
	defer service.CloseResponseBodyGracefully(response)
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	var wire *dto.EmbeddingResponse
	if err = json.Unmarshal(data, &wire); err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if wire == nil {
		return nil, types.NewError(errors.New("Moka embedding response must be an object"), types.ErrorCodeBadResponseBody)
	}
	output := embeddingResponseMoka2OpenAI(wire)
	output.Model = info.UpstreamModelName
	body, err := json.Marshal(output)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	service.IOCopyBytesGracefully(c, response, body)
	return &output.Usage, nil
}
