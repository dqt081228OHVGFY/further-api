package controller

import (
	"errors"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

type Setup struct {
	Status       bool   `json:"status"`
	RootInit     bool   `json:"root_init"`
	DatabaseType string `json:"database_type"`
}

type SetupRequest struct {
	Username           string `json:"username"`
	Password           string `json:"password"`
	ConfirmPassword    string `json:"confirmPassword"`
	SelfUseModeEnabled bool   `json:"SelfUseModeEnabled"`
	DemoSiteEnabled    bool   `json:"DemoSiteEnabled"`
}

func GetSetup(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": service.ReadWorkspaceSetup()})
}

func PostSetup(c *gin.Context) {
	var request SetupRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "初始化信息格式有误"})
		return
	}
	err := model.BootstrapGateway(c.Request.Context(), model.BootstrapRequest{
		Username: request.Username, Password: request.Password, ConfirmPassword: request.ConfirmPassword,
		SelfUseModeEnabled: request.SelfUseModeEnabled, DemoSiteEnabled: request.DemoSiteEnabled,
	})
	if err != nil {
		message := "初始化未完成，请重试；如持续失败，请查看服务日志"
		var inputError *model.SetupInputError
		if errors.Is(err, model.ErrSetupAlreadyCompleted) || errors.As(err, &inputError) {
			message = err.Error()
		} else {
			common.SysError("initialize further-api: " + err.Error())
		}
		c.JSON(http.StatusOK, gin.H{"success": false, "message": message})
		return
	}
	constant.Setup = true
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "further-api 初始化完成"})
}
