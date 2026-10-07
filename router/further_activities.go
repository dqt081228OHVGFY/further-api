package router

import (
	"github.com/dqt081228OHVGFY/further-api/controller"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/gin-gonic/gin"
)

func SetFurtherActivitiesRouter(api *gin.RouterGroup) {
	api.GET("/further/activities/theme", middleware.DisableCache(), controller.FurtherPublicActivityTheme)
	user := api.Group("/user/activities", middleware.UserAuth(), middleware.DisableCache())
	user.GET("", controller.FurtherUserActivities)
	user.POST("/claim", middleware.CriticalRateLimit(), controller.FurtherClaimActivity)
	user.PUT("/subscription", controller.FurtherHolidayFollowRetired)
	admin := api.Group("/admin/activities", middleware.RootAuth(), middleware.DisableCache())
	admin.GET("", controller.FurtherAdminActivities)
	admin.PUT("", middleware.CriticalRateLimit(), controller.FurtherSaveActivity)
	admin.PUT("/checkin", middleware.CriticalRateLimit(), controller.FurtherConfigureActivityCheckin)
}
