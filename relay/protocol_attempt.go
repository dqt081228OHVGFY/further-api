package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"reflect"
	"sync"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting/model_setting"
	"github.com/gin-gonic/gin"
)

// protocolAttempt owns attempt-scoped state. The request's accounting lease,
// usage evidence, original DTO and conversion diagnostics remain request-scoped.
type protocolAttempt struct {
	ctx           *gin.Context
	info          *relaycommon.RelayInfo
	adaptor       channel.Adaptor
	passThrough   bool
	acceptCreated bool
}

type attemptBody struct {
	reader io.Reader
	owner  io.Closer
}

func (body attemptBody) Close() {
	if body.owner != nil {
		_ = body.owner.Close()
	}
}

func protocolCancellation(c *gin.Context) *types.GatewayError {
	if c == nil || c.Request == nil {
		return types.NewErrorWithStatusCode(errors.New("网关请求上下文不完整。"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if err := c.Request.Context().Err(); err != nil {
		status := http.StatusRequestTimeout
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		return types.NewErrorWithStatusCode(err, types.ErrorCodeDoRequestFailed, status,
			types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog(),
			types.ErrOptionWithHideErrMsg("请求已取消或等待超时。"))
	}
	return nil
}

func beginProtocolAttempt(c *gin.Context, info *relaycommon.RelayInfo) (*protocolAttempt, *types.GatewayError) {
	if err := protocolCancellation(c); err != nil {
		return nil, err
	}
	if info == nil {
		return nil, types.NewErrorWithStatusCode(errors.New("网关请求信息缺失。"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	info.InitChannelMeta(c)
	return &protocolAttempt{ctx: c, info: info, passThrough: model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled}, nil
}

func cloneProtocolRequest[T dto.Request](attempt *protocolAttempt, modifiers bool) (T, *types.GatewayError) {
	var empty T
	request, ok := attempt.info.Request.(T)
	if !ok || reflect.ValueOf(request).IsNil() {
		return empty, types.NewErrorWithStatusCode(fmt.Errorf("请求协议类型无效：%T。", attempt.info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	copy, err := cloneProtocolDTO(request)
	if err != nil {
		return empty, types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	if err := helper.ModelMappedHelper(attempt.ctx, attempt.info, copy); err != nil {
		return empty, types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	if modifiers {
		if err := helper.ApplyReasoningModelSuffix(attempt.ctx, attempt.info, copy); err != nil {
			return empty, newConvertRequestFailedError(attempt.ctx, attempt.info, err)
		}
	}
	return copy, nil
}

func (attempt *protocolAttempt) bindAdaptor() *types.GatewayError {
	if attempt.adaptor != nil {
		return nil
	}
	attempt.adaptor = GetAdaptor(attempt.info.ApiType)
	if attempt.adaptor == nil {
		return types.NewError(fmt.Errorf("渠道协议类型 %d 暂不可用。", attempt.info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	attempt.adaptor.Init(attempt.info)
	return nil
}

func (attempt *protocolAttempt) rawBody() (attemptBody, *types.GatewayError) {
	if err := protocolCancellation(attempt.ctx); err != nil {
		return attemptBody{}, err
	}
	storage, err := common.GetBodyStorage(attempt.ctx)
	if err != nil {
		return attemptBody{}, types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	return attemptBody{reader: common.NewReplayableBodyReader(storage)}, nil
}

func (attempt *protocolAttempt) jsonBytes(data []byte, disabled, overrides bool, validate func([]byte) *types.GatewayError) (attemptBody, *types.GatewayError) {
	if err := protocolCancellation(attempt.ctx); err != nil {
		return attemptBody{}, err
	}
	var err error
	if disabled {
		data, err = relaycommon.RemoveDisabledFields(data, attempt.info.ChannelOtherSettings, attempt.passThrough)
		if err != nil {
			return attemptBody{}, newConvertRequestFailedError(attempt.ctx, attempt.info, err)
		}
	}
	if overrides && len(attempt.info.ParamOverride) > 0 {
		data, err = relaycommon.ApplyParamOverrideWithRelayInfo(data, attempt.info)
		if err != nil {
			return attemptBody{}, gatewayErrorFromParamOverride(err)
		}
	}
	if validate != nil {
		if err := validate(data); err != nil {
			return attemptBody{}, err
		}
	}
	body, owner, err := relaycommon.NewOutboundJSONBody(data)
	if err != nil {
		return attemptBody{}, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	return attemptBody{reader: body, owner: owner}, nil
}

func (attempt *protocolAttempt) jsonBody(payload any, disabled bool, validate func([]byte) *types.GatewayError) (attemptBody, *types.GatewayError) {
	relaycommon.AppendRequestConversionFromRequest(attempt.info, payload)
	data, err := common.Marshal(payload)
	if err != nil {
		return attemptBody{}, types.NewError(err, types.ErrorCodeJsonMarshalFailed, types.ErrOptionWithSkipRetry())
	}
	return attempt.jsonBytes(data, disabled, true, validate)
}

type attemptResponseBody struct {
	io.ReadCloser
	once sync.Once
	err  error
}

func (body *attemptResponseBody) Close() error {
	body.once.Do(func() { body.err = body.ReadCloser.Close() })
	return body.err
}

func (attempt *protocolAttempt) mapError(err *types.GatewayError) *types.GatewayError {
	var secrets []string
	if attempt.info.HasChannelMeta() {
		secrets = service.UpstreamCredentialSecrets(attempt.info.ApiKey)
		secrets = append(secrets, service.UpstreamCredentialSecrets(attempt.info.ChannelSetting.Proxy)...)
	}
	err = service.RedactUpstreamGatewayError(err, secrets...)
	service.ResetStatusCode(err, attempt.ctx.GetString("status_code_mapping"))
	return err
}

func (attempt *protocolAttempt) send(body io.Reader) (*http.Response, *types.GatewayError) {
	if err := protocolCancellation(attempt.ctx); err != nil {
		return nil, err
	}
	if err := attempt.bindAdaptor(); err != nil {
		return nil, err
	}
	result, err := attempt.adaptor.DoRequest(attempt.ctx, attempt.info, body)
	if err != nil {
		var classified *types.GatewayError
		if errors.As(err, &classified) {
			return nil, attempt.mapError(types.NewError(err, types.ErrorCodeDoRequestFailed))
		}
		if cancel := protocolCancellation(attempt.ctx); cancel != nil {
			return nil, cancel
		}
		return nil, attempt.mapError(types.NewErrorWithStatusCode(err, types.ErrorCodeDoRequestFailed, http.StatusBadGateway,
			types.ErrOptionWithHideErrMsg("上游请求未能完成，请稍后重试。")))
	}
	if result == nil {
		if attempt.info.ApiType == constant.APITypeAws {
			return nil, nil
		}
		return nil, types.NewErrorWithStatusCode(errors.New("上游没有返回 HTTP 响应。"), types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	response, ok := result.(*http.Response)
	if !ok || response == nil {
		return nil, types.NewErrorWithStatusCode(fmt.Errorf("上游返回了无效响应类型 %T。", result), types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	if response.Body == nil {
		// Spark executes its WebSocket exchange in DoResponse and returns a
		// dummy HTTP 200 envelope from DoRequest.
		if attempt.info.ApiType != constant.APITypeXunfei {
			return nil, types.NewErrorWithStatusCode(errors.New("上游响应内容缺失。"), types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
		}
	} else {
		response.Body = &attemptResponseBody{ReadCloser: response.Body}
	}
	return response, nil
}

func (attempt *protocolAttempt) perform(body io.Reader) (*dto.Usage, *types.GatewayError) {
	return attempt.performWithDecoder(body, nil)
}

func (attempt *protocolAttempt) performWithDecoder(body io.Reader, decode func(*http.Response) (any, *types.GatewayError)) (*dto.Usage, *types.GatewayError) {
	response, err := attempt.send(body)
	if err != nil {
		return nil, err
	}
	if response != nil {
		defer service.CloseResponseBodyGracefully(response)
		if response.StatusCode == http.StatusCreated && attempt.acceptCreated && attempt.info.ApiType == constant.APITypeReplicate {
			response.StatusCode = http.StatusOK
		}
		if response.StatusCode != http.StatusOK {
			return nil, attempt.mapError(service.RelayErrorHandler(attempt.ctx.Request.Context(), response, false))
		}
		contentType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
		attempt.info.IsStream = attempt.info.IsStream || contentType == "text/event-stream"
	}
	var usage any
	if decode != nil {
		usage, err = decode(response)
	} else {
		usage, err = attempt.adaptor.DoResponse(attempt.ctx, response, attempt.info)
	}
	if err != nil {
		if response != nil {
			err = service.RedactUpstreamGatewayError(err, service.UpstreamRequestSecrets(response.Request)...)
		}
		return nil, attempt.mapError(err)
	}
	switch facts := usage.(type) {
	case *dto.Usage:
		if facts != nil {
			return facts, nil
		}
	case dto.Usage:
		return &facts, nil
	case nil:
		return &dto.Usage{}, nil
	default:
		return nil, types.NewErrorWithStatusCode(fmt.Errorf("上游返回了无效使用量类型 %T。", usage), types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	return &dto.Usage{}, nil
}
func (attempt *protocolAttempt) convertedBody(disabled bool, convert func() (any, error), validate func([]byte) *types.GatewayError) (attemptBody, *types.GatewayError) {
	if attempt.passThrough {
		return attempt.rawBody()
	}
	if err := protocolCancellation(attempt.ctx); err != nil {
		return attemptBody{}, err
	}
	payload, err := convert()
	if err != nil {
		return attemptBody{}, newConvertRequestFailedError(attempt.ctx, attempt.info, err)
	}
	if reader, ok := payload.(io.Reader); ok {
		if storage, shared := reader.(common.BodyStorage); shared {
			return attemptBody{reader: common.NewReplayableBodyReader(storage)}, nil
		}
		owner, _ := reader.(io.Closer)
		return attemptBody{reader: reader, owner: owner}, nil
	}
	return attempt.jsonBody(payload, disabled, validate)
}

// Clone Go values rather than JSON wire fields: billing parameters, raw source
// data and reasoning metadata intentionally marked json:"-" must survive.
func cloneProtocolDTO[T any](source T) (T, error) {
	var empty T
	cloned, err := cloneProtocolValue(reflect.ValueOf(source), 0)
	if err != nil {
		return empty, err
	}
	if !cloned.IsValid() {
		return empty, nil
	}
	return cloned.Interface().(T), nil
}

func cloneProtocolValue(source reflect.Value, depth int) (reflect.Value, error) {
	if !source.IsValid() {
		return source, nil
	}
	if depth > 128 {
		return reflect.Value{}, errors.New("请求数据嵌套层数过深。")
	}
	switch source.Kind() {
	case reflect.Pointer, reflect.Interface:
		if source.IsNil() {
			return reflect.Zero(source.Type()), nil
		}
		inner, err := cloneProtocolValue(source.Elem(), depth+1)
		if err != nil {
			return reflect.Value{}, err
		}
		if source.Kind() == reflect.Pointer {
			target := reflect.New(source.Type().Elem())
			target.Elem().Set(inner)
			return target, nil
		}
		target := reflect.New(source.Type()).Elem()
		target.Set(inner)
		return target, nil
	case reflect.Slice:
		if source.IsNil() {
			return reflect.Zero(source.Type()), nil
		}
		target := reflect.MakeSlice(source.Type(), source.Len(), source.Len())
		if source.Type().Elem().Kind() == reflect.Uint8 {
			reflect.Copy(target, source)
			return target, nil
		}
		for index := range source.Len() {
			value, err := cloneProtocolValue(source.Index(index), depth+1)
			if err != nil {
				return reflect.Value{}, err
			}
			target.Index(index).Set(value)
		}
		return target, nil
	case reflect.Array:
		target := reflect.New(source.Type()).Elem()
		for index := range source.Len() {
			value, err := cloneProtocolValue(source.Index(index), depth+1)
			if err != nil {
				return reflect.Value{}, err
			}
			target.Index(index).Set(value)
		}
		return target, nil
	case reflect.Map:
		if source.IsNil() {
			return reflect.Zero(source.Type()), nil
		}
		target := reflect.MakeMapWithSize(source.Type(), source.Len())
		iterator := source.MapRange()
		for iterator.Next() {
			value, err := cloneProtocolValue(iterator.Value(), depth+1)
			if err != nil {
				return reflect.Value{}, err
			}
			target.SetMapIndex(iterator.Key(), value)
		}
		return target, nil
	case reflect.Struct:
		target := reflect.New(source.Type()).Elem()
		target.Set(source)
		for index := range source.NumField() {
			if !target.Field(index).CanSet() || !source.Field(index).CanInterface() {
				continue
			}
			value, err := cloneProtocolValue(source.Field(index), depth+1)
			if err != nil {
				return reflect.Value{}, err
			}
			target.Field(index).Set(value)
		}
		return target, nil
	default:
		return source, nil
	}
}
