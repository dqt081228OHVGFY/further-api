package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestLoggerRedactsQueryCredentialsWithoutChangingHandlerInputs(t *testing.T) {
	priorWriter := gin.DefaultWriter
	var output bytes.Buffer
	gin.DefaultWriter = &output
	t.Cleanup(func() { gin.DefaultWriter = priorWriter })
	engine := gin.New()
	SetUpLogger(engine)
	engine.GET("/v1beta/models", func(c *gin.Context) {
		require.Equal(t, "private-query-credential", c.Query("key"))
		require.Equal(t, "billing", c.Query("scope"))
		c.Status(http.StatusUnauthorized)
	})
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1beta/models?key=private-query-credential&scope=billing", nil))
	require.NotContains(t, output.String(), "private-query-credential")
	require.Contains(t, output.String(), "REDACTED")
	require.Contains(t, output.String(), "scope=billing")
	for _, path := range []string{"/v1/models?%6bey=secret", "/api/login?PASSWORD=secret", "/api/oauth/github?code=secret&state=secret", "/path?key=secret;other=bad"} {
		require.NotContains(t, redactLoggedRequestPath(path), "secret")
	}
}
