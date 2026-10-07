package controller

import (
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/gin-gonic/gin"
	"strings"
)

func managedSecurityOperator(c *gin.Context) model.AccountSecurityOperator {
	operator := model.AccountSecurityOperator{UserID: c.GetInt("id"), Role: c.GetInt("role")}
	if c.GetBool("use_access_token") {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) == 1 {
			operator.PersonalAccessTokenRef = model.AccessTokenFingerprint(parts[0])
		} else if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			operator.PersonalAccessTokenRef = model.AccessTokenFingerprint(parts[1])
		}
	}
	if identity, valid := middleware.GetSessionAuthIdentity(c); valid {
		operator.Session = &identity
	}
	return operator
}
