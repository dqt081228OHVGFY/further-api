package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/oauth"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

func FurtherWorkspaceSiteSettings(c *gin.Context) {
	settings, err := service.ReadWorkspaceSiteSettings(c.Request.Context(), oauth.LinuxDORedirectURI(c))
	if err != nil {
		c.Status(499)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"settings": settings}})
}

func FurtherSaveWorkspaceSiteSettings(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var change service.WorkspaceSiteChange
	if err := decoder.Decode(&change); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "设置格式有误，请重新读取后提交。"})
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请只提交一份设置。"})
		return
	}
	if err := service.SaveWorkspaceSiteSettings(c.Request.Context(), change); err != nil {
		var input *service.WorkspaceSiteInputError
		if errors.As(err, &input) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": input.Message})
		} else {
			common.SysError("save workspace site settings: " + err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "设置未能保存，请稍后重试。"})
		}
		return
	}
	recordManageAudit(c, "workspace.site_settings", map[string]any{"linuxdo_enabled_changed": change.LinuxDOEnabled != nil, "credentials_changed": change.LinuxDOClientID != nil || (change.LinuxDOClientSecret != nil && *change.LinuxDOClientSecret != "")})
	FurtherWorkspaceSiteSettings(c)
}
