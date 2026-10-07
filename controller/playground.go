package controller

import (
	"errors"
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"net/http"
)

func Playground(c *gin.Context) {
	fail := func(status int, code types.ErrorCode, err error) {
		classified := types.NewErrorWithStatusCode(err, code, status, types.ErrOptionWithSkipRetry())
		c.JSON(status, gin.H{"error": classified.ToOpenAIError()})
	}
	if c.GetBool("use_access_token") {
		fail(http.StatusForbidden, types.ErrorCodeAccessDenied, errors.New("请使用账户会话打开在线调试。"))
		return
	}
	if err := c.Request.Context().Err(); err != nil {
		fail(http.StatusRequestTimeout, types.ErrorCodeInvalidRequest, err)
		return
	}
	userID := c.GetInt("id")
	if userID <= 0 {
		fail(http.StatusUnauthorized, types.ErrorCodeAccessDenied, errors.New("请先登录账户。"))
		return
	}
	owner, err := model.GetUserCacheContext(c.Request.Context(), userID)
	if err != nil {
		fail(http.StatusInternalServerError, types.ErrorCodeQueryDataError, err)
		return
	}
	if owner.Status != common.UserStatusEnabled {
		fail(http.StatusForbidden, types.ErrorCodeAccessDenied, errors.New("当前账户不可用。"))
		return
	}
	selected := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if selected == "" {
		selected = owner.Group
	}
	owner.WriteContext(c)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, selected)
	transient := &model.Token{UserId: userID, Name: "在线调试 · " + selected, Group: selected, UnlimitedQuota: true}
	if err := middleware.SetupContextForToken(c, transient); err != nil {
		fail(http.StatusBadRequest, types.ErrorCodeInvalidRequest, err)
		return
	}
	Relay(c, types.RelayFormatOpenAI)
}
