package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func SetRelayForBusinessRouter(apiRouter *gin.RouterGroup) {
	apiRouter.GET("/relayfor/registration/config", middleware.DisableCache(), controller.RelayForGetRegistrationConfig)
	user := apiRouter.Group("/user/public-benefits/token-loan", middleware.UserAuth(), middleware.DisableCache())
	user.GET("", controller.RelayForGetLoan)
	user.POST("/borrow", middleware.CriticalRateLimit(), controller.RelayForBorrow)
	user.POST("/check-in", middleware.CriticalRateLimit(), controller.RelayForLoanCheckIn)
	user.POST("/loss-pool/cut", middleware.CriticalRateLimit(), controller.RelayForCutLoanPool)
	admin := apiRouter.Group("/admin", middleware.AdminAuth(), middleware.DisableCache())
	admin.GET("/settings/token-loan", controller.RelayForGetLoanSettings)
	admin.GET("/settings/registration", controller.RelayForGetRegistrationSettings)
	admin.GET("/registration-codes", middleware.SearchRateLimit(), controller.RelayForListRegistrationCodes)
	admin.GET("/prepaid-cards", middleware.SearchRateLimit(), controller.RelayForPrepaidList)
	root := apiRouter.Group("/admin", middleware.RootAuth(), middleware.DisableCache(), middleware.CriticalRateLimit())
	root.PUT("/settings/token-loan", controller.RelayForPutLoanSettings)
	root.PATCH("/settings/registration", controller.RelayForPatchRegistrationSettings)
	root.POST("/registration-codes/generate", controller.RelayForGenerateRegistrationCodes)
	root.DELETE("/registration-codes/used", controller.RelayForDeleteRegistrationCodes)
	root.DELETE("/registration-codes/:id", controller.RelayForDeleteRegistrationCodes)
	root.DELETE("/registration-codes", controller.RelayForDeleteRegistrationCodes)
	root.POST("/prepaid-cards/generate", controller.RelayForPrepaidGenerate)
	root.DELETE("/prepaid-cards/used", controller.RelayForPrepaidDeleteUsed)
	root.DELETE("/prepaid-cards/:id", controller.RelayForPrepaidDelete)
	root.DELETE("/prepaid-cards", controller.RelayForPrepaidDelete)
	prepaid := apiRouter.Group("/prepaid", middleware.UserAuth(), middleware.DisableCache())
	prepaid.POST("/redeem", middleware.CriticalRateLimit(), controller.RelayForPrepaidRedeem)
	prepaid.GET("/balance", controller.RelayForPrepaidBalance)
}
