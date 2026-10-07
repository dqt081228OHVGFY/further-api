package service

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func siteSettingValue[T any](value T) *T { return &value }

func siteSettingsDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	connection, err := database.DB()
	require.NoError(t, err)
	connection.SetMaxOpenConns(1)
	previousDB := model.DB
	previousMap := maps.Clone(common.OptionMap)
	previousName, previousLogo, previousServer := common.SystemName, common.Logo, system_setting.ServerAddress
	previousID, previousSecret, previousEnabled, previousTrust := common.LinuxDOClientId, common.LinuxDOClientSecret, common.LinuxDOOAuthEnabled, common.LinuxDOMinimumTrustLevel
	model.DB = database
	common.OptionMap = map[string]string{"SystemName": common.ProductName, "Logo": common.ProductLogo, "ServerAddress": "https://old.example", "LinuxDOClientId": "previous-client", "LinuxDOClientSecret": "retained-test-secret", "LinuxDOOAuthEnabled": "false", "LinuxDOMinimumTrustLevel": "1"}
	common.SystemName, common.Logo, system_setting.ServerAddress = common.ProductName, common.ProductLogo, "https://old.example"
	common.LinuxDOClientId, common.LinuxDOClientSecret, common.LinuxDOOAuthEnabled, common.LinuxDOMinimumTrustLevel = "previous-client", "retained-test-secret", false, 1
	t.Cleanup(func() {
		model.DB, common.OptionMap = previousDB, previousMap
		common.SystemName, common.Logo, system_setting.ServerAddress = previousName, previousLogo, previousServer
		common.LinuxDOClientId, common.LinuxDOClientSecret, common.LinuxDOOAuthEnabled, common.LinuxDOMinimumTrustLevel = previousID, previousSecret, previousEnabled, previousTrust
		require.NoError(t, connection.Close())
	})
	require.NoError(t, database.AutoMigrate(&model.Option{}))
	for key, value := range common.OptionMap {
		require.NoError(t, database.Create(&model.Option{Key: key, Value: value}).Error)
	}
	return database
}

func TestWorkspaceSiteSavePreservesBlankSecretAndDoesNotExposeCredential(t *testing.T) {
	database := siteSettingsDatabase(t)
	require.NoError(t, SaveWorkspaceSiteSettings(context.Background(), WorkspaceSiteChange{
		ServerAddress: siteSettingValue("https://next.example/"), LinuxDOEnabled: siteSettingValue(true),
		LinuxDOClientID: siteSettingValue("new-client"), LinuxDOClientSecret: siteSettingValue(""), LinuxDOMinimumTrust: siteSettingValue(3),
	}))
	settings, err := ReadWorkspaceSiteSettings(context.Background(), "https://next.example/api/oauth/linuxdo")
	require.NoError(t, err)
	require.True(t, settings.LinuxDOEnabled)
	require.True(t, settings.LinuxDOSecretConfigured)
	require.Equal(t, "https://next.example", settings.ServerAddress)
	require.Equal(t, "new-client", settings.LinuxDOClientID)
	encoded, err := json.Marshal(settings)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "retained-test-secret")
	var secret model.Option
	require.NoError(t, database.Where("key = ?", "LinuxDOClientSecret").First(&secret).Error)
	require.Equal(t, "retained-test-secret", secret.Value)
}

func TestWorkspaceSiteValidatesWholeChangeBeforeWriting(t *testing.T) {
	database := siteSettingsDatabase(t)
	for _, change := range []WorkspaceSiteChange{
		{ServerAddress: siteSettingValue("https://user:password@example.com/path"), LinuxDOClientID: siteSettingValue("should-not-be-written")},
		{SystemName: siteSettingValue("another gateway"), ServerAddress: siteSettingValue("https://next.example")},
		{LinuxDOMinimumTrust: siteSettingValue(5), LinuxDOEnabled: siteSettingValue(true)},
	} {
		var input *WorkspaceSiteInputError
		require.ErrorAs(t, SaveWorkspaceSiteSettings(context.Background(), change), &input)
	}
	var id model.Option
	require.NoError(t, database.Where("key = ?", "LinuxDOClientId").First(&id).Error)
	require.Equal(t, "previous-client", id.Value)
	require.Equal(t, "https://old.example", system_setting.ServerAddress)
	require.False(t, common.LinuxDOOAuthEnabled)
}

func TestWorkspaceSiteRollsBackAllFieldsWhenStorageFails(t *testing.T) {
	database := siteSettingsDatabase(t)
	require.NoError(t, database.Callback().Update().Before("gorm:update").Register("site-setting-write-failure", func(tx *gorm.DB) {
		if option, ok := tx.Statement.Dest.(*model.Option); ok && option.Key == "LinuxDOClientSecret" {
			tx.AddError(errors.New("test write failure"))
		}
	}))
	require.Error(t, SaveWorkspaceSiteSettings(context.Background(), WorkspaceSiteChange{ServerAddress: siteSettingValue("https://next.example"), LinuxDOClientID: siteSettingValue("new-client"), LinuxDOClientSecret: siteSettingValue("new-secret"), LinuxDOEnabled: siteSettingValue(true)}))
	var options []model.Option
	require.NoError(t, database.Find(&options).Error)
	for _, option := range options {
		require.Equal(t, common.OptionMap[option.Key], option.Value)
	}
	require.Equal(t, "https://old.example", system_setting.ServerAddress)
	require.Equal(t, "retained-test-secret", common.LinuxDOClientSecret)
	require.False(t, common.LinuxDOOAuthEnabled)
}

func TestWorkspaceSiteRejectsCanceledWrites(t *testing.T) {
	siteSettingsDatabase(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, SaveWorkspaceSiteSettings(ctx, WorkspaceSiteChange{ServerAddress: siteSettingValue("https://next.example")}), context.Canceled)
	require.Equal(t, "https://old.example", system_setting.ServerAddress)
}
