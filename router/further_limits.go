package router

import (
	"github.com/dqt081228OHVGFY/further-api/controller"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/dqt081228OHVGFY/further-api/service/authz"
	"github.com/gin-gonic/gin"
)

func SetFurtherLimitsRouter(apiRouter *gin.RouterGroup) {
	read := apiRouter.Group("/further", middleware.AdminAuth(), middleware.DisableCache())
	read.GET("/models/access", controller.FurtherModelAccessList)
	read.GET("/models/:id/access", controller.FurtherModelAccessGet)
	read.GET("/channels/policies", middleware.RequirePermission(authz.ChannelRead), controller.FurtherChannelPoliciesGet)
	read.GET("/channels/:id/policy", middleware.RequirePermission(authz.ChannelRead), controller.FurtherChannelPolicyGet)
	read.GET("/channels/:id/keys", middleware.RequirePermission(authz.ChannelRead), controller.FurtherChannelKeysGet)
	write := apiRouter.Group("/further", middleware.RootAuth(), middleware.DisableCache(), middleware.CriticalRateLimit())
	write.PUT("/models/:id/access", controller.FurtherModelAccessPut)
	write.PUT("/channels/:id/policy", controller.FurtherChannelPolicyPut)
	write.POST("/channels/:id/keys", controller.FurtherChannelKeySave)
	write.PUT("/channels/:id/keys/:key_id", controller.FurtherChannelKeySave)
	write.PATCH("/channels/:id/keys/:key_id/status", controller.FurtherChannelKeySave)
	write.DELETE("/channels/:id/keys/:key_id", controller.FurtherChannelKeyDelete)
}
