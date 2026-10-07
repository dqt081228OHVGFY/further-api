package router

import (
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/controller"
	"github.com/dqt081228OHVGFY/further-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetRouter(router *gin.Engine, assets WebAssets) {
	SetApiRouter(router)
	SetDashboardRouter(router)
	SetRelayRouter(router)
	SetTaskPluginProtocolRouter(router)
	SetVideoRouter(router)
	SetTaskRouter(router)
	pluginDispatcher := SetPluginRouter(router)
	frontendBaseUrl := os.Getenv("FRONTEND_BASE_URL")
	if common.IsMasterNode && frontendBaseUrl != "" {
		frontendBaseUrl = ""
		common.SysLog("FRONTEND_BASE_URL is ignored on master node")
	}
	if frontendBaseUrl == "" {
		SetWebRouter(router, assets, pluginDispatcher)
	} else {
		router.NoRoute(
			pluginDispatcher,
			middleware.RouteTag("web"),
			middleware.AccessTokenAudit(),
			redirectFrontend(frontendBaseUrl),
		)
	}
}

// Only browser navigation is delegated to a separate frontend. Unknown API
// requests retain gateway JSON errors, including when no local assets exist.
func redirectFrontend(base string) gin.HandlerFunc {
	parsed, err := url.Parse(strings.TrimSpace(base))
	valid := err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" && strings.IndexAny(base, "\r\n") == -1
	return func(c *gin.Context) {
		if (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) || backendWebPath(c.Request.URL.Path) || c.Request.URL.Path == "/gateway.html" {
			controller.RelayNotFound(c)
			return
		}
		if !valid {
			c.Header("Cache-Control", "no-store")
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "code": "FRONTEND_UNAVAILABLE", "message": "further-api frontend URL is invalid"})
			return
		}
		target := *parsed
		target.Path = strings.TrimSuffix(parsed.Path, "/") + c.Request.URL.Path
		target.RawPath = ""
		target.RawQuery = c.Request.URL.RawQuery
		c.Redirect(http.StatusTemporaryRedirect, target.String())
	}
}
