package service

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/oauth"
	"github.com/dqt081228OHVGFY/further-api/service/authz"
	"github.com/dqt081228OHVGFY/further-api/setting"
	"github.com/dqt081228OHVGFY/further-api/setting/console_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/ratio_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
)

var ErrWorkspaceAccountUnavailable = errors.New("workspace account is not available")

type WorkspaceSetup struct {
	Status       bool   `json:"status"`
	RootInit     bool   `json:"root_init"`
	DatabaseType string `json:"database_type"`
}

type WorkspaceGroup struct {
	Ratio any    `json:"ratio"`
	Desc  string `json:"desc"`
}

type WorkspacePermissions struct {
	SidebarSettings  bool                 `json:"sidebar_settings"`
	SidebarModules   map[string]any       `json:"sidebar_modules"`
	AdminPermissions authz.PermissionsMap `json:"admin_permissions"`
}

// WorkspaceAccount exposes only the signed-in user's profile. It never embeds
// model.User, so later database fields cannot silently become public JSON.
type WorkspaceAccount struct {
	ID               int                  `json:"id"`
	Username         string               `json:"username"`
	DisplayName      string               `json:"display_name"`
	HasPassword      bool                 `json:"has_password"`
	WelcomeCompleted bool                 `json:"welcome_completed"`
	Role             int                  `json:"role"`
	Status           int                  `json:"status"`
	Email            string               `json:"email"`
	GitHubID         string               `json:"github_id"`
	DiscordID        string               `json:"discord_id"`
	OIDCID           string               `json:"oidc_id"`
	WeChatID         string               `json:"wechat_id"`
	TelegramID       string               `json:"telegram_id"`
	LinuxDOID        string               `json:"linux_do_id"`
	Group            string               `json:"group"`
	Quota            int                  `json:"quota"`
	UsedQuota        int                  `json:"used_quota"`
	RequestCount     int                  `json:"request_count"`
	AffCode          string               `json:"aff_code"`
	AffCount         int                  `json:"aff_count"`
	AffQuota         int                  `json:"aff_quota"`
	AffHistoryQuota  int                  `json:"aff_history_quota"`
	InviterID        int                  `json:"inviter_id"`
	Setting          string               `json:"setting"`
	StripeCustomer   string               `json:"stripe_customer"`
	SidebarModules   string               `json:"sidebar_modules"`
	Permissions      WorkspacePermissions `json:"permissions"`
}

type WorkspaceBootstrap struct {
	SchemaVersion int                       `json:"schema_version"`
	Setup         WorkspaceSetup            `json:"setup"`
	Status        map[string]any            `json:"status"`
	Authenticated bool                      `json:"authenticated"`
	User          *WorkspaceAccount         `json:"user"`
	Groups        map[string]WorkspaceGroup `json:"groups"`
}

func ReadWorkspaceSetup() WorkspaceSetup {
	state := WorkspaceSetup{Status: constant.Setup}
	if !state.Status {
		state.RootInit = model.RootUserExists()
		state.DatabaseType = string(common.MainDatabaseType())
	}
	return state
}

func WorkspaceAccountPermissions(userID, role int) WorkspacePermissions {
	result := WorkspacePermissions{
		SidebarSettings:  role != common.RoleRootUser,
		SidebarModules:   make(map[string]any),
		AdminPermissions: authz.Capabilities(userID, role),
	}
	switch role {
	case common.RoleRootUser:
	case common.RoleAdminUser:
		result.SidebarModules["admin"] = map[string]any{"setting": false}
	default:
		result.SidebarModules["admin"] = false
	}
	return result
}

func WorkspaceAccountFromUser(user *model.User, authenticatedRole int) WorkspaceAccount {
	role := min(user.Role, authenticatedRole)
	preferences := user.GetSetting()
	return WorkspaceAccount{
		ID: user.Id, Username: user.Username, DisplayName: user.DisplayName,
		HasPassword: user.HasPassword, WelcomeCompleted: preferences.WelcomeCompleted,
		Role: role, Status: user.Status, Email: user.Email,
		GitHubID: user.GitHubId, DiscordID: user.DiscordId, OIDCID: user.OidcId,
		WeChatID: user.WeChatId, TelegramID: user.TelegramId, LinuxDOID: user.LinuxDOId,
		Group: user.Group, Quota: user.Quota, UsedQuota: user.UsedQuota, RequestCount: user.RequestCount,
		AffCode: user.AffCode, AffCount: user.AffCount, AffQuota: user.AffQuota,
		AffHistoryQuota: user.AffHistoryQuota, InviterID: user.InviterId,
		Setting: user.Setting, StripeCustomer: user.StripeCustomer,
		SidebarModules: preferences.SidebarModules,
		Permissions:    WorkspaceAccountPermissions(user.Id, role),
	}
}

