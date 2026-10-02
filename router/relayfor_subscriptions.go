package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func SetRelayForSubscriptionRouter(apiRouter *gin.RouterGroup) {
	user := apiRouter.Group("/user", middleware.UserAuth())
	user.GET("/subscription-plans", controller.RelayForUserSubscriptionPlans)
	user.GET("/subscriptions", controller.RelayForUserSubscriptions)
	user.DELETE("/subscriptions/:id", controller.RelayForDeleteUserSubscription)
	user.POST("/subscriptions/:id/abandon", controller.RelayForAbandonSubscription)
	user.GET("/timed-quotas", controller.RelayForUserTimedQuotas)
	user.PATCH("/billing-preference", controller.RelayForPatchBillingPreference)
	user.GET("/benefits", controller.RelayForUserBenefits)
	user.POST("/benefits/:id/claim", controller.RelayForClaimBenefit)
	user.DELETE("/benefits/:id", controller.RelayForDismissBenefit)
	admin := apiRouter.Group("/admin", middleware.AdminAuth())
	admin.GET("/subscription-plans", controller.RelayForAdminSubscriptionPlans)
	admin.POST("/subscription-plans", controller.RelayForSaveSubscriptionPlan)
	admin.PUT("/subscription-plans/:id", controller.RelayForSaveSubscriptionPlan)
	admin.PATCH("/subscription-plans/:id/status", controller.RelayForSubscriptionPlanStatus)
	admin.GET("/subscription-benefits", controller.RelayForAdminBenefitCampaigns)
	admin.POST("/subscription-benefits", controller.RelayForCreateBenefitCampaign)
	admin.DELETE("/subscription-benefits/:id/pending", controller.RelayForRevokeBenefitCampaign)
	admin.DELETE("/subscription-benefits/:id", controller.RelayForRevokeBenefitCampaign)
	admin.POST("/subscriptions/process-resets", controller.RelayForProcessSubscriptionResets)
}
