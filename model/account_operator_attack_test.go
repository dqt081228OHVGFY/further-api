package model

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func accountOperatorAttackFixture(t *testing.T) (*gorm.DB, User, User, AccountSecurityOperator) {
	t.Helper()
	db := businessCommandDatabase(t)
	require.NoError(t, db.AutoMigrate(&AuthFlow{}, &TwoFA{}, &TwoFABackupCode{}, &PasskeyCredential{}, &UserOAuthBinding{}, &ExternalIdentityClaim{}))
	pat := "operator-attack-fixture-token"
	actor := User{Username: "attack-admin", AffCode: "operator-attack-admin", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AuthVersion: 1, AccessToken: &pat}
	target := User{Username: "attack-target", AffCode: "operator-attack-target", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1, Password: "unchanged-password", Quota: 100, DisplayName: "unchanged"}
	require.NoError(t, db.Create(&actor).Error)
	require.NoError(t, db.Create(&target).Error)
	require.NoError(t, db.Create(&UserOAuthBinding{UserId: target.Id, ProviderId: 7, ProviderUserId: "fixture-external-user"}).Error)
	identity := AuthSessionIdentity{UserID: actor.Id, SessionID: "operator-attack-fixture-session", UserAuthVersion: 1, SessionVersion: 1}
	require.NoError(t, db.Create(&UserSession{SID: identity.SessionID, UserID: actor.Id, Version: 1, UserAuthVersion: 1, Status: UserSessionStatusActive, ExpiresAt: time.Now().Unix() + 3600, RefreshHash: "fixture-refresh"}).Error)
	return db, actor, target, AccountSecurityOperator{UserID: actor.Id, Role: actor.Role, Session: &identity}
}