// ReadWorkspaceGroups reuses the already verified profile group; it performs no
// second account query and cannot choose a target user from query parameters.
func ReadWorkspaceGroups(userGroup string) map[string]WorkspaceGroup {
	usable := GetUserUsableGroups(userGroup)
	result := make(map[string]WorkspaceGroup)
	for name := range ratio_setting.GetGroupRatioCopy() {
		if description, ok := usable[name]; ok {
			result[name] = WorkspaceGroup{Ratio: GetUserGroupRatio(userGroup, name), Desc: description}
		}
	}
	if _, ok := usable["auto"]; ok {
		result["auto"] = WorkspaceGroup{Ratio: "自动", Desc: setting.GetUsableGroupDescription("auto")}
	}
	return result
}

// ReadWorkspaceBootstrap is the further-api dashboard read boundary. Public
// callers receive no account data. Authentication and the subject ID are taken
// from middleware, and a failed authenticated read fails the whole request.
func ReadWorkspaceBootstrap(ctx context.Context, userID, authenticatedRole int, linuxDORedirectURI string) (*WorkspaceBootstrap, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := &WorkspaceBootstrap{
		SchemaVersion: 1, Setup: ReadWorkspaceSetup(),
		Status: ReadWorkspaceStatus(linuxDORedirectURI),
	}
	if userID <= 0 {
		result.Groups = ReadWorkspaceGroups("")
		return result, nil
	}
	user, err := model.GetSelfUserByIdContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Status != common.UserStatusEnabled || authenticatedRole < common.RoleCommonUser || !common.IsValidateRole(user.Role) {
		return nil, ErrWorkspaceAccountUnavailable
	}
	account := WorkspaceAccountFromUser(user, authenticatedRole)
	result.Authenticated, result.User = true, &account
	result.Groups = ReadWorkspaceGroups(user.Group)
	return result, nil
}

