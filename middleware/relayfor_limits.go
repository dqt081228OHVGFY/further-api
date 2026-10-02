package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func EnforceRelayForModelAccess(c *gin.Context, modelName string) bool {
	allowed, err := model.RelayForSubscriptionModelAllowed(c.GetInt("id"), modelName)
	if err != nil {
		abortWithOpenAiMessage(c, http.StatusServiceUnavailable, "无法确认模型订阅权限，请稍后重试", "subscription_access_unavailable")
		return false
	}
	if !allowed {
		abortWithOpenAiMessage(c, http.StatusForbidden, "此模型需要有效订阅授权", "subscription_required")
		return false
	}
	return true
}
