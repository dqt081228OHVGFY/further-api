package types

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestGatewayErrorRedactionIsAppliedToEveryWireProtocol(t *testing.T) {
	cause := errors.New("private upstream failure")
	err := NewOpenAIError(cause, ErrorCodeBadResponse, 502)
	err.SetMessage("请求失败，请联系管理员")
	if !errors.Is(err, cause) {
		t.Fatal("public message discarded original cause")
	}
	if got := err.ToOpenAIError().Message; got != "请求失败，请联系管理员" {
		t.Fatal(got)
	}
	if got := err.ToClaudeError().Message; got != "请求失败，请联系管理员" {
		t.Fatal(got)
	}
	if err.RelayError.(OpenAIError).Message != "private upstream failure" {
		t.Fatal("redaction mutated upstream payload")
	}
}

func TestGatewayErrorWrappingDoesNotMutateSourcePolicy(t *testing.T) {
	cause := errors.New("upstream disconnected")
	source := NewError(cause, ErrorCodeDoRequestFailed)
	wrapped := NewError(fmt.Errorf("relay: %w", source), ErrorCodeBadResponse, ErrOptionWithSkipRetry(), ErrOptionWithStatusCode(503), ErrOptionWithHideErrMsg("unavailable"))
	if IsSkipRetryError(source) || source.StatusCode != 500 || source.Error() != cause.Error() {
		t.Fatal("source error was mutated")
	}
	if !IsSkipRetryError(wrapped) || wrapped.StatusCode != 503 || wrapped.Error() != "unavailable" {
		t.Fatal("wrapper policy was not applied")
	}
	if wrapped.GetErrorCode() != ErrorCodeDoRequestFailed || !errors.Is(wrapped, cause) {
		t.Fatal("original code/cause lost")
	}
}

func TestGatewayErrorNilAndInvalidStatusDoNotPanic(t *testing.T) {
	var absent *GatewayError
	absent.SetMessage("ignored")
	if absent.ToOpenAIError().Message != "" || absent.ToClaudeError().Message != "" || absent.ErrorWithStatusCode() != "" {
		t.Fatal("unexpected nil error payload")
	}
	for _, err := range []*GatewayError{NewOpenAIError(nil, ErrorCodeInvalidRequest, 0), NewErrorWithStatusCode(nil, ErrorCodeInvalidRequest, 999), NewError(absent, ErrorCodeInvalidRequest, nil)} {
		if err.StatusCode != http.StatusInternalServerError || err.Error() == "" {
			t.Fatalf("invalid error: %#v", err)
		}
	}
}

func TestGatewayErrorMetadataIsCopiedWithoutLeakingIntoMessage(t *testing.T) {
	metadata := []byte(`{"provider_name":"upstream"}`)
	err := WithOpenAIError(OpenAIError{Message: "rate limited", Metadata: metadata}, 429)
	metadata[0] = 'X'
	if err.Error() != "rate limited" || err.Metadata[0] != '{' {
		t.Fatal("metadata changed message or was not copied")
	}
	wire := err.ToOpenAIError()
	wire.Metadata[0] = 'X'
	if err.ToOpenAIError().Metadata[0] != '{' {
		t.Fatal("wire output mutated stored metadata")
	}
}
