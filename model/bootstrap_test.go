package model

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func bootstrapTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	db := namespaceTestDB(t)
	require.NoError(t, db.AutoMigrate(&Setup{}, &User{}, &Option{}))
	previousDB, previousOptions := DB, common.OptionMap
	previousSelf, previousDemo := operation_setting.SelfUseModeEnabled, operation_setting.DemoSiteEnabled
	DB, common.OptionMap = db, map[string]string{}
	t.Cleanup(func() {
		DB, common.OptionMap = previousDB, previousOptions
		operation_setting.SelfUseModeEnabled, operation_setting.DemoSiteEnabled = previousSelf, previousDemo
	})
	return db
}

func validBootstrapRequest() BootstrapRequest {
	return BootstrapRequest{Username: "站点管理员", Password: "Further-admin-pass-123!", ConfirmPassword: "Further-admin-pass-123!", SelfUseModeEnabled: true}
}

func TestBootstrapAcceptsEmailAdministratorWithinAccountLimit(t *testing.T) {
	db := bootstrapTestDatabase(t)
	request := validBootstrapRequest()
	request.Username = "admin.user@mail.test"
	require.Len(t, request.Username, 20)
	require.NoError(t, BootstrapGateway(context.Background(), request))
	var owner User
	require.NoError(t, db.Where("username = ?", request.Username).First(&owner).Error)
	require.Equal(t, common.RoleRootUser, owner.Role)
	require.True(t, common.ValidatePasswordAndHash(request.Password, owner.Password))
	request.Username = strings.Repeat("a", 21)
	_, err := bootstrapAccount(request)
	require.ErrorContains(t, err, "20")
}

func TestBootstrapRollsBackOwnerOptionsAndMarkerOnFailure(t *testing.T) {
	db := bootstrapTestDatabase(t)
	failure := errors.New("simulated option store outage")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("bootstrap_option_failure", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "options" {
			tx.AddError(failure)
		}
	}))
	require.ErrorIs(t, BootstrapGateway(context.Background(), validBootstrapRequest()), failure)
	for _, target := range []any{&User{}, &Option{}, &Setup{}} {
		var count int64
		require.NoError(t, db.Model(target).Count(&count).Error)
		require.Zero(t, count)
	}
	require.Empty(t, common.OptionMap)
	require.NoError(t, db.Callback().Create().Remove("bootstrap_option_failure"))
	require.NoError(t, BootstrapGateway(context.Background(), validBootstrapRequest()))
	var owner User
	require.NoError(t, db.First(&owner).Error)
	require.Equal(t, "管理员", owner.DisplayName)
	require.NotEqual(t, validBootstrapRequest().Password, owner.Password)
	require.Equal(t, "true", common.OptionMap["SelfUseModeEnabled"])
	require.ErrorIs(t, BootstrapGateway(context.Background(), validBootstrapRequest()), ErrSetupAlreadyCompleted)
}

func TestBootstrapRejectsEmptyUsernameWithoutPartialSetup(t *testing.T) {
	db := bootstrapTestDatabase(t)
	request := validBootstrapRequest()
	request.Username = "   "
	var inputError *SetupInputError
	require.ErrorAs(t, BootstrapGateway(context.Background(), request), &inputError)
	var count int64
	require.NoError(t, db.Model(&Setup{}).Count(&count).Error)
	require.Zero(t, count)
}
