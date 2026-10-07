package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/setting/console_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestWorkspaceBootstrapReadsOnlyCurrentUserAndHonorsCancellation(t *testing.T) {
	previousDB, previousSetup := model.DB, constant.Setup
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB, constant.Setup = db, true
	require.NoError(t, db.AutoMigrate(&model.User{}))
	t.Cleanup(func() {
		model.DB, constant.Setup = previousDB, previousSetup
		connection, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, connection.Close())
	})
	pat := "private-account-token"
	user := model.User{Username: "workspace-user", AffCode: "workspace-user", Password: "private-password-hash", Remark: "private-admin-remark", AccessToken: &pat,
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", DisplayName: "工作台用户"}
	require.NoError(t, db.Create(&user).Error)
	queries := 0
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("workspace:count", func(tx *gorm.DB) { queries++ }))
	workspace, err := ReadWorkspaceBootstrap(context.Background(), user.Id, common.RoleCommonUser, "https://gateway.example/oauth/linuxdo")
	require.NoError(t, err)
	require.Equal(t, 1, queries, "bootstrap should read the account once and reuse its group")
	require.True(t, workspace.Authenticated)
	require.Equal(t, user.Id, workspace.User.ID)
	require.True(t, workspace.User.HasPassword)
	require.Equal(t, "工作台用户", workspace.User.DisplayName)
	encoded, err := json.Marshal(workspace)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private-")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ReadWorkspaceBootstrap(ctx, user.Id, common.RoleCommonUser, "")
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, queries, "cancelled work must not reach the database")
	require.NoError(t, db.Model(&user).Update("status", common.UserStatusDisabled).Error)
	_, err = ReadWorkspaceBootstrap(context.Background(), user.Id, common.RoleCommonUser, "")
	require.ErrorIs(t, err, ErrWorkspaceAccountUnavailable)
}

func TestWorkspacePublicStatusUsesSafeSnapshots(t *testing.T) {
	workspaceTestOptions(t, map[string]string{"SMTPToken": "private-smtp", "discord.client_secret": "private-discord", "Chats": "retired-preset"})
	console := console_setting.GetConsoleSetting()
	previousConsole := *console
	console.AnnouncementsEnabled = true
	console.Announcements = `[{"content":"older","publishDate":"2026-01-01T00:00:00Z"},{"content":"newer","publishDate":"2026-10-01T00:00:00Z"}]`
	t.Cleanup(func() { *console = previousConsole })
	status := ReadWorkspaceStatus("https://gateway.example/oauth/linuxdo")
	require.Equal(t, common.ProductName, status["system_name"])
	require.NotContains(t, status, "chats")
	encoded, err := json.Marshal(status)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private-")
	require.NotContains(t, string(encoded), "retired-preset")
	announcements := status["announcements"].([]map[string]any)
	require.Equal(t, "newer", announcements[0]["content"])
	announcements[0]["content"] = "caller-edit"
	again := ReadWorkspaceStatus("")["announcements"].([]map[string]any)
	require.Equal(t, "newer", again[0]["content"])
}

func TestWorkspaceAccountWelcomeStateIsExplicitAndRefreshable(t *testing.T) {
	user := model.User{Id: 1, Username: "welcome-profile", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	initial := WorkspaceAccountFromUser(&user, common.RoleCommonUser)
	require.False(t, initial.WelcomeCompleted)
	encoded, err := json.Marshal(initial)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"welcome_completed":false`)
	user.SetSetting(dto.UserSetting{Language: "zh-CN", SidebarModules: `{"workspace":true}`, WelcomeCompleted: true})
	completed := WorkspaceAccountFromUser(&user, common.RoleCommonUser)
	require.True(t, completed.WelcomeCompleted)
	require.Equal(t, `{"workspace":true}`, completed.SidebarModules)
	encoded, err = json.Marshal(completed)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"welcome_completed":true`)
}
