package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

func AlphaSearchHelper(c *gin.Context, info *relaycommon.RelayInfo) *types.GatewayError {
	attempt, err := beginProtocolAttempt(c, info)
	if err != nil {
		return err
	}
	switch info.ChannelType {
	case constant.ChannelTypeSub2API, constant.ChannelTypeGateway, constant.ChannelTypeCodex, constant.ChannelTypeAdvancedCustom:
	default:
		return types.NewError(errors.New("此渠道不支持 /v1/alpha/search。"), types.ErrorCodeInvalidRequest)
	}
	request, err := cloneProtocolRequest[*dto.AlphaSearchRequest](attempt, false)
	if err != nil {
		return err
	}
	raw, rawErr := buildAlphaSearchRequestBody(request.RawBody, info.OriginModelName, info.UpstreamModelName)
	if rawErr != nil {
		return newConvertRequestFailedError(c, info, rawErr)
	}
	body, err := attempt.jsonBytes(raw, false, true, nil)
	if err != nil {
		return err
	}
	defer body.Close()
	response, err := attempt.send(body.reader)
	if err != nil {
		return err
	}
	if response == nil {
		return types.NewErrorWithStatusCode(errors.New("搜索上游未返回 HTTP 响应。"), types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	defer service.CloseResponseBodyGracefully(response)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return attempt.mapError(service.RelayErrorHandler(c.Request.Context(), response, false))
	}
	connectionFields := make(map[string]bool)
	for _, value := range response.Header.Values("Connection") {
		for _, name := range strings.Split(value, ",") {
			connectionFields[strings.ToLower(strings.TrimSpace(name))] = true
		}
	}
	for name, values := range response.Header {
		if !service.ShouldCopyUpstreamHeader(c, name, values) || connectionFields[strings.ToLower(name)] {
			continue
		}
		c.Writer.Header().Del(name)
		for _, value := range values {
			c.Writer.Header().Add(name, value)
		}
	}
	c.Writer.WriteHeader(response.StatusCode)
	if _, err := io.Copy(c.Writer, response.Body); err != nil {
		if cancel := protocolCancellation(c); cancel != nil {
			return cancel
		}
		return types.NewError(err, types.ErrorCodeDoRequestFailed, types.ErrOptionWithSkipRetry())
	}
	if info.ResponsesUsageInfo == nil {
		info.ResponsesUsageInfo = &relaycommon.ResponsesUsageInfo{}
	}
	if info.ResponsesUsageInfo.BuiltInTools == nil {
		info.ResponsesUsageInfo.BuiltInTools = make(map[string]*relaycommon.BuildInToolInfo)
	}
	info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview] = &relaycommon.BuildInToolInfo{ToolName: dto.BuildInToolWebSearchPreview, CallCount: 1}
	service.PostTextConsumeQuota(c, info, &dto.Usage{}, nil)
	return nil
}

func buildAlphaSearchRequestBody(raw []byte, original, upstream string) ([]byte, error) {
	if len(raw) == 0 {
		return nil, errors.New("搜索请求内容为空。")
	}
	if upstream == "" || upstream == original {
		return raw, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, fmt.Errorf("搜索请求必须为 JSON 对象。")
	}
	model, err := json.Marshal(upstream)
	if err != nil {
		return nil, err
	}
	fields["model"] = model
	return json.Marshal(fields)
}
