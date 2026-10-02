package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func SetRelayForStatsRouter(apiRouter *gin.RouterGroup) {
	user := apiRouter.Group("/user", middleware.UserAuth())
	user.GET("/overview", controller.RelayForUserOverview)
	user.GET("/model-health", controller.RelayForUserModelHealth)
	user.GET("/logs", controller.RelayForUserLogs)
	user.GET("/logs/export", controller.RelayForExportUserLogs)
	user.GET("/ui-preferences", controller.RelayForGetPreferences)
	user.PATCH("/ui-preferences", controller.RelayForPatchPreferences)
	admin := apiRouter.Group("/admin", middleware.AdminAuth())
	admin.GET("/overview", controller.RelayForAdminOverview)
	admin.GET("/model-health", controller.RelayForModelHealth)
	admin.GET("/logs", controller.RelayForAdminLogs)
	admin.GET("/token-usage", controller.RelayForAdminTokenUsage)
	admin.GET("/multi-user-stats", controller.RelayForAdminMultiUserStats)
	admin.GET("/traces", controller.RelayForAdminTraces)
	admin.GET("/traces/:request_id", controller.RelayForAdminTrace)
}
