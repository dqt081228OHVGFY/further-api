package router

import (
	"github.com/dqt081228OHVGFY/further-api/controller"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/gin-gonic/gin"
)

func SetFurtherSubscriptionRouter(apiRouter *gin.RouterGroup) {
	user := apiRouter.Group("/user", middleware.UserAuth())
	user.GET("/subscription-plans", controller.FurtherUserSubscriptionPlans)
	user.GET("/subscriptions", controller.FurtherUserSubscriptions)
	user.DELETE("/subscriptions/:id", controller.FurtherDeleteUserSubscription)
	user.POST("/subscriptions/:id/abandon", controller.FurtherAbandonSubscription)
	user.GET("/timed-quotas", controller.FurtherUserTimedQuotas)
	user.PATCH("/billing-preference", controller.FurtherPatchBillingPreference)
	user.GET("/benefits", controller.FurtherUserBenefits)
	user.POST("/benefits/:id/claim", controller.FurtherClaimBenefit)
	user.DELETE("/benefits/:id", controller.FurtherDismissBenefit)
	admin := apiRouter.Group("/admin", middleware.AdminAuth())
	admin.GET("/subscription-plans", controller.FurtherAdminSubscriptionPlans)
	admin.POST("/subscription-plans", controller.FurtherSaveSubscriptionPlan)
	admin.PUT("/subscription-plans/:id", controller.FurtherSaveSubscriptionPlan)
	admin.PATCH("/subscription-plans/:id/status", controller.FurtherSubscriptionPlanStatus)
	admin.GET("/subscription-benefits", controller.FurtherAdminBenefitCampaigns)
	admin.POST("/subscription-benefits", controller.FurtherCreateBenefitCampaign)
	admin.DELETE("/subscription-benefits/:id/pending", controller.FurtherRevokeBenefitCampaign)
	admin.DELETE("/subscription-benefits/:id", controller.FurtherRevokeBenefitCampaign)
	admin.POST("/subscriptions/process-resets", controller.FurtherProcessSubscriptionResets)
}
