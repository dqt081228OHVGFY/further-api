package business

import (
	"context"
	"errors"
	"fmt"
	"html"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
)

var ErrRegistrationEmailDelivery = errors.New("验证码邮件发送失败，请稍后重试")

func SendGatewayRegistrationEmailContext(ctx context.Context, input string) error {
	email, err := service.ValidateAccountEmail(input)
	if err != nil {
		return err
	}
	challenge, err := model.CreateRegistrationEmailChallengeContext(ctx, email)
	if err != nil {
		return err
	}
	name := html.EscapeString(common.ProductName)
	subject := common.ProductName + " 邮箱验证"
	content := fmt.Sprintf("<p>您好，您正在验证 %s 注册邮箱。</p><p>验证码：<strong>%s</strong></p><p>验证码在 %d 分钟内有效。若并非本人操作，请忽略此邮件。</p>", name, challenge.Code, common.VerificationValidMinutes)
	if err := common.SendEmailContext(ctx, subject, email, content); err != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if revokeErr := model.RevokeRegistrationEmailChallengeContext(cleanup, challenge.ID); revokeErr != nil {
			logger.LogError(ctx, "注册邮件挑战撤销失败")
		}
		logger.LogWarn(ctx, "注册邮件发送未完成")
		return ErrRegistrationEmailDelivery
	}
	return nil
}

func ReadGatewayPublicContent(kind string) (string, error) {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	switch kind {
	case "Notice", "About", "HomePageContent", "Midjourney":
		return common.OptionMap[kind], nil
	case "UserAgreement":
		return system_setting.GetLegalSettings().UserAgreement, nil
	case "PrivacyPolicy":
		return system_setting.GetLegalSettings().PrivacyPolicy, nil
	default:
		return "", errors.New("公开内容类型无效")
	}
}
