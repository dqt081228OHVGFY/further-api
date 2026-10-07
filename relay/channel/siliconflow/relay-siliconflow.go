package siliconflow

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

func siliconflowRerankHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if c == nil || info == nil || response == nil || response.Body == nil {
		return nil, types.NewError(errors.New("SiliconFlow rerank context is missing"), types.ErrorCodeBadResponseBody)
	}
	defer service.CloseResponseBodyGracefully(response)
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeReadResponseBodyFailed)
	}
	var wire *SFRerankResponse
	if err = json.Unmarshal(data, &wire); err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if wire == nil {
		return nil, types.NewError(errors.New("SiliconFlow rerank response must be an object"), types.ErrorCodeBadResponseBody)
	}
	usage := dto.Usage{PromptTokens: wire.Meta.Tokens.InputTokens, CompletionTokens: wire.Meta.Tokens.OutputTokens, TotalTokens: wire.Meta.Tokens.InputTokens + wire.Meta.Tokens.OutputTokens}
	body, err := json.Marshal(dto.RerankResponse{Results: wire.Results, Usage: usage})
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	service.IOCopyBytesGracefully(c, response, body)
	return &usage, nil
}
