package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayRecoveryUsesDashboardNotificationAndRequestID(t *testing.T) {
	engine := gin.New()
	engine.Use(RequestId(), RelayPanicRecover())
	engine.GET("/api/test", func(*gin.Context) { panic("sensitive implementation detail") })
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/test", nil))
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	var result map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
	require.Equal(t, false, result["success"])
	require.Equal(t, "GATEWAY_INTERNAL_ERROR", result["code"])
	require.Equal(t, recorder.Header().Get(common.RequestIdKey), result["request_id"])
	require.NotEmpty(t, result["request_id"])
	require.NotContains(t, recorder.Body.String(), "sensitive implementation detail")
}

func TestGatewayRecoveryPreservesAlreadyCommittedStream(t *testing.T) {
	engine := gin.New()
	engine.Use(RelayPanicRecover())
	engine.GET("/v1/responses", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		_, _ = c.Writer.WriteString("data: first\n\n")
		c.Writer.Flush()
		panic("after stream commitment")
	})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/responses", nil))
	require.Equal(t, "data: first\n\n", recorder.Body.String())
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
}
