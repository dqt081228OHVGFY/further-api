package controller

import (
	"errors"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/i18n"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/oauth"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/service/business"
	"github.com/gin-gonic/gin"
)

func TestStatus(c *gin.Context) {
	if err := model.PingDBContext(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "数据库连接暂不可用"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "服务运行正常", "http_stats": middleware.GetStats()})
}

func GetStatus(c *gin.Context) {
	common.ApiSuccess(c, service.ReadWorkspaceStatus(oauth.LinuxDORedirectURI(c)))
}

func serveGatewayPublicContent(c *gin.Context, kind string) {
	content, err := business.ReadGatewayPublicContent(kind)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	serveRevalidatedJSON(c, content)
}
func GetNotice(c *gin.Context)          { serveGatewayPublicContent(c, "Notice") }
func GetAbout(c *gin.Context)           { serveGatewayPublicContent(c, "About") }
func GetUserAgreement(c *gin.Context)   { serveGatewayPublicContent(c, "UserAgreement") }
func GetPrivacyPolicy(c *gin.Context)   { serveGatewayPublicContent(c, "PrivacyPolicy") }
func GetMidjourney(c *gin.Context)      { serveGatewayPublicContent(c, "Midjourney") }
func GetHomePageContent(c *gin.Context) { serveGatewayPublicContent(c, "HomePageContent") }

func SendEmailVerification(c *gin.Context) {
	err := business.SendGatewayRegistrationEmailContext(c.Request.Context(), c.Query("email"))
	if errors.Is(err, model.ErrEmailAlreadyTaken) {
		common.ApiErrorI18n(c, i18n.MsgUserEmailAlreadyTaken)
		return
	}
	if errors.Is(err, service.ErrAccountEmailInvalid) || errors.Is(err, service.ErrAccountEmailRestricted) {
		writeSecurityOperationError(c, err)
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
