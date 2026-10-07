package business

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
)

type GatewayOptionInputError struct {
	Status        int
	Code, Message string
}

func (e *GatewayOptionInputError) Error() string { return e.Message }

func GatewayOptionValue(value any) (string, error) {
	switch value := value.(type) {
	case string:
		return value, nil
	case bool:
		return strconv.FormatBool(value), nil
	case json.Number:
		return value.String(), nil
	case nil:
		return "", fmt.Errorf("设置值不能为空值")
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("设置值无法编码")
		}
		return string(raw), nil
	}
}

func ValidateGatewayOptionCommand(key, value string) error {
	if key == "Chats" {
		return &GatewayOptionInputError{Status: http.StatusGone, Code: "FEATURE_REMOVED", Message: "聊天预设已移除"}
	}
	if err := common.ValidateBrandingOption(key, value); err != nil {
		return &GatewayOptionInputError{Status: http.StatusBadRequest, Code: "BRANDING_READ_ONLY", Message: err.Error()}
	}
	if strings.HasPrefix(key, "payment_setting.compliance_") {
		return fmt.Errorf("付款渠道确认须通过专用确认入口提交")
	}
	if (key == "QuotaForInviter" || key == "QuotaForInvitee") && !operation_setting.IsPaymentComplianceConfirmed() {
		amount, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err == nil && amount > 0 {
			return &GatewayOptionInputError{Code: "PAYMENT_COMPLIANCE_REQUIRED", Status: http.StatusOK, Message: "请先完成付款渠道使用确认，再启用邀请额度"}
		}
	}
	if key == "TaskPublicAddress" && value != "" {
		if err := service.ValidateTaskArtifactBaseURL(value); err != nil {
			return err
		}
	}
	if value == "true" {
		common.OptionMapRWMutex.RLock()
		githubReady := strings.TrimSpace(common.GitHubClientId) != "" && strings.TrimSpace(common.GitHubClientSecret) != ""
		linuxDOReady := strings.TrimSpace(common.LinuxDOClientId) != "" && strings.TrimSpace(common.LinuxDOClientSecret) != ""
		emailReady := len(common.EmailDomainWhitelist) > 0
		wechatReady := strings.TrimSpace(common.WeChatServerAddress) != ""
		turnstileReady := strings.TrimSpace(common.TurnstileSiteKey) != "" && strings.TrimSpace(common.TurnstileSecretKey) != ""
		common.OptionMapRWMutex.RUnlock()
		dependencies := map[string]struct {
			ready   bool
			message string
		}{
			"GitHubOAuthEnabled":            {githubReady, "请先配置 GitHub 客户端编号和密钥"},
			"LinuxDOOAuthEnabled":           {linuxDOReady, "请先配置 Linux DO 客户端编号和密钥"},
			"discord.enabled":               {strings.TrimSpace(system_setting.GetDiscordSettings().ClientId) != "" && strings.TrimSpace(system_setting.GetDiscordSettings().ClientSecret) != "", "请先配置 Discord 客户端编号和密钥"},
			"oidc.enabled":                  {strings.TrimSpace(system_setting.GetOIDCSettings().ClientId) != "", "请先配置 OIDC 客户端编号"},
			"EmailDomainRestrictionEnabled": {emailReady, "请先填写允许注册的邮箱域名"},
			"WeChatAuthEnabled":             {wechatReady, "请先配置微信登录服务地址"},
			"TurnstileCheckEnabled":         {turnstileReady, "请先配置 Turnstile 站点编号和密钥"},
			"TelegramOAuthEnabled":          {system_setting.GetTelegramSettings().IsConfigured(), "请先配置 Telegram 登录机器人"},
		}
		if dependency, ok := dependencies[key]; ok && !dependency.ready {
			code := "OPTION_DEPENDENCY_REQUIRED"
			if key == "TelegramOAuthEnabled" {
				code = "TELEGRAM_OAUTH_NOT_CONFIGURED"
			}
			return &GatewayOptionInputError{Status: http.StatusOK, Code: code, Message: dependency.message}
		}
	}
	return model.ValidateGatewayOptionValues(map[string]string{key: value})
}