// These tests model a request that already passed HTTP authentication before a
// concurrent security change. The committing transaction must reject it.
func TestAccountCommandsRejectAuthorityRevokedAfterAuthentication(t *testing.T) {
	changes := []struct {
		name   string
		change func(*testing.T, *gorm.DB, User, *AccountSecurityOperator)
	}{
		{"actor_demoted", func(t *testing.T, db *gorm.DB, actor User, _ *AccountSecurityOperator) {
			require.NoError(t, db.Model(&actor).Update("role", common.RoleCommonUser).Error)
		}},
		{"actor_disabled", func(t *testing.T, db *gorm.DB, actor User, _ *AccountSecurityOperator) {
			require.NoError(t, db.Model(&actor).Update("status", common.UserStatusDisabled).Error)
		}},
		{"actor_deleted", func(t *testing.T, db *gorm.DB, actor User, _ *AccountSecurityOperator) {
			require.NoError(t, db.Delete(&actor).Error)
		}},
		{"auth_version_advanced", func(t *testing.T, db *gorm.DB, actor User, _ *AccountSecurityOperator) {
			require.NoError(t, db.Model(&actor).Update("auth_version", 2).Error)
		}},
		{"session_revoked", func(t *testing.T, db *gorm.DB, actor User, _ *AccountSecurityOperator) {
			require.NoError(t, db.Model(&UserSession{}).Where("user_id = ?", actor.Id).Update("status", UserSessionStatusRevoked).Error)
		}},
		{"session_expired", func(t *testing.T, db *gorm.DB, actor User, _ *AccountSecurityOperator) {
			require.NoError(t, db.Model(&UserSession{}).Where("user_id = ?", actor.Id).Update("expires_at", time.Now().Unix()-1).Error)
		}},
		{"pat_rotated", func(t *testing.T, db *gorm.DB, actor User, operator *AccountSecurityOperator) {
			operator.Session = nil
			operator.PersonalAccessTokenRef = AccessTokenFingerprint(actor.GetAccessToken())
			require.NoError(t, db.Model(&actor).Update("access_token", "replacement-token").Error)
		}},
		{"missing_credential", func(_ *testing.T, _ *gorm.DB, _ User, operator *AccountSecurityOperator) { operator.Session = nil }},
		{"wrong_session_owner", func(_ *testing.T, _ *gorm.DB, actor User, operator *AccountSecurityOperator) {
			operator.Session.UserID = actor.Id + 1
		}},
	}
	commands := []struct {
		name   string
		invoke func(AccountSecurityOperator, User) error
	}{
		{"edit_password", func(operator AccountSecurityOperator, target User) error {
			_, err := EditManagedAccount(context.Background(), operator, AccountProfileInput{ID: target.Id, Username: target.Username, DisplayName: "attacker-edit", Password: "attacker-password"}, nil)
			return err
		}},
		{"create_admin", func(operator AccountSecurityOperator, _ User) error {
			role := common.RoleAdminUser
			_, err := CreateManagedAccount(context.Background(), operator, AccountProfileInput{Username: "attacker-created", Password: "attacker-password", Role: &role}, nil)
			return err
		}},
		{"promote", func(operator AccountSecurityOperator, target User) error {
			_, err := ApplyManagedAccountAction(context.Background(), operator, AccountActionInput{ID: target.Id, Action: "promote"}, nil)
			return err
		}},
		{"erase", func(operator AccountSecurityOperator, target User) error {
			_, err := EraseManagedAccount(context.Background(), operator, target.Id)
			return err
		}},
		{"add_quota", func(operator AccountSecurityOperator, target User) error {
			_, err := AdjustUserQuota(context.Background(), target.Id, operator, "add", 100000)
			return err
		}},
		{"oauth-unbind", func(operator AccountSecurityOperator, target User) error {
			_, err := MutateManagedAccountSecurityContext(context.Background(), operator, ManagedSecurityMutation{TargetID: target.Id, Action: "oauth-unbind", ProviderID: 7})
			return err
		}},
	}
	for _, command := range commands {
		for _, change := range changes {
			t.Run(command.name+"/"+change.name, func(t *testing.T) {
				db, actor, target, operator := accountOperatorAttackFixture(t)
				change.change(t, db, actor, &operator)
				err := command.invoke(operator, target)
				require.True(t, errors.Is(err, ErrAccountSecurityPermission) || errors.Is(err, ErrUserSessionInactive) || errors.Is(err, ErrUserQuotaPermission), "expected an authorization rejection, got %v", err)
				var stored User
				require.NoError(t, db.First(&stored, target.Id).Error)
				require.Equal(t, target.Password, stored.Password)
				require.Equal(t, target.DisplayName, stored.DisplayName)
				require.Equal(t, target.Role, stored.Role)
				require.Equal(t, target.Quota, stored.Quota)
				require.Equal(t, target.AuthVersion, stored.AuthVersion)
				var created int64
				require.NoError(t, db.Model(&User{}).Where("username = ?", "attacker-created").Count(&created).Error)
				require.Zero(t, created)
				var bindings int64
				require.NoError(t, db.Model(&UserOAuthBinding{}).Where("user_id = ? AND provider_id = ?", target.Id, 7).Count(&bindings).Error)
				require.EqualValues(t, 1, bindings)
			})
		}
	}
}

func TestAccountCommandsAllowLiveSessionAndCurrentPersonalToken(t *testing.T) {
	for _, credential := range []string{"session", "pat"} {
		t.Run(credential, func(t *testing.T) {
			db, actor, target, operator := accountOperatorAttackFixture(t)
			if credential == "pat" {
				operator.Session = nil
				operator.PersonalAccessTokenRef = AccessTokenFingerprint(actor.GetAccessToken())
			}
			_, err := EditManagedAccount(context.Background(), operator, AccountProfileInput{ID: target.Id, Username: target.Username, DisplayName: "authorized-edit"}, nil)
			require.NoError(t, err)
			_, err = AdjustUserQuota(context.Background(), target.Id, operator, "add", 25)
			require.NoError(t, err)
			role := common.RoleAdminUser
			_, err = CreateManagedAccount(context.Background(), operator, AccountProfileInput{Username: "authorized-created", Password: "authorized-password", Role: &role}, nil)
			require.NoError(t, err)
			_, err = ApplyManagedAccountAction(context.Background(), operator, AccountActionInput{ID: target.Id, Action: "promote"}, nil)
			require.NoError(t, err)
			var stored User
			require.NoError(t, db.First(&stored, target.Id).Error)
			require.Equal(t, "authorized-edit", stored.DisplayName)
			require.Equal(t, 125, stored.Quota)
			require.Equal(t, common.RoleAdminUser, stored.Role)
			_, err = EraseManagedAccount(context.Background(), operator, target.Id)
			require.NoError(t, err)
			require.ErrorIs(t, db.First(&stored, target.Id).Error, gorm.ErrRecordNotFound)
		})
	}
}
