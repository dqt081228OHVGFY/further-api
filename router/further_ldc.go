package router

import (
	"github.com/dqt081228OHVGFY/further-api/controller"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/gin-gonic/gin"
)

func SetFurtherLDCRouter(api *gin.RouterGroup) {
	user := api.Group("/user/ldc", middleware.UserAuth(), middleware.DisableCache())
	user.GET("/info", controller.FurtherLDCInfo)
	user.POST("/quote", middleware.CriticalRateLimit(), controller.FurtherLDCQuote)
	user.POST("/pay", middleware.CriticalRateLimit(), controller.FurtherLDCPay)
	user.GET("/orders", controller.FurtherLDCOrders)
	user.GET("/orders/:trade_no", controller.FurtherLDCOrderStatus)
	api.POST("/subscription/ldc/pay", middleware.UserAuth(), middleware.DisableCache(), middleware.CriticalRateLimit(), controller.FurtherLDCSubscriptionPay)
	admin := api.Group("/admin/ldc", middleware.RootAuth(), middleware.DisableCache())
	admin.GET("/config", controller.FurtherGetLDCConfig)
	admin.PUT("/config", middleware.CriticalRateLimit(), controller.FurtherSaveLDCConfig)
	api.GET("/ldc/notify", middleware.CriticalRateLimit(), controller.FurtherLDCNotify)
	api.POST("/ldc/notify", middleware.CriticalRateLimit(), controller.FurtherLDCNotify)
	api.GET("/ldc/return", controller.FurtherLDCReturn)
}
