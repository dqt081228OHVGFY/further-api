package model

import (
	"context"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/stretchr/testify/require"
)

func TestManagedSecurityMutationRechecksPromotedTargetAndDemotedActor(t *testing.T) {
	db := businessCommandDatabase(t)
	require.NoError(t, db.AutoMigrate(&TwoFA{}, &TwoFABackupCode{}))
	pat := "security-fixture-pat"
	actor := User{Username: "security-admin", AffCode: "security-admin", Role: common.RoleAdminUser, AccessToken: &pat}
	target := User{Username: "security-target", AffCode: "security-target", Role: common.RoleCommonUser}
	require.NoError(t, db.Create(&actor).Error)
	require.NoError(t, db.Create(&target).Error)
	require.NoError(t, db.Create(&TwoFA{UserId: target.Id, Secret: "fixture", IsEnabled: true}).Error)
	operator := AccountSecurityOperator{UserID: actor.Id, Role: actor.Role, PersonalAccessTokenRef: AccessTokenFingerprint(pat)}
	require.NoError(t, db.Model(&User{}).Where("id = ?", target.Id).Update("role", common.RoleRootUser).Error)
	_, err := MutateManagedAccountSecurityContext(context.Background(), operator, ManagedSecurityMutation{TargetID: target.Id, Action: "twofa-disable"})
	require.ErrorIs(t, err, ErrAccountSecurityPermission)
	require.NoError(t, db.Model(&User{}).Where("id = ?", target.Id).Update("role", common.RoleCommonUser).Error)
	require.NoError(t, db.Model(&User{}).Where("id = ?", actor.Id).Update("role", common.RoleCommonUser).Error)
	_, err = MutateManagedAccountSecurityContext(context.Background(), operator, ManagedSecurityMutation{TargetID: target.Id, Action: "twofa-disable"})
	require.ErrorIs(t, err, ErrAccountSecurityPermission)
	factor, err := GetTwoFAByUserId(target.Id)
	require.NoError(t, err)
	require.True(t, factor.IsEnabled)
}

func TestCanceledSecurityCommandsDoNotChangeCredentialOrVersion(t *testing.T) {
	db := businessCommandDatabase(t)
	require.NoError(t, db.AutoMigrate(&TwoFA{}, &TwoFABackupCode{}, &PasskeyCredential{}))
	user := User{Username: "canceled-security", AffCode: "canceled-security", AuthVersion: 1}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&TwoFA{UserId: user.Id, Secret: "fixture", IsEnabled: true}).Error)
	require.NoError(t, db.Create(&PasskeyCredential{UserID: user.Id, CredentialID: "fixture", PublicKey: "fixture"}).Error)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	identity := AuthSessionIdentity{UserID: user.Id}
	require.ErrorIs(t, DisableTwoFAForSessionContext(ctx, identity), context.Canceled)
	require.ErrorIs(t, DeletePasskeyForSessionContext(ctx, identity), context.Canceled)
	require.ErrorIs(t, ReplaceBackupCodesForSessionContext(ctx, identity, []string{"fixture"}), context.Canceled)
	require.ErrorIs(t, DeleteUserForSessionContext(ctx, identity), context.Canceled)
	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	require.EqualValues(t, 1, stored.AuthVersion)
	factor, err := GetTwoFAByUserId(user.Id)
	require.NoError(t, err)
	require.True(t, factor.IsEnabled)
	_, err = GetPasskeyByUserID(user.Id)
	require.NoError(t, err)
}
