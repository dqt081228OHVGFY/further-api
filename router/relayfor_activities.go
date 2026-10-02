package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func SetRelayForActivitiesRouter(api *gin.RouterGroup) {
	api.GET("/relayfor/activities/theme", middleware.DisableCache(), controller.RelayForPublicActivityTheme)
	user := api.Group("/user/activities", middleware.UserAuth(), middleware.DisableCache())
	user.GET("", controller.RelayForUserActivities)
	user.POST("/claim", middleware.CriticalRateLimit(), controller.RelayForClaimActivity)
	user.PUT("/subscription", middleware.CriticalRateLimit(), controller.RelayForSetActivitySubscription)
	admin := api.Group("/admin/activities", middleware.RootAuth(), middleware.DisableCache())
	admin.GET("", controller.RelayForAdminActivities)
	admin.PUT("", middleware.CriticalRateLimit(), controller.RelayForSaveActivity)
	admin.PUT("/checkin", middleware.CriticalRateLimit(), controller.RelayForConfigureActivityCheckin)
}
