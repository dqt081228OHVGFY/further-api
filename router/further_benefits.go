package router

import (
	"github.com/dqt081228OHVGFY/further-api/controller"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/gin-gonic/gin"
)

func SetFurtherBusinessRouter(apiRouter *gin.RouterGroup) {
	apiRouter.GET("/further/registration/config", middleware.DisableCache(), controller.FurtherGetRegistrationConfig)
	user := apiRouter.Group("/user/public-benefits/token-loan", middleware.UserAuth(), middleware.DisableCache())
	user.GET("", controller.FurtherGetLoan)
	user.POST("/borrow", middleware.CriticalRateLimit(), controller.FurtherBorrow)
	user.POST("/check-in", middleware.CriticalRateLimit(), controller.FurtherLoanCheckIn)
	user.POST("/loss-pool/cut", middleware.CriticalRateLimit(), controller.FurtherCutLoanPool)
	admin := apiRouter.Group("/admin", middleware.AdminAuth(), middleware.DisableCache())
	admin.GET("/settings/token-loan", controller.FurtherGetLoanSettings)
	admin.GET("/settings/registration", controller.FurtherGetRegistrationSettings)
	admin.GET("/registration-codes", middleware.SearchRateLimit(), controller.FurtherListRegistrationCodes)
	admin.GET("/prepaid-cards", middleware.SearchRateLimit(), controller.FurtherPrepaidList)
	root := apiRouter.Group("/admin", middleware.RootAuth(), middleware.DisableCache(), middleware.CriticalRateLimit())
	root.PUT("/settings/token-loan", controller.FurtherPutLoanSettings)
	root.PATCH("/settings/registration", controller.FurtherPatchRegistrationSettings)
	root.POST("/registration-codes/generate", controller.FurtherGenerateRegistrationCodes)
	root.DELETE("/registration-codes/used", controller.FurtherDeleteRegistrationCodes)
	root.DELETE("/registration-codes/:id", controller.FurtherDeleteRegistrationCodes)
	root.DELETE("/registration-codes", controller.FurtherDeleteRegistrationCodes)
	root.POST("/prepaid-cards/generate", controller.FurtherPrepaidGenerate)
	root.DELETE("/prepaid-cards/used", controller.FurtherPrepaidDeleteUsed)
	root.DELETE("/prepaid-cards/:id", controller.FurtherPrepaidDelete)
	root.DELETE("/prepaid-cards", controller.FurtherPrepaidDelete)
	prepaid := apiRouter.Group("/prepaid", middleware.UserAuth(), middleware.DisableCache())
	prepaid.POST("/redeem", middleware.CriticalRateLimit(), controller.FurtherPrepaidRedeem)
	prepaid.GET("/balance", controller.FurtherPrepaidBalance)
}
