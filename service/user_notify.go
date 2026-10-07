package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
)

func NotifyRootUser(kind, title, content string) {
	root := model.GetRootUser()
	if root == nil {
		common.SysError("通知管理员失败：管理员账户尚不可用。")
		return
	}
	if err := NotifyUser(root.Id, root.Email, root.GetSetting(), dto.NewNotify(kind, title, content, nil)); err != nil {
		common.SysError("通知管理员失败：" + err.Error())
	}
}

func NotifyUpstreamModelUpdateWatchers(title, content string) {
	ctx, cancel := context.WithTimeout(context.Background(), notificationRequestTimeout)
	defer cancel()
	if model.DB == nil {
		common.SysError("通知订阅用户失败：账户数据尚不可用。")
		return
	}
	var recipients []model.User
	if err := model.DB.WithContext(ctx).Select("id", "email", "setting").
		Where("status = ? AND role >= ?", common.UserStatusEnabled, common.RoleAdminUser).Find(&recipients).Error; err != nil {
		common.SysError("读取通知订阅用户失败：" + err.Error())
		return
	}
	for _, recipient := range recipients {
		preferences := recipient.GetSetting()
		if !preferences.UpstreamModelUpdateNotifyEnabled {
			continue
		}
		if err := NotifyUser(recipient.Id, recipient.Email, preferences, dto.NewNotify(dto.NotifyTypeChannelUpdate, title, content, nil)); err != nil {
			common.SysError(fmt.Sprintf("发送渠道更新通知失败（用户 %d）：%s", recipient.Id, err))
		}
	}
}

func NotifyUser(userID int, email string, settings dto.UserSetting, data dto.Notify) error {
	return NotifyUserContext(context.Background(), userID, email, settings, data)
}

func NotifyUserContext(ctx context.Context, userID int, email string, settings dto.UserSetting, data dto.Notify) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	method := settings.NotifyType
	if method == "" {
		method = dto.NotifyTypeEmail
	}
	// An unconfigured destination does not consume an admission slot.
	switch method {
	case dto.NotifyTypeEmail:
		if settings.NotificationEmail != "" {
			email = settings.NotificationEmail
		}
		if strings.TrimSpace(email) == "" {
			return nil
		}
	case dto.NotifyTypeWebhook:
		if strings.TrimSpace(settings.WebhookUrl) == "" {
			return nil
		}
	case dto.NotifyTypeBark:
		if strings.TrimSpace(settings.BarkUrl) == "" {
			return nil
		}
	case dto.NotifyTypeGotify:
		if settings.GotifyUrl == "" || settings.GotifyToken == "" {
			return nil
		}
	default:
		return errors.New("通知方式无效。")
	}
	allowed, err := CheckNotificationLimitContext(ctx, userID, data.Type)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("用户 %d 的 %s 通知已达到频率限制。", userID, method)
	}
	switch method {
	case dto.NotifyTypeEmail:
		return sendEmailNotifyContext(ctx, email, data)
	case dto.NotifyTypeWebhook:
		return SendWebhookNotifyContext(ctx, settings.WebhookUrl, settings.WebhookSecret, data)
	case dto.NotifyTypeBark:
		return sendBarkNotifyContext(ctx, settings.BarkUrl, data)
	case dto.NotifyTypeGotify:
		return sendGotifyNotifyContext(ctx, settings.GotifyUrl, settings.GotifyToken, settings.GotifyPriority, data)
	}
	return nil
}

func sendEmailNotify(email string, data dto.Notify) error {
	return sendEmailNotifyContext(context.Background(), email, data)
}
func sendEmailNotifyContext(ctx context.Context, email string, data dto.Notify) error {
	return common.SendEmailContext(ctx, data.Title, email, renderNotificationContent(data))
}

func sendBarkNotify(target string, data dto.Notify) error {
	return sendBarkNotifyContext(context.Background(), target, data)
}
func sendBarkNotifyContext(ctx context.Context, target string, data dto.Notify) error {
	target = strings.ReplaceAll(target, "{{title}}", url.QueryEscape(data.Title))
	target = strings.ReplaceAll(target, "{{content}}", url.QueryEscape(renderNotificationContent(data)))
	headers := make(http.Header)
	headers.Set("User-Agent", "further-api/1.0")
	return sendNotificationRequest(ctx, target, http.MethodGet, headers, nil)
}

func sendGotifyNotify(target, token string, priority int, data dto.Notify) error {
	return sendGotifyNotifyContext(context.Background(), target, token, priority, data)
}
func sendGotifyNotifyContext(ctx context.Context, target, token string, priority int, data dto.Notify) error {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Hostname() == "" {
		return errors.New("Gotify 通知地址无效。")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/message"
	query := parsed.Query()
	query.Set("token", token)
	parsed.RawQuery = query.Encode()
	if priority < 0 || priority > 10 {
		priority = 5
	}
	payload, err := common.Marshal(struct {
		Title    string `json:"title"`
		Message  string `json:"message"`
		Priority int    `json:"priority"`
	}{data.Title, renderNotificationContent(data), priority})
	if err != nil {
		return err
	}
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json; charset=utf-8")
	headers.Set("User-Agent", "further-api/1.0")
	return sendNotificationRequest(ctx, parsed.String(), http.MethodPost, headers, payload)
}
