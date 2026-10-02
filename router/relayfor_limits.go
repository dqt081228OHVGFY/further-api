package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func SetRelayForLimitsRouter(apiRouter *gin.RouterGroup) {
	read := apiRouter.Group("/relayfor", middleware.AdminAuth(), middleware.DisableCache())
	read.GET("/models/access", controller.RelayForModelAccessList)
	read.GET("/models/:id/access", controller.RelayForModelAccessGet)
	read.GET("/channels/policies", controller.RelayForChannelPoliciesGet)
	read.GET("/channels/:id/policy", controller.RelayForChannelPolicyGet)
	read.GET("/channels/:id/keys", controller.RelayForChannelKeysGet)
	write := apiRouter.Group("/relayfor", middleware.RootAuth(), middleware.DisableCache(), middleware.CriticalRateLimit())
	write.PUT("/models/:id/access", controller.RelayForModelAccessPut)
	write.PUT("/channels/:id/policy", controller.RelayForChannelPolicyPut)
	write.POST("/channels/:id/keys", controller.RelayForChannelKeySave)
	write.PUT("/channels/:id/keys/:key_id", controller.RelayForChannelKeySave)
	write.PATCH("/channels/:id/keys/:key_id/status", controller.RelayForChannelKeySave)
	write.DELETE("/channels/:id/keys/:key_id", controller.RelayForChannelKeyDelete)
}
