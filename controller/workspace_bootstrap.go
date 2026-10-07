package controller

import (
	"errors"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/oauth"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func FurtherWorkspaceBootstrap(c *gin.Context) {
	workspace, err := service.ReadWorkspaceBootstrap(c.Request.Context(), c.GetInt("id"), c.GetInt("role"), oauth.LinuxDORedirectURI(c))
	if errors.Is(err, service.ErrWorkspaceAccountUnavailable) || errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "code": "AUTH_USER_UNAVAILABLE", "message": "账户状态已变化，请重新登录"})
		return
	}
	if err != nil {
		common.SysError("read workspace bootstrap: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "WORKSPACE_LOAD_FAILED", "message": "工作台加载失败，请稍后重试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": workspace})
}
