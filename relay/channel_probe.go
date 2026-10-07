package relay

import (
	"errors"
	"reflect"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// Channel probes use the same transport, cancellation and response boundary as
// live requests. Their caller owns diagnostic logging and never moves funds.
func ExecuteChannelProbe(c *gin.Context, info *relaycommon.RelayInfo, source dto.Request) (*dto.Usage, *types.GatewayError) {
	if source == nil || reflect.ValueOf(source).Kind() == reflect.Pointer && reflect.ValueOf(source).IsNil() {
		return nil, types.NewError(errors.New("渠道探测请求为空。"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	attempt, failure := beginProtocolAttempt(c, info)
	if failure != nil {
		return nil, failure
	}
	if failure := attempt.bindAdaptor(); failure != nil {
		return nil, failure
	}
	request, err := cloneProtocolDTO(source)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	attempt.passThrough, attempt.acceptCreated = false, true
	body, failure := attempt.convertedBody(false, func() (any, error) {
		switch request := request.(type) {
		case *dto.GeneralOpenAIRequest:
			return attempt.adaptor.ConvertOpenAIRequest(c, info, request)
		case *dto.ClaudeRequest:
			return attempt.adaptor.ConvertClaudeRequest(c, info, request)
		case *dto.GeminiChatRequest:
			return attempt.adaptor.ConvertGeminiRequest(c, info, request)
		case *dto.EmbeddingRequest:
			return attempt.adaptor.ConvertEmbeddingRequest(c, info, *request)
		case *dto.ImageRequest:
			return attempt.adaptor.ConvertImageRequest(c, info, *request)
		case *dto.RerankRequest:
			return attempt.adaptor.ConvertRerankRequest(c, info.RelayMode, *request)
		case *dto.OpenAIResponsesRequest:
			return attempt.adaptor.ConvertOpenAIResponsesRequest(c, info, *request)
		case *dto.OpenAIResponsesCompactionRequest:
			return attempt.adaptor.ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{Model: request.Model, Input: request.Input,
				Instructions: request.Instructions, PreviousResponseID: request.PreviousResponseID})
		default:
			return nil, errors.New("渠道探测协议暂不支持此请求类型。")
		}
	}, nil)
	if failure != nil {
		return nil, failure
	}
	defer body.Close()
	return attempt.perform(body.reader)
}
