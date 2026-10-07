package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	kitutil "github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert/kitutil"
)

// GatewayError separates the original cause, the public message and the
// upstream protocol payload. Wrapping takes a snapshot so request-specific
// retry/status/redaction policies cannot mutate a shared upstream error.
type GatewayError struct {
	Err            error
	RelayError     any
	StatusCode     int
	Metadata       json.RawMessage
	errorType      ErrorType
	errorCode      ErrorCode
	skipRetry      bool
	recordErrorLog *bool
	publicMessage  string
	messageSet     bool
}

type GatewayErrorOptions func(*GatewayError)

func (e *GatewayError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *GatewayError) GetErrorCode() ErrorCode {
	if e == nil {
		return ""
	}
	return e.errorCode
}

func (e *GatewayError) GetErrorType() ErrorType {
	if e == nil {
		return ""
	}
	if e.errorType == "" {
		return ErrorTypeGatewayError
	}
	return e.errorType
}

func (e *GatewayError) Error() string {
	if e == nil {
		return ""
	}
	if e.messageSet {
		return e.publicMessage
	}
	if e.Err != nil {
		if message := e.Err.Error(); message != "" {
			return message
		}
	}
	if e.errorCode != "" {
		return string(e.errorCode)
	}
	return string(e.GetErrorType())
}

func (e *GatewayError) SetMessage(message string) {
	if e != nil {
		e.publicMessage, e.messageSet = message, true
	}
}

func (e *GatewayError) MaskSensitiveError() string {
	if e == nil {
		return ""
	}
	return kitutil.MaskSensitiveInfo(e.Error())
}

func (e *GatewayError) statusMessage(message string) string {
	if e == nil || e.StatusCode == 0 {
		return message
	}
	if message == "" {
		return fmt.Sprintf("status_code=%d", e.StatusCode)
	}
	return fmt.Sprintf("status_code=%d, %s", e.StatusCode, message)
}

func (e *GatewayError) ErrorWithStatusCode() string {
	return e.statusMessage(e.Error())
}

func (e *GatewayError) MaskSensitiveErrorWithStatusCode() string {
	return e.statusMessage(e.MaskSensitiveError())
}

func (e *GatewayError) ToOpenAIError() OpenAIError {
	if e == nil {
		return OpenAIError{}
	}
	result := OpenAIError{Type: string(e.GetErrorType()), Code: e.errorCode}
	switch upstream := e.RelayError.(type) {
	case OpenAIError:
		result = upstream
		result.Metadata = append(json.RawMessage(nil), upstream.Metadata...)
	case ClaudeError:
		result.Type = upstream.Type
	}
	// Never use the old upstream message after a controller has replaced it.
	result.Message = e.MaskSensitiveError()
	if result.Message == "" {
		result.Message = string(e.GetErrorType())
	}
	return result
}

func (e *GatewayError) ToClaudeError() ClaudeError {
	if e == nil {
		return ClaudeError{}
	}
	result := ClaudeError{Type: string(e.GetErrorType()), Message: e.MaskSensitiveError()}
	switch upstream := e.RelayError.(type) {
	case OpenAIError:
		if upstream.Code != nil {
			result.Type = fmt.Sprint(upstream.Code)
		}
	case ClaudeError:
		result.Type = upstream.Type
	}
	if result.Message == "" {
		result.Message = string(e.GetErrorType())
	}
	return result
}

func snapshotGatewayError(err error, code ErrorCode) *GatewayError {
	var prior *GatewayError
	if errors.As(err, &prior) && prior != nil {
		copy := *prior
		copy.Metadata = append(json.RawMessage(nil), prior.Metadata...)
		if prior.recordErrorLog != nil {
			value := *prior.recordErrorLog
			copy.recordErrorLog = &value
		}
		if upstream, ok := prior.RelayError.(OpenAIError); ok {
			upstream.Metadata = append(json.RawMessage(nil), upstream.Metadata...)
			copy.RelayError = upstream
		}
		return &copy
	}
	return &GatewayError{Err: err, errorCode: code, errorType: ErrorTypeGatewayError, StatusCode: http.StatusInternalServerError}
}

func applyErrorOptions(e *GatewayError, options []GatewayErrorOptions) *GatewayError {
	for _, option := range options {
		if option != nil {
			option(e)
		}
	}
	if e.StatusCode < 100 || e.StatusCode > 599 {
		e.StatusCode = http.StatusInternalServerError
	}
	return e
}

func NewError(err error, code ErrorCode, options ...GatewayErrorOptions) *GatewayError {
	return applyErrorOptions(snapshotGatewayError(err, code), options)
}

func NewErrorWithStatusCode(err error, code ErrorCode, status int, options ...GatewayErrorOptions) *GatewayError {
	e := snapshotGatewayError(err, code)
	e.StatusCode = status
	return applyErrorOptions(e, options)
}

func NewOpenAIError(err error, code ErrorCode, status int, options ...GatewayErrorOptions) *GatewayError {
	e := snapshotGatewayError(err, code)
	if e.RelayError == nil {
		e.RelayError = OpenAIError{Message: e.Error(), Type: string(code), Code: code}
		e.errorType = ErrorTypeOpenAIError
	}
	e.StatusCode = status
	return applyErrorOptions(e, options)
}

func InitOpenAIError(code ErrorCode, status int, options ...GatewayErrorOptions) *GatewayError {
	return NewOpenAIError(nil, code, status, options...)
}

func WithOpenAIError(upstream OpenAIError, status int, options ...GatewayErrorOptions) *GatewayError {
	if upstream.Type == "" {
		upstream.Type = string(ErrorTypeUpstreamError)
	}
	code := "unknown_error"
	if upstream.Code != nil {
		code = fmt.Sprint(upstream.Code)
	}
	upstream.Metadata = append(json.RawMessage(nil), upstream.Metadata...)
	e := &GatewayError{Err: errors.New(upstream.Message), RelayError: upstream, StatusCode: status, errorType: ErrorTypeOpenAIError, errorCode: ErrorCode(code), Metadata: append(json.RawMessage(nil), upstream.Metadata...)}
	return applyErrorOptions(e, options)
}

func WithClaudeError(upstream ClaudeError, status int, options ...GatewayErrorOptions) *GatewayError {
	if upstream.Type == "" {
		upstream.Type = string(ErrorTypeUpstreamError)
	}
	e := &GatewayError{Err: errors.New(upstream.Message), RelayError: upstream, StatusCode: status, errorType: ErrorTypeClaudeError, errorCode: ErrorCode(upstream.Type)}
	return applyErrorOptions(e, options)
}

func IsChannelError(e *GatewayError) bool {
	return e != nil && strings.HasPrefix(string(e.errorCode), "channel:")
}

func IsSkipRetryError(e *GatewayError) bool { return e != nil && e.skipRetry }

func IsRecordErrorLog(e *GatewayError) bool {
	return e != nil && (e.recordErrorLog == nil || *e.recordErrorLog)
}

func ErrOptionWithSkipRetry() GatewayErrorOptions {
	return func(e *GatewayError) { e.skipRetry = true }
}

func ErrOptionWithNoRecordErrorLog() GatewayErrorOptions {
	return func(e *GatewayError) { value := false; e.recordErrorLog = &value }
}

func ErrOptionWithStatusCode(status int) GatewayErrorOptions {
	return func(e *GatewayError) { e.StatusCode = status }
}

func ErrOptionWithHideErrMsg(message string) GatewayErrorOptions {
	return func(e *GatewayError) { e.SetMessage(message) }
}
