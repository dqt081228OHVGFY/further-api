package model

import (
	"context"
	"errors"
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestPublicRegistrationTokenFailureRestoresCodeAndAccount(t *testing.T) {
	setupFurtherBenefitsDB(t)
	require.NoError(t, DB.AutoMigrate(&Token{}))
	require.NoError(t, SaveFurtherRegistrationSettings(FurtherRegistrationSettings{LocalRegistrationEnabled: true, RegistrationCodeRequired: true, PublicInitialQuota: 100}, ""))
	codes, err := GenerateFurtherRegistrationCodes("atomic signup", "default", 1, time.Now())
	require.NoError(t, err)
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register("test:default-token-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" {
			tx.AddError(errors.New("default token unavailable"))
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Create().Remove("test:default-token-failure") })
	_, err = CreateRegisteredAccountContext(context.Background(), PublicRegistrationInput{Username: "atomic-public", Password: "password-for-atomic-signup", RegistrationCode: codes[0].Code, IssueDefaultToken: true})
	require.ErrorContains(t, err, "default token unavailable")
	var accountCount int64
	require.NoError(t, DB.Model(&User{}).Where("username = ?", "atomic-public").Count(&accountCount).Error)
	require.Zero(t, accountCount)
	var code FurtherRegistrationCode
	require.NoError(t, DB.First(&code, codes[0].ID).Error)
	require.Equal(t, 1, code.Status)
	require.Zero(t, code.UsedByUserID)
}

func TestPublicRegistrationUsesServerRoleAndCommittedInitialToken(t *testing.T) {
	setupFurtherBenefitsDB(t)
	require.NoError(t, DB.AutoMigrate(&Token{}))
	require.NoError(t, SaveFurtherRegistrationSettings(FurtherRegistrationSettings{LocalRegistrationEnabled: true, PublicInitialQuota: 123}, ""))
	account, err := CreateRegisteredAccountContext(context.Background(), PublicRegistrationInput{Username: "public-token", Password: "password-for-public-signup", IssueDefaultToken: true, DefaultTokenAutoGroup: true})
	require.NoError(t, err)
	require.Equal(t, common.RoleCommonUser, account.Role)
	require.Equal(t, 123, account.Quota)
	var token Token
	require.NoError(t, DB.Where("user_id = ?", account.Id).First(&token).Error)
	require.Equal(t, "auto", token.Group)
	require.True(t, token.UnlimitedQuota)
	require.NotEqual(t, "password-for-public-signup", account.Password)
}

func TestCredentialPreviewUsesItsSnapshotWithoutDatabase(t *testing.T) {
	previous := DB
	DB = nil
	t.Cleanup(func() { DB = previous })
	channel := Channel{Key: "disabled\nenabled", ChannelInfo: ChannelInfo{IsMultiKey: true, MultiKeyStatusList: map[int]int{0: common.ChannelStatusManuallyDisabled}}}
	key, index, err := channel.GetNextEnabledKeyContext(context.Background())
	require.Nil(t, err)
	require.Equal(t, "enabled", key)
	require.Equal(t, 1, index)
}
