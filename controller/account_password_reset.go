package controller

import (
	"errors"
	"fmt"
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/i18n"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"html"
	"net/url"
)

type PasswordResetRequest struct {
	Email string `json:"email"`
	Token string `json:"token"`
}

func SendPasswordResetEmail(c *gin.Context) {
	email := model.NormalizeEmail(c.Query("email"))
	if common.Validate.Var(email, "required,email") != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	token, err := model.CreatePasswordResetFlowContext(c.Request.Context(), email)
	if err == nil {
		address, parseErr := url.Parse(system_setting.ServerAddress + "/user/reset")
		if parseErr == nil {
			query := address.Query()
			query.Set("email", email)
			query.Set("token", token)
			address.RawQuery = query.Encode()
			link := html.EscapeString(address.String())
			content := fmt.Sprintf("<p>您好，您正在重置 further-api 密码。</p><p>点击 <a href=\"%s\">重置密码</a>，链接在 %d 分钟内有效。</p><p>若非本人操作，请忽略此邮件。</p>", link, common.VerificationValidMinutes)
			_ = common.SendEmail("further-api 密码重置", email, content)
		}
	}
	// The public result never distinguishes absent or ambiguous email owners.
	common.ApiSuccess(c, nil)
}

func ResetPassword(c *gin.Context) {
	var input PasswordResetRequest
	if err := c.ShouldBindJSON(&input); err != nil || input.Email == "" || input.Token == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	input.Email = model.NormalizeEmail(input.Email)
	password := common.GenerateVerificationCode(12)
	err := model.ResetPasswordWithFlowContext(c.Request.Context(), input.Email, input.Token, password)
	if errors.Is(err, model.ErrAuthFlowInvalid) && common.VerifyCodeWithKey(input.Email, input.Token, common.PasswordResetPurpose) {
		// Existing links issued before durable flows expire through their
		// original verification store; serializing keeps them single-use.
		LockOrder("password-reset:" + input.Email)
		defer UnlockOrder("password-reset:" + input.Email)
		if common.VerifyCodeWithKey(input.Email, input.Token, common.PasswordResetPurpose) {
			err = model.ResetUserPasswordByEmailContext(c.Request.Context(), input.Email, password)
			if err == nil {
				common.DeleteKey(input.Email, common.PasswordResetPurpose)
			}
		}
	}
	if err != nil {
		if errors.Is(err, model.ErrAuthFlowInvalid) || errors.Is(err, model.ErrAuthFlowConsumed) || errors.Is(err, model.ErrAuthFlowExpired) || errors.Is(err, model.ErrEmailNotFound) || errors.Is(err, model.ErrEmailAmbiguous) {
			common.ApiErrorI18n(c, i18n.MsgUserPasswordResetLinkInvalid)
		} else {
			common.ApiError(c, err)
		}
		return
	}
	common.ApiSuccess(c, password)
}
