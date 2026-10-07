package gemini

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

func GeminiTextGenerationHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	body, wire, failure := readGeminiChat(c, info, response)
	if failure != nil {
		return nil, failure
	}
	usage := buildUsageFromGeminiResponse(c, info, wire)
	service.IOCopyBytesGracefully(c, response, body)
	return &usage, nil
}
func NativeGeminiEmbeddingHandler(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.GatewayError) {
	if err := contentResponseContext(c, info, response); err != nil {
		return nil, contentResponseError(err)
	}
	defer service.CloseResponseBodyGracefully(response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, contentResponseError(err)
	}
	if failure := geminiWireError(body); failure != nil {
		return nil, failure
	}
	if info.IsGeminiBatchEmbedding {
		var wire *dto.GeminiBatchEmbeddingResponse
		if err = json.Unmarshal(body, &wire); err != nil {
			return nil, contentResponseError(err)
		}
		if wire == nil || len(wire.Embeddings) == 0 {
			return nil, contentResponseError(errors.New("Gemini returned no batch embeddings"))
		}
	} else {
		var wire *dto.GeminiEmbeddingResponse
		if err = json.Unmarshal(body, &wire); err != nil {
			return nil, contentResponseError(err)
		}
		if wire == nil || len(wire.Embedding.Values) == 0 {
			return nil, contentResponseError(errors.New("Gemini returned no embedding values"))
		}
	}
	usage := service.ResponseText2Usage(c, "", info.UpstreamModelName, info.GetEstimatePromptTokens())
	service.IOCopyBytesGracefully(c, response, body)
	return usage, nil
}
func GeminiTextGenerationStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if err := contentResponseContext(c, info, response); err != nil {
		return nil, contentResponseError(err)
	}
	helper.SetEventStreamHeaders(c)
	return geminiStreamHandler(c, info, response, func(data string, _ *dto.GeminiChatResponse) bool {
		if err := helper.StringData(c, data); err != nil {
			return false
		}
		info.IncrSendResponseCount()
		return true
	})
}
