package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	taskdto "github.com/dqt081228OHVGFY/further-api/dto"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
)

const maxUpstreamErrorBodyBytes = 1 << 20

func MidjourneyErrorWrapper(code int, description string) *taskdto.MidjourneyResponse {
	return &taskdto.MidjourneyResponse{Code: code, Description: description}
}

func MidjourneyErrorWithStatusCodeWrapper(code int, description string, status int) *taskdto.MidjourneyResponseWithStatusCode {
	return &taskdto.MidjourneyResponseWithStatusCode{StatusCode: status, Response: *MidjourneyErrorWrapper(code, description)}
}

func publicLocalError(err error) string {
	if err == nil {
		return "网关暂时无法完成请求，请稍后重试。"
	}
	return common.MaskSensitiveInfo(err.Error())
}

func ClaudeErrorWrapper(err error, code string, status int) *dto.ClaudeErrorWithStatusCode {
	return &dto.ClaudeErrorWithStatusCode{
		Error:      types.ClaudeError{Message: publicLocalError(err), Type: "gateway_error"},
		StatusCode: status,
	}
}

func ClaudeErrorWrapperLocal(err error, code string, status int) *dto.ClaudeErrorWithStatusCode {
	result := ClaudeErrorWrapper(err, code, status)
	result.LocalError = true
	return result
}

func upstreamResponseStatus(response *http.Response) int {
	if response == nil || response.StatusCode < 200 || response.StatusCode > 599 {
		return http.StatusBadGateway
	}
	return response.StatusCode
}

// Failure bodies are separate from successful protocol payloads: bounded,
// always closed, and interruptible even before the provider's body has data.
func RelayErrorHandler(ctx context.Context, response *http.Response, showBody bool) *types.GatewayError {
	status := upstreamResponseStatus(response)
	fallback := fmt.Sprintf("bad response status code %d", status)
	if response == nil || response.Body == nil {
		return types.NewOpenAIError(errors.New(fallback), types.ErrorCodeBadResponseStatusCode, status)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	defer CloseResponseBodyGracefully(response)
	stop := context.AfterFunc(ctx, func() { _ = response.Body.Close() })
	defer stop()
	if err := ctx.Err(); err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponseStatusCode, status, types.ErrOptionWithSkipRetry())
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, maxUpstreamErrorBodyBytes+1))
	if readErr != nil {
		cause := readErr
		if ctx.Err() != nil {
			cause = ctx.Err()
		}
		return types.NewErrorWithStatusCode(fmt.Errorf("read upstream failure: %w", cause), types.ErrorCodeBadResponseStatusCode, status,
			types.ErrOptionWithHideErrMsg(fallback), types.ErrOptionWithSkipRetry())
	}
	truncated := len(body) > maxUpstreamErrorBodyBytes
	if truncated {
		body = body[:maxUpstreamErrorBodyBytes]
	}
	body = []byte(RedactKnownUpstreamSecrets(string(body), UpstreamRequestSecrets(response.Request)...))
	text := string(body)
	var envelope dto.GeneralErrorResponse
	var gatewayErr *types.GatewayError
	if !truncated && json.Unmarshal(body, &envelope) == nil {
		if structured := envelope.TryToOpenAIError(); structured != nil {
			if len(structured.Metadata) == 0 {
				structured.Metadata = append(json.RawMessage(nil), envelope.Metadata...)
			}
			gatewayErr = types.WithOpenAIError(*structured, status)
		} else if message := strings.TrimSpace(envelope.ToMessage()); message != "" {
			gatewayErr = types.NewOpenAIError(errors.New(message), types.ErrorCodeBadResponseStatusCode, status)
		}
	}
	if gatewayErr == nil {
		preview := common.LocalLogPreview(common.MaskSensitiveInfo(text))
		if truncated {
			preview += fmt.Sprintf(" [body exceeded %d bytes]", maxUpstreamErrorBodyBytes)
		}
		logger.LogError(ctx, fmt.Sprintf("%s, body: %s", fallback, preview))
		gatewayErr = types.NewOpenAIError(errors.New(fallback), types.ErrorCodeBadResponseStatusCode, status)
	}
	if showBody {
		diagnostic := fmt.Sprintf("%s, message: %s, body: %s", fallback, gatewayErr.Error(), common.MaskSensitiveInfo(text))
		if truncated {
			diagnostic += " [truncated upstream error body]"
		}
		gatewayErr.Err = errors.New(diagnostic)
	}
	return gatewayErr
}

func ResetStatusCode(gatewayErr *types.GatewayError, configuration string) {
	if gatewayErr == nil || gatewayErr.StatusCode == http.StatusOK || strings.TrimSpace(configuration) == "" {
		return
	}
	var mapping map[string]json.RawMessage
	if json.Unmarshal([]byte(configuration), &mapping) != nil {
		return
	}
	raw, found := mapping[strconv.Itoa(gatewayErr.StatusCode)]
	if !found {
		return
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return
	}
	status, valid := parseStatusCodeMappingValue(value)
	if valid {
		gatewayErr.StatusCode = status
	}
}

func parseStatusCodeMappingValue(value any) (int, bool) {
	var status int
	switch typed := value.(type) {
	case string:
		number, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		status = number
	case json.Number:
		number, err := strconv.Atoi(typed.String())
		if err != nil {
			return 0, false
		}
		status = number
	case int:
		status = typed
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || typed != math.Trunc(typed) || typed < 200 || typed > 599 {
			return 0, false
		}
		status = int(typed)
	default:
		return 0, false
	}
	return status, status >= 200 && status <= 599
}

func TaskErrorWrapper(err error, code string, status int) *taskdto.TaskError {
	return &taskdto.TaskError{Code: code, Message: publicLocalError(err), StatusCode: status, Error: err}
}

func TaskErrorWrapperLocal(err error, code string, status int) *taskdto.TaskError {
	result := TaskErrorWrapper(err, code, status)
	result.LocalError = true
	return result
}

func TaskErrorFromAPIError(err *types.GatewayError) *taskdto.TaskError {
	if err == nil {
		return nil
	}
	return &taskdto.TaskError{
		Code: string(err.GetErrorCode()), Message: err.MaskSensitiveError(),
		StatusCode: err.StatusCode, Error: err,
	}
}
