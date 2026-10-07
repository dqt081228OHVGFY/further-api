package router

import (
	"strings"

	"github.com/dqt081228OHVGFY/further-api/controller"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/gin-gonic/gin"
)

func SetFurtherWorkspaceRouter(api *gin.RouterGroup) {
	site := api.Group("/further/workspace/site-settings", middleware.RootAuth(), middleware.DisableCache())
	site.GET("", controller.FurtherWorkspaceSiteSettings)
	site.PUT("", middleware.CriticalRateLimit(), controller.FurtherSaveWorkspaceSiteSettings)
	api.GET("/further/workspace/bootstrap", middleware.DisableCache(), func(c *gin.Context) {
		if strings.TrimSpace(c.GetHeader("Authorization")) != "" {
			middleware.UserAuth()(c)
			return
		}
		c.Next()
	}, controller.FurtherWorkspaceBootstrap)
}
