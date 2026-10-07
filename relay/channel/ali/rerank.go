package ali

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

func ConvertRerankRequest(request dto.RerankRequest) *AliRerankRequest {
	documents := true
	if request.ReturnDocuments != nil {
		documents = *request.ReturnDocuments
	}
	return &AliRerankRequest{Model: request.Model, Input: AliRerankInput{Query: request.Query, Documents: request.Documents}, Parameters: AliRerankParameters{TopN: request.TopN, ReturnDocuments: &documents}}
}
func RerankHandler(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (*types.GatewayError, *dto.Usage) {
	if c == nil || info == nil || response == nil || response.Body == nil {
		return types.NewError(errors.New("Bailian rerank context is missing"), types.ErrorCodeBadResponseBody), nil
	}
	defer service.CloseResponseBodyGracefully(response)
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return types.NewError(err, types.ErrorCodeReadResponseBodyFailed), nil
	}
	var wire *AliRerankResponse
	if err = json.Unmarshal(data, &wire); err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	if wire == nil {
		return types.NewError(errors.New("Bailian rerank response must be an object"), types.ErrorCodeBadResponseBody), nil
	}
	if wire.Code != "" {
		return types.WithOpenAIError(types.OpenAIError{Message: wire.Message, Type: wire.Code, Code: wire.Code, Param: wire.RequestId}, http.StatusBadGateway), nil
	}
	usage := dto.Usage{PromptTokens: wire.Usage.TotalTokens, TotalTokens: wire.Usage.TotalTokens}
	body, err := json.Marshal(dto.RerankResponse{Results: wire.Output.Results, Usage: usage})
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	service.IOCopyBytesGracefully(c, response, body)
	return nil, &usage
}
