package openai

import (
	"encoding/json"
	"errors"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

func OaiResponsesCompactionHandler(c *gin.Context, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if c == nil || response == nil || response.Body == nil {
		return nil, chatResponseError(errors.New("Responses compaction context or body is missing"))
	}
	defer service.CloseResponseBodyGracefully(response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusBadGateway)
	}
	var wire *dto.OpenAIResponsesCompactionResponse
	if err = json.Unmarshal(body, &wire); err != nil {
		return nil, chatResponseError(err)
	}
	if wire == nil {
		return nil, chatResponseError(errors.New("Responses compaction response must be an object"))
	}
	if failure := wire.GetOpenAIError(); failure != nil && failure.Type != "" {
		status := response.StatusCode
		if status < 400 {
			status = http.StatusBadGateway
		}
		return nil, types.WithOpenAIError(*failure, status)
	}
	usage := &dto.Usage{}
	service.ApplyResponsesUsage(usage, wire.Usage)
	service.IOCopyBytesGracefully(c, response, body)
	return usage, nil
}
