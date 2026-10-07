package relay

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

type attemptNoopCloser struct{}

func (attemptNoopCloser) Close() error { return nil }

// HTTP and WebSocket share a prepared adaptor and body ownership contract.
// Keep the exact converted instance: providers retain conversion route state.
func PrepareResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, source *dto.OpenAIResponsesRequest) (channel.Adaptor, common.ReplayableBody, io.Closer, *types.GatewayError) {
	attempt, err := beginProtocolAttempt(c, info)
	if err != nil {
		return nil, nil, nil, err
	}
	if source == nil {
		return nil, nil, nil, imageQuantityError(errors.New("Responses 请求内容缺失。"))
	}
	if info.RelayMode == relayconstant.RelayModeResponsesCompact && !common.SupportsResponsesCompact(info.ChannelType, info.ApiType) {
		return nil, nil, nil, imageQuantityError(fmt.Errorf("渠道协议 %d 不支持 /v1/responses/compact。", info.ApiType))
	}
	request, copyErr := cloneProtocolDTO(source)
	if copyErr != nil {
		return nil, nil, nil, types.NewError(copyErr, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	if copyErr = helper.ModelMappedHelper(c, info, request); copyErr != nil {
		return nil, nil, nil, types.NewError(copyErr, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	if copyErr = helper.ApplyReasoningModelSuffix(c, info, request); copyErr != nil {
		return nil, nil, nil, newConvertRequestFailedError(c, info, copyErr)
	}
	if err = attempt.bindAdaptor(); err != nil {
		return nil, nil, nil, err
	}
	body, err := attempt.convertedBody(true, func() (any, error) { return attempt.adaptor.ConvertOpenAIResponsesRequest(c, info, *request) }, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	reader, ok := body.reader.(common.ReplayableBody)
	if !ok {
		body.Close()
		return nil, nil, nil, types.NewErrorWithStatusCode(errors.New("Responses 请求体不支持安全重放。"), types.ErrorCodeConvertRequestFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	owner := body.owner
	if owner == nil {
		owner = attemptNoopCloser{}
	}
	return attempt.adaptor, reader, owner, nil
}
