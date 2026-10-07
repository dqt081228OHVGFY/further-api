package router

import (
	"github.com/dqt081228OHVGFY/further-api/controller"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/gin-gonic/gin"
)

func SetFurtherStatsRouter(apiRouter *gin.RouterGroup) {
	user := apiRouter.Group("/user", middleware.UserAuth())
	user.GET("/overview", controller.FurtherUserOverview)
	user.GET("/model-health", controller.FurtherUserModelHealth)
	user.GET("/logs", controller.FurtherUserLogs)
	user.GET("/logs/export", controller.FurtherExportUserLogs)
	user.GET("/ui-preferences", controller.FurtherGetPreferences)
	user.PATCH("/ui-preferences", controller.FurtherPatchPreferences)
	admin := apiRouter.Group("/admin", middleware.AdminAuth())
	admin.GET("/overview", controller.FurtherAdminOverview)
	admin.GET("/model-health", controller.FurtherModelHealth)
	admin.GET("/logs", controller.FurtherAdminLogs)
	admin.GET("/token-usage", controller.FurtherAdminTokenUsage)
	admin.GET("/multi-user-stats", controller.FurtherAdminMultiUserStats)
	admin.GET("/traces", controller.FurtherAdminTraces)
	admin.GET("/traces/:request_id", controller.FurtherAdminTrace)
}
