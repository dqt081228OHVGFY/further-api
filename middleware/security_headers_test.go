package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBrowserSecurityHeadersPreserveResponseAndArtifactSandbox(t *testing.T) {
	engine := gin.New()
	engine.Use(SecurityHeaders())
	engine.GET("/", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte("<main>further-api</main>"))
	})
	engine.GET("/artifact", func(c *gin.Context) {
		c.Header("Content-Security-Policy", "sandbox; default-src 'none'")
		c.Data(http.StatusOK, "image/png", []byte{137, 80, 78, 71})
	})
	engine.GET("/api", func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.JSON(http.StatusForbidden, gin.H{"success": false})
	})
	for _, path := range []string{"/", "/artifact", "/api"} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))
		require.Equal(t, "SAMEORIGIN", response.Header().Get("X-Frame-Options"))
		if path == "/artifact" {
			require.Equal(t, "sandbox; default-src 'none'", response.Header().Get("Content-Security-Policy"))
			require.Equal(t, []byte{137, 80, 78, 71}, response.Body.Bytes())
		} else {
			require.Contains(t, response.Header().Get("Content-Security-Policy"), "frame-ancestors 'self'")
		}
		if path == "/api" {
			require.Equal(t, http.StatusForbidden, response.Code)
			require.Equal(t, "*", response.Header().Get("Access-Control-Allow-Origin"))
		}
	}
}
