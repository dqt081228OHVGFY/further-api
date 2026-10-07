package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/stretchr/testify/require"
)

func TestCompleteFurtherWelcomeKeepsPreferencesAndCurrentAccount(t *testing.T) {
	db := setupManageUserTestDB(t)
	preferences := dto.UserSetting{
		NotifyType: dto.NotifyTypeWebhook, QuotaWarningThreshold: 75,
		WebhookUrl: "https://hooks.example/notify", WebhookSecret: "existing-webhook-secret",
		NotificationEmail: "notifications@example.com", BarkUrl: "https://bark.example/notify",
		GotifyUrl: "https://gotify.example", GotifyToken: "existing-gotify-token", GotifyPriority: 7,
		UpstreamModelUpdateNotifyEnabled: true, AcceptUnsetRatioModel: true, RecordIpLog: true,
		SidebarModules: `{"workspace":true}`, BillingPreference: "wallet_only", Language: "zh-CN",
	}
	owner := model.User{Username: "welcome-owner", AffCode: "welcome-owner", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 100}
	owner.SetSetting(preferences)
	other := model.User{Username: "welcome-other", AffCode: "welcome-other", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&other).Error)
	var previousSetting string
	for attempt := 0; attempt < 2; attempt++ {
		ctx, response := newAuthenticatedContext(t, http.MethodPost, "/api/further/account/welcome?user_id=9999", map[string]any{"user_id": other.Id, "welcome_completed": false, "language": "en", "quota": 999999}, owner.Id)
		CompleteFurtherWelcome(ctx)
		require.Equal(t, http.StatusOK, response.Code)
		result := decodeAPIResponse(t, response)
		require.True(t, result.Success, result.Message)
		require.JSONEq(t, `{"welcome_completed":true}`, string(result.Data))
		require.NotContains(t, response.Body.String(), "existing-")
		var stored model.User
		require.NoError(t, db.First(&stored, owner.Id).Error)
		expected := preferences
		expected.WelcomeCompleted = true
		require.Equal(t, expected, stored.GetSetting(), "only the welcome completion flag may change")
		require.Equal(t, 100, stored.Quota)
		require.Equal(t, common.RoleCommonUser, stored.Role)
		if attempt > 0 {
			require.Equal(t, previousSetting, stored.Setting, "repeating completion is idempotent")
		}
		previousSetting = stored.Setting
	}
	require.NoError(t, db.First(&other, other.Id).Error)
	require.False(t, other.GetSetting().WelcomeCompleted)

	ctx, response := newAuthenticatedContext(t, http.MethodGet, "/api/user/self", nil, owner.Id)
	ctx.Set("role", common.RoleCommonUser)
	GetSelf(ctx)
	var result struct {
		Success bool                     `json:"success"`
		Data    service.WorkspaceAccount `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.True(t, result.Success)
	require.True(t, result.Data.WelcomeCompleted, "a fresh profile read on another device receives completion")

	ctx, response = newAuthenticatedContext(t, http.MethodPut, "/api/user/self", map[string]any{"language": "en"}, owner.Id)
	UpdateSelf(ctx)
	require.True(t, decodeAPIResponse(t, response).Success)
	require.NoError(t, db.First(&owner, owner.Id).Error)
	require.True(t, owner.GetSetting().WelcomeCompleted, "other preference edits cannot reset completion")
	ctx, response = newAuthenticatedContext(t, http.MethodPut, "/api/user/setting", map[string]any{"notify_type": dto.NotifyTypeEmail, "quota_warning_threshold": 80, "record_ip_log": false, "welcome_completed": false}, owner.Id)
	UpdateUserSetting(ctx)
	require.True(t, decodeAPIResponse(t, response).Success)
	require.NoError(t, db.First(&owner, owner.Id).Error)
	require.True(t, owner.GetSetting().WelcomeCompleted, "notification updates cannot reset completion")
	require.Equal(t, "en", owner.GetSetting().Language)
	require.Equal(t, "wallet_only", owner.GetSetting().BillingPreference)
	require.NoError(t, model.UpdateUserSetting(owner.Id, dto.UserSetting{Language: "fr", BillingPreference: "wallet_only"}))
	require.NoError(t, db.First(&owner, owner.Id).Error)
	require.True(t, owner.GetSetting().WelcomeCompleted, "full preference replacement cannot reset completion to its zero value")
	require.Equal(t, "fr", owner.GetSetting().Language)
}

func TestCompleteFurtherWelcomeRejectsMissingIdentityAndCancelledWrite(t *testing.T) {
	db := setupManageUserTestDB(t)
	user := model.User{Username: "welcome-cancelled", AffCode: "welcome-cancelled", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	ctx, response := newAuthenticatedContext(t, http.MethodPost, "/api/further/account/welcome", map[string]any{"user_id": user.Id}, 0)
	CompleteFurtherWelcome(ctx)
	require.Equal(t, http.StatusUnauthorized, response.Code)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	ctx, response = newAuthenticatedContext(t, http.MethodPost, "/api/further/account/welcome", nil, user.Id)
	ctx.Request = ctx.Request.WithContext(cancelled)
	CompleteFurtherWelcome(ctx)
	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.False(t, decodeAPIResponse(t, response).Success)
	require.NoError(t, db.First(&user, user.Id).Error)
	require.False(t, user.GetSetting().WelcomeCompleted)
}
