package controller

import (
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

// CompleteFurtherWelcome records a monotonic account preference. The subject
// comes exclusively from UserAuth; request bodies cannot choose another user
// or replace the account's notification, language or billing preferences.
func CompleteFurtherWelcome(c *gin.Context) {
	userID := c.GetInt("id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "code": "AUTH_REQUIRED", "message": "请先登录"})
		return
	}
	if err := model.MutateAccountPreferences(c.Request.Context(), userID, func(_ *model.User, preferences *dto.UserSetting) error {
		preferences.WelcomeCompleted = true
		return nil
	}); err != nil {
		common.SysError("complete account welcome: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "WELCOME_SAVE_FAILED", "message": "欢迎页状态保存失败，请稍后重试"})
		return
	}
	common.ApiSuccess(c, gin.H{"welcome_completed": true})
}
