package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func SetRelayForLDCRouter(api *gin.RouterGroup) {
	user := api.Group("/user/ldc", middleware.UserAuth(), middleware.DisableCache())
	user.GET("/info", controller.RelayForLDCInfo)
	user.POST("/quote", middleware.CriticalRateLimit(), controller.RelayForLDCQuote)
	user.POST("/pay", middleware.CriticalRateLimit(), controller.RelayForLDCPay)
	user.GET("/orders", controller.RelayForLDCOrders)
	user.GET("/orders/:trade_no", controller.RelayForLDCOrderStatus)
	api.POST("/subscription/ldc/pay", middleware.UserAuth(), middleware.DisableCache(), middleware.CriticalRateLimit(), controller.RelayForLDCSubscriptionPay)
	admin := api.Group("/admin/ldc", middleware.RootAuth(), middleware.DisableCache())
	admin.GET("/config", controller.RelayForGetLDCConfig)
	admin.PUT("/config", middleware.CriticalRateLimit(), controller.RelayForSaveLDCConfig)
	api.GET("/ldc/notify", middleware.CriticalRateLimit(), controller.RelayForLDCNotify)
	api.POST("/ldc/notify", middleware.CriticalRateLimit(), controller.RelayForLDCNotify)
	api.GET("/ldc/return", controller.RelayForLDCReturn)
}
