package controller

import (
	"encoding/json"
	"fmt"
	"math"
	"net/mail"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/i18n"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

type SelfAccountRequest struct {
	Username         *string `json:"username"`
	DisplayName      *string `json:"display_name"`
	Password         string  `json:"password"`
	OriginalPassword string  `json:"original_password"`
	Language         *string `json:"language"`
	SidebarModules   *string `json:"sidebar_modules"`
}

func UpdateSelf(c *gin.Context) {
	var input SelfAccountRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if input.Password != "" {
		changeSelfPassword(c, input)
		return
	}
	if input.Language != nil || input.SidebarModules != nil {
		if input.SidebarModules != nil && *input.SidebarModules != "" && !json.Valid([]byte(*input.SidebarModules)) {
			common.ApiErrorMsg(c, "导航偏好格式无效")
			return
		}
		if input.Language != nil && (strings.TrimSpace(*input.Language) == "" || len(*input.Language) > 32) {
			common.ApiErrorMsg(c, "语言偏好无效")
			return
		}
		err := model.MutateAccountPreferences(c.Request.Context(), c.GetInt("id"), func(_ *model.User, preferences *dto.UserSetting) error {
			if input.Language != nil {
				preferences.Language = strings.TrimSpace(*input.Language)
			}
			if input.SidebarModules != nil {
				preferences.SidebarModules = *input.SidebarModules
			}
			return nil
		})
		if err != nil {
			common.ApiError(c, err)
			return
		}
		common.ApiSuccessI18n(c, i18n.MsgUpdateSuccess, nil)
		return
	}
	if err := model.UpdateSelfAccountProfile(c.Request.Context(), c.GetInt("id"), model.SelfAccountProfileInput{Username: input.Username, DisplayName: input.DisplayName}); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func changeSelfPassword(c *gin.Context, input SelfAccountRequest) {
	succeeded, notificationFailed := false, false
	defer func() {
		recordUserSecurityAudit(c, c.GetInt("id"), "user.password_change", map[string]any{"success": succeeded, "notification_failed": notificationFailed})
	}()
	if (input.Username != nil && utf8.RuneCountInString(*input.Username) > 20) || (input.DisplayName != nil && utf8.RuneCountInString(*input.DisplayName) > 20) {
		common.ApiErrorI18n(c, i18n.MsgInvalidInput)
		return
	}
	identity, valid := middleware.GetSessionAuthIdentity(c)
	if !valid {
		writeSecurityOperationError(c, service.ErrAuthTokenInvalid)
		return
	}
	current, err := model.GetUserById(identity.UserID, true)
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	first := current.Password == ""
	scope := service.VerificationScopePasswordChange
	if first {
		scope = service.VerificationScopePasswordSet
	}
	if middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: scope}) == nil {
		return
	}
	update := model.User{Id: identity.UserID, Password: input.Password, OriginalPassword: input.OriginalPassword}
	if input.Username != nil {
		update.Username = *input.Username
	}
	if input.DisplayName != nil {
		update.DisplayName = *input.DisplayName
	}
	if err := model.ChangeUserPasswordContext(c.Request.Context(), identity, &update, first); err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	succeeded = true
	notificationFailed = service.NotifyAccountSecurityChange(current.Email, "Password updated") != nil
	if err := model.PublishUserAuthCache(update.Id); err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	bundle, err := service.AdvanceCurrentSessionToUserVersion(identity, "password_changed")
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"access_token": bundle.AccessToken, "token_type": bundle.TokenType, "access_expires_at": bundle.AccessExpiresAt, "session": bundle.Session, "has_password": true, "notification_warning": notificationFailed})
}

type UpdateUserSettingRequest struct {
	QuotaWarningType                 string  `json:"notify_type"`
	QuotaWarningThreshold            float64 `json:"quota_warning_threshold"`
	WebhookUrl                       string  `json:"webhook_url,omitempty"`
	WebhookSecret                    string  `json:"webhook_secret,omitempty"`
	NotificationEmail                string  `json:"notification_email,omitempty"`
	BarkUrl                          string  `json:"bark_url,omitempty"`
	GotifyUrl                        string  `json:"gotify_url,omitempty"`
	GotifyToken                      string  `json:"gotify_token,omitempty"`
	GotifyPriority                   int     `json:"gotify_priority,omitempty"`
	UpstreamModelUpdateNotifyEnabled *bool   `json:"upstream_model_update_notify_enabled,omitempty"`
	AcceptUnsetModelRatioModel       bool    `json:"accept_unset_model_ratio_model"`
	RecordIpLog                      bool    `json:"record_ip_log"`
}

func validNotificationURL(address string) bool {
	parsed, err := url.ParseRequestURI(address)
	return err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func (input UpdateUserSettingRequest) validate() error {
	if input.QuotaWarningThreshold <= 0 || math.IsNaN(input.QuotaWarningThreshold) || math.IsInf(input.QuotaWarningThreshold, 0) {
		return fmt.Errorf("额度预警阈值必须大于 0")
	}
	switch input.QuotaWarningType {
	case dto.NotifyTypeEmail:
		if input.NotificationEmail != "" {
			if _, err := mail.ParseAddress(input.NotificationEmail); err != nil {
				return fmt.Errorf("通知邮箱格式无效")
			}
		}
	case dto.NotifyTypeWebhook:
		if !validNotificationURL(input.WebhookUrl) {
			return fmt.Errorf("请提供有效的 HTTP 或 HTTPS 通知地址")
		}
	case dto.NotifyTypeBark:
		if !validNotificationURL(input.BarkUrl) {
			return fmt.Errorf("请提供有效的 Bark 地址")
		}
	case dto.NotifyTypeGotify:
		if !validNotificationURL(input.GotifyUrl) || input.GotifyToken == "" {
			return fmt.Errorf("请提供有效的 Gotify 地址与访问令牌")
		}
	default:
		return fmt.Errorf("请选择有效的通知方式")
	}
	return nil
}

func UpdateUserSetting(c *gin.Context) {
	var input UpdateUserSettingRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := input.validate(); err != nil {
		common.ApiError(c, err)
		return
	}
	err := model.MutateAccountPreferences(c.Request.Context(), c.GetInt("id"), func(account *model.User, settings *dto.UserSetting) error {
		settings.NotifyType, settings.QuotaWarningThreshold = input.QuotaWarningType, input.QuotaWarningThreshold
		settings.AcceptUnsetRatioModel, settings.RecordIpLog = input.AcceptUnsetModelRatioModel, input.RecordIpLog
		if account.Role >= common.RoleAdminUser && input.UpstreamModelUpdateNotifyEnabled != nil {
			settings.UpstreamModelUpdateNotifyEnabled = *input.UpstreamModelUpdateNotifyEnabled
		}
		switch input.QuotaWarningType {
		case dto.NotifyTypeEmail:
			settings.NotificationEmail = input.NotificationEmail
		case dto.NotifyTypeWebhook:
			settings.WebhookUrl = input.WebhookUrl
			if input.WebhookSecret != "" {
				settings.WebhookSecret = input.WebhookSecret
			}
		case dto.NotifyTypeBark:
			settings.BarkUrl = input.BarkUrl
		case dto.NotifyTypeGotify:
			settings.GotifyUrl, settings.GotifyToken = input.GotifyUrl, input.GotifyToken
			settings.GotifyPriority = input.GotifyPriority
			if settings.GotifyPriority < 0 || settings.GotifyPriority > 10 {
				settings.GotifyPriority = 5
			}
		}
		return nil
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccessI18n(c, i18n.MsgSettingSaved, nil)
}
