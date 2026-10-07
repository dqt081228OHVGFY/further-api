package openai

import (
	"errors"
	"net/http"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/common_handler"
	"github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

func ProtocolResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo, audioFormat string) (any, *types.GatewayError) {
	if info == nil || c == nil || c.Request == nil {
		return nil, types.NewOpenAIError(errors.New("OpenAI response context is missing"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	if info.RelayMode == constant.RelayModeRealtime {
		err, usage := OpenaiRealtimeHandler(c, info)
		return usage, err
	}
	if response == nil || response.Body == nil {
		return nil, types.NewOpenAIError(errors.New("OpenAI provider returned an empty response"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	switch info.RelayMode {
	case constant.RelayModeAudioSpeech:
		return OpenaiTTSHandler(c, response, info)
	case constant.RelayModeAudioTranslation, constant.RelayModeAudioTranscription:
		err, usage := OpenaiSTTHandler(c, response, info, audioFormat)
		return usage, err
	case constant.RelayModeImagesGenerations, constant.RelayModeImagesEdits:
		if info.IsStream {
			return OpenaiImageStreamHandler(c, info, response)
		}
		return OpenaiImageHandler(c, info, response)
	case constant.RelayModeRerank:
		return common_handler.RerankHandler(c, info, response)
	case constant.RelayModeResponses:
		if info.IsStream {
			return OaiResponsesStreamHandler(c, info, response)
		}
		return OaiResponsesHandler(c, info, response)
	case constant.RelayModeResponsesCompact:
		return OaiResponsesCompactionHandler(c, response)
	default:
		if info.IsStream {
			return OaiStreamHandler(c, info, response)
		}
		return OpenaiHandler(c, info, response)
	}
}