// ReadWorkspaceStatus uses an explicit public allowlist. Option map contents,
// OAuth secrets, provider configuration objects and administrator credentials
// are never serialized. Locks cover snapshots, not response encoding or DB IO.
func ReadWorkspaceStatus(linuxDORedirectURI string) map[string]any {
	passkey := system_setting.PasskeySettingsSnapshot()
	telegramConfigured := oauth.TelegramConfigurationError() == nil
	common.OptionMapRWMutex.RLock()
	console := *console_setting.GetConsoleSetting()
	legal := *system_setting.GetLegalSettings()
	discord := *system_setting.GetDiscordSettings()
	oidc := *system_setting.GetOIDCSettings()
	general := *operation_setting.GetGeneralSetting()
	data := map[string]any{
		"version": common.Version, "start_time": common.StartTime,
		"system_name": common.ProductName, "logo": common.ProductLogo, "theme": "default",
		"footer_html": common.Footer, "server_address": system_setting.ServerAddress,
		"email_verification": common.EmailVerificationEnabled,
		"github_oauth":       common.GitHubOAuthEnabled, "github_client_id": common.GitHubClientId,
		"discord_oauth": discord.Enabled, "discord_client_id": discord.ClientId,
		"linuxdo_oauth": common.LinuxDOOAuthEnabled, "linuxdo_client_id": common.LinuxDOClientId,
		"linuxdo_redirect_url": linuxDORedirectURI, "linuxdo_minimum_trust_level": common.LinuxDOMinimumTrustLevel,
		"telegram_oauth": common.TelegramOAuthEnabled, "telegram_oauth_configured": telegramConfigured,
		"telegram_bot_name": common.TelegramBotName,
		"wechat_qrcode":     common.WeChatAccountQRCodeImageURL, "wechat_login": common.WeChatAuthEnabled,
		"turnstile_check": common.TurnstileCheckEnabled, "turnstile_site_key": common.TurnstileSiteKey,
		"docs_link": general.DocsLink, "quota_per_unit": common.QuotaPerUnit,
		"display_in_currency":           operation_setting.IsCurrencyDisplay(),
		"quota_display_type":            operation_setting.GetQuotaDisplayType(),
		"custom_currency_symbol":        general.CustomCurrencySymbol,
		"custom_currency_exchange_rate": general.CustomCurrencyExchangeRate,
		"enable_batch_update":           common.BatchUpdateEnabled, "enable_drawing": common.DrawingEnabled,
		"enable_task": common.TaskEnabled, "enable_data_export": common.DataExportEnabled,
		"data_export_default_time": common.DataExportDefaultTime,
		"default_collapse_sidebar": common.DefaultCollapseSidebar, "mj_notify_enabled": setting.MjNotifyEnabled,
		"demo_site_enabled": operation_setting.DemoSiteEnabled, "self_use_mode_enabled": operation_setting.SelfUseModeEnabled,
		"register_enabled": common.RegisterEnabled, "password_login_enabled": common.PasswordLoginEnabled,
		"password_register_enabled": common.PasswordRegisterEnabled, "default_use_auto_group": setting.DefaultUseAutoGroup,
		"password_login_encryption_enabled": common.PasswordLoginEncryptionEnabled,
		"usd_exchange_rate":                 operation_setting.USDExchangeRate, "price": operation_setting.Price,
		"stripe_unit_price": setting.StripeUnitPrice,
		"api_info_enabled":  console.ApiInfoEnabled, "uptime_kuma_enabled": console.UptimeKumaEnabled,
		"announcements_enabled": console.AnnouncementsEnabled, "faq_enabled": console.FAQEnabled,
		"HeaderNavModules":    common.OptionMap["HeaderNavModules"],
		"SidebarModulesAdmin": common.OptionMap["SidebarModulesAdmin"],
		"oidc_enabled":        oidc.Enabled, "oidc_client_id": oidc.ClientId,
		"oidc_authorization_endpoint": oidc.AuthorizationEndpoint, "oidc_display_name": oidc.GetEffectiveDisplayName(),
		"passkey_login": passkey.Enabled, "passkey_display_name": passkey.RPDisplayName,
		"passkey_rp_id": passkey.EffectiveRPID(), "passkey_rp_ids": passkey.RelyingPartyIDs(),
		"passkey_allow_insecure": passkey.AllowInsecureOrigin, "passkey_user_verification": passkey.UserVerification,
		"passkey_attachment": passkey.AttachmentPreference, "setup": constant.Setup,
		"user_agreement_enabled": legal.UserAgreement != "", "privacy_policy_enabled": legal.PrivacyPolicy != "",
		"checkin_enabled": operation_setting.GetCheckinSetting().Enabled,
	}
	common.OptionMapRWMutex.RUnlock()
	if console.ApiInfoEnabled {
		data["api_info"] = decodeWorkspaceConsoleList(console.ApiInfo)
	}
	if console.AnnouncementsEnabled {
		announcements := decodeWorkspaceConsoleList(console.Announcements)
		sort.SliceStable(announcements, func(i, j int) bool {
			return workspaceAnnouncementTime(announcements[i]).After(workspaceAnnouncementTime(announcements[j]))
		})
		data["announcements"] = announcements
	}
	if console.FAQEnabled {
		data["faq"] = decodeWorkspaceConsoleList(console.FAQ)
	}
	providers := oauth.GetEnabledCustomProviders()
	if len(providers) > 0 {
		type publicProvider struct {
			ID                    int    `json:"id"`
			Name                  string `json:"name"`
			Slug                  string `json:"slug"`
			Icon                  string `json:"icon"`
			ClientID              string `json:"client_id"`
			AuthorizationEndpoint string `json:"authorization_endpoint"`
			Scopes                string `json:"scopes"`
		}
		public := make([]publicProvider, 0, len(providers))
		for _, provider := range providers {
			config := provider.GetConfig()
			public = append(public, publicProvider{config.Id, config.Name, config.Slug, config.Icon, config.ClientId, config.AuthorizationEndpoint, config.Scopes})
		}
		sort.Slice(public, func(i, j int) bool { return public[i].Slug < public[j].Slug })
		data["custom_oauth_providers"] = public
	}
	return data
}

func workspaceAnnouncementTime(announcement map[string]any) time.Time {
	raw, _ := announcement["publishDate"].(string)
	parsed, _ := time.Parse(time.RFC3339, raw)
	return parsed
}

func decodeWorkspaceConsoleList(raw string) []map[string]any {
	result := make([]map[string]any, 0)
	if common.UnmarshalJsonStr(raw, &result) != nil || result == nil {
		return []map[string]any{}
	}
	return result
}
