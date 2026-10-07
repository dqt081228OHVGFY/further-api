package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	taskdto "github.com/dqt081228OHVGFY/further-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUpstreamCredentialReflectionIsRemovedFromMessageMetadataAndLogs(t *testing.T) {
	const credential = "opaque_provider_credential_84030"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"type": "authentication_error", "code": "bad_credentials", "message": "Rejected credential " + strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), "metadata": map[string]any{"echo": r.Header.Get("Authorization")}}}))
	}))
	defer upstream.Close()
	request, err := http.NewRequest(http.MethodPost, upstream.URL, nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+credential)
	response, err := upstream.Client().Do(request)
	require.NoError(t, err)
	failure := RelayErrorHandler(context.Background(), response, true)
	require.Equal(t, http.StatusUnauthorized, failure.StatusCode)
	encoded, err := json.Marshal(failure.ToOpenAIError())
	require.NoError(t, err)
	require.NotContains(t, string(encoded), credential)
	require.Contains(t, string(encoded), "bad_credentials")
	require.Contains(t, string(encoded), "REDACTED")
	require.NotContains(t, failure.Err.Error(), credential)
	var output bytes.Buffer
	common.LogWriterMu.Lock()
	priorWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &output
	common.LogWriterMu.Unlock()
	t.Cleanup(func() { common.LogWriterMu.Lock(); gin.DefaultErrorWriter = priorWriter; common.LogWriterMu.Unlock() })
	response = &http.Response{StatusCode: http.StatusBadGateway, Request: request, Body: io.NopCloser(strings.NewReader("failure details include " + credential))}
	RelayErrorHandler(context.Background(), response, false)
	require.NotContains(t, output.String(), credential)
}

func TestTaskAndStreamErrorRedactionPreservesOriginalCauseAndErrorCode(t *testing.T) {
	const credential = "opaque_callback_credential_84031"
	cause := errors.New("provider echoed " + credential)
	failure := &taskdto.TaskError{Code: "plugin_hook_failed", Message: cause.Error(), Error: cause, Data: map[string]any{"echo": credential}, StatusCode: 502}
	RedactUpstreamTaskError(failure, credential)
	require.ErrorIs(t, failure.Error, cause)
	require.NotContains(t, failure.Error.Error(), credential)
	encoded, err := json.Marshal(failure)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), credential)
	require.Contains(t, string(encoded), "plugin_hook_failed")
	secrets := UpstreamCredentialSecrets(`{"access_token":"token-opaque-84032","nested":{"client_secret":"secret-opaque-84033"}}`)
	redacted := RedactKnownUpstreamSecrets("token-opaque-84032 and secret-opaque-84033", secrets...)
	require.NotContains(t, redacted, "token-opaque-84032")
	require.NotContains(t, redacted, "secret-opaque-84033")
}
