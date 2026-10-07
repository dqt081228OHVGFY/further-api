package relay

import (
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	openaichannel "github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

func applySystemPromptIfNeeded(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) {
	if info == nil || request == nil || info.ChannelSetting.SystemPrompt == "" {
		return
	}
	prompt, role := info.ChannelSetting.SystemPrompt, request.GetSystemRoleName()
	first := -1
	for index, message := range request.Messages {
		if message.Role == role && len(message.Tools) == 0 {
			first = index
			break
		}
	}
	if first < 0 {
		request.Messages = append([]dto.Message{{Role: role, Content: prompt}}, request.Messages...)
		return
	}
	if !info.ChannelSetting.SystemPromptOverride {
		return
	}
	common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
	message := &request.Messages[first]
	if message.IsStringContent() {
		message.SetStringContent(prompt + "\n" + message.StringContent())
		return
	}
	message.Content = append([]dto.MediaContent{{Type: dto.ContentTypeText, Text: prompt}}, message.ParseContent()...)
}

func textRequestViaResponses(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, source any) (*dto.Usage, *types.GatewayError) {
	attempt := &protocolAttempt{ctx: c, info: info, adaptor: adaptor}
	overridesApplied := false
	if request, ok := source.(*dto.GeneralOpenAIRequest); ok {
		raw, err := common.Marshal(request)
		if err != nil {
			return nil, newConvertRequestFailedError(c, info, err)
		}
		raw, err = relaycommon.RemoveDisabledFields(raw, info.ChannelOtherSettings, false)
		if err != nil {
			return nil, newConvertRequestFailedError(c, info, err)
		}
		if len(info.ParamOverride) > 0 {
			raw, err = relaycommon.ApplyParamOverrideWithRelayInfo(raw, info)
			if err != nil {
				return nil, gatewayErrorFromParamOverride(err)
			}
			overridesApplied = true
		}
		var rewritten dto.GeneralOpenAIRequest
		if err := common.Unmarshal(raw, &rewritten); err != nil {
			return nil, gatewayErrorFromParamOverride(err)
		}
		source = &rewritten
	}
	if err := protocolCancellation(c); err != nil {
		return nil, err
	}
	converted, err := service.ConvertRequest(c, info, types.RelayFormatOpenAIResponses, source)
	if err != nil {
		return nil, newConvertRequestFailedError(c, info, err)
	}
	request, ok := converted.Value.(*dto.OpenAIResponsesRequest)
	if !ok || request == nil {
		return nil, newConvertRequestFailedError(c, info, errors.New("Responses 转换未生成有效请求。"))
	}
	return attempt.relayResponsesBridge(request, overridesApplied)
}

func relayResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, request *dto.OpenAIResponsesRequest, overridesApplied bool) (*dto.Usage, *types.GatewayError) {
	return (&protocolAttempt{ctx: c, info: info, adaptor: adaptor}).relayResponsesBridge(request, overridesApplied)
}

func (attempt *protocolAttempt) relayResponsesBridge(request *dto.OpenAIResponsesRequest, overridesApplied bool) (*dto.Usage, *types.GatewayError) {
	info, c := attempt.info, attempt.ctx
	mode, path, clientStream := info.RelayMode, info.RequestURLPath, info.IsStream
	defer func() { info.RelayMode, info.RequestURLPath = mode, path }()
	info.RelayMode, info.RequestURLPath = relayconstant.RelayModeResponses, "/v1/responses"
	payload, err := attempt.adaptor.ConvertOpenAIResponsesRequest(c, info, *request)
	if err != nil {
		return nil, newConvertRequestFailedError(c, info, err)
	}
	relaycommon.AppendRequestConversionFromRequest(info, payload)
	raw, err := common.Marshal(payload)
	if err != nil {
		return nil, newConvertRequestFailedError(c, info, err)
	}
	body, apiErr := attempt.jsonBytes(raw, true, !overridesApplied, nil)
	if apiErr != nil {
		return nil, apiErr
	}
	defer body.Close()
	return attempt.performWithDecoder(body.reader, func(response *http.Response) (any, *types.GatewayError) {
		if response == nil {
			return nil, types.NewErrorWithStatusCode(errors.New("Responses 桥接没有返回 HTTP 响应。"), types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
		}
		stream := isResponsesEventStreamContentType(response.Header.Get("Content-Type"))
		switch {
		case stream && clientStream:
			return openaichannel.OaiResponsesToChatStreamHandler(c, info, response)
		case stream:
			info.IsStream = false
			return openaichannel.OaiResponsesToChatBufferedStreamHandler(c, info, response)
		default:
			return openaichannel.OaiResponsesToChatHandler(c, info, response)
		}
	})
}

func isResponsesEventStreamContentType(contentType string) bool {
	kind, _, err := mime.ParseMediaType(contentType)
	return err == nil && strings.EqualFold(kind, "text/event-stream")
}
