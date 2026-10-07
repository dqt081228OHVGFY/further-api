package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/gin-gonic/gin"
)

// One recovery boundary serves dashboard and provider traffic. Once a stream
// has committed headers, adding a JSON error would corrupt the wire protocol.
func RelayPanicRecover() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			c.Abort()
			if cause, ok := recovered.(error); ok && errors.Is(cause, http.ErrAbortHandler) {
				// Let net/http terminate the connection using its documented signal.
				panic(http.ErrAbortHandler)
			}
			if c.Request != nil && c.Request.Context().Err() != nil {
				return
			}
			message := common.LocalLogPreview(common.MaskSensitiveInfo(fmt.Sprint(recovered)))
			logger.LogError(c, "gateway request panic: "+message)
			logger.LogDebug(c, "gateway panic stack: %s", debug.Stack())
			if c.Writer == nil || c.Writer.Written() {
				return
			}
			id := c.GetString(common.RequestIdKey)
			public := "网关暂时无法完成请求，请稍后重试或联系管理员。"
			path := ""
			if c.Request != nil && c.Request.URL != nil {
				path = c.Request.URL.Path
			}
			// Administrative APIs use the same localized notification envelope
			// as other business errors, so the website can show the failure.
			if path == "/api" || strings.HasPrefix(path, "/api/") {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false, "code": "GATEWAY_INTERNAL_ERROR",
					"message": public, "request_id": id,
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"message": common.MessageWithRequestId(public, id),
					"type":    "gateway_internal_error", "code": "gateway_internal_error",
				},
			})
		}()
		c.Next()
	}
}
