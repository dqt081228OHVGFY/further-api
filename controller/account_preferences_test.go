package controller

import (
	"net/http"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestNotificationSettingsKeepBillingAndLanguagePreferences(t *testing.T) {
	db := setupManageUserTestDB(t)
	user := model.User{Username: "notification-preferences", Role: common.RoleCommonUser, AffCode: "notification-preferences"}
	user.SetSetting(dto.UserSetting{Language: "zh-CN", BillingPreference: "wallet_only", SidebarModules: `{"workspace":true}`, WebhookSecret: "existing-secret"})
	require.NoError(t, db.Create(&user).Error)
	context, response := newAuthenticatedContext(t, http.MethodPut, "/api/user/setting", map[string]any{"notify_type": dto.NotifyTypeEmail, "quota_warning_threshold": 100, "record_ip_log": true}, user.Id)
	UpdateUserSetting(context)
	result := decodeAPIResponse(t, response)
	require.True(t, result.Success, result.Message)
	var stored model.User
	require.NoError(t, db.First(&stored, user.Id).Error)
	settings := stored.GetSetting()
	require.Equal(t, "zh-CN", settings.Language)
	require.Equal(t, "wallet_only", settings.BillingPreference)
	require.Equal(t, `{"workspace":true}`, settings.SidebarModules)
	require.Equal(t, "existing-secret", settings.WebhookSecret)
	require.True(t, settings.RecordIpLog)
}

func TestSelfLanguagePatchKeepsNotificationAndBilling(t *testing.T) {
	db := setupManageUserTestDB(t)
	user := model.User{Username: "language-preferences", Role: common.RoleCommonUser, AffCode: "language-preferences"}
	user.SetSetting(dto.UserSetting{NotifyType: dto.NotifyTypeEmail, QuotaWarningThreshold: 50, BillingPreference: "subscription_only"})
	require.NoError(t, db.Create(&user).Error)
	context, response := newAuthenticatedContext(t, http.MethodPut, "/api/user/self", map[string]any{"language": "zh-CN", "role": common.RoleRootUser, "quota": 999999}, user.Id)
	UpdateSelf(context)
	result := decodeAPIResponse(t, response)
	require.True(t, result.Success, result.Message)
	var stored model.User
	require.NoError(t, db.First(&stored, user.Id).Error)
	require.Equal(t, common.RoleCommonUser, stored.Role)
	require.Zero(t, stored.Quota)
	require.Equal(t, "zh-CN", stored.GetSetting().Language)
	require.Equal(t, "subscription_only", stored.GetSetting().BillingPreference)
	require.EqualValues(t, 50, stored.GetSetting().QuotaWarningThreshold)
}

func TestChannelBatchDraftDoesNotReuseNamesOrAcceptCounters(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	previousMemory := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemory })
	context, response := newAuthenticatedContext(t, http.MethodPost, "/api/channel/", map[string]any{
		"mode": "batch", "batch_add_set_key_prefix_2_name": true,
		"channel": map[string]any{"id": 999, "name": "Batch", "type": 1, "key": "aaaaaaaa-key\nbbbbbbbb-key", "group": "default", "models": "test-model", "used_quota": 999999, "balance": 999999},
	}, 9999)
	context.Set("role", common.RoleRootUser)
	AddChannel(context)
	result := decodeAPIResponse(t, response)
	require.True(t, result.Success, result.Message)
	var channels []model.Channel
	require.NoError(t, db.Order("id").Find(&channels).Error)
	require.Len(t, channels, 2)
	require.Equal(t, "Batch aaaaaaaa", channels[0].Name)
	require.Equal(t, "Batch bbbbbbbb", channels[1].Name)
	for _, channel := range channels {
		require.NotEqual(t, 999, channel.Id)
		require.Zero(t, channel.UsedQuota)
		require.Zero(t, channel.Balance)
	}
}
