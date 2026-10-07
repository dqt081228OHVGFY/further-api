package model

import (
	"context"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/stretchr/testify/require"
)

func TestPersonalTokenMutationRequiresStillActiveProofSession(t *testing.T) {
	for _, command := range []string{"generate", "revoke"} {
		for _, change := range []string{"session_revoked", "account_disabled", "auth_version_advanced"} {
			t.Run(command+"/"+change, func(t *testing.T) {
				db, actor, _, operator := accountOperatorAttackFixture(t)
				identity := *operator.Session
				switch change {
				case "session_revoked":
					require.NoError(t, db.Model(&UserSession{}).Where("sid = ?", identity.SessionID).Update("status", UserSessionStatusRevoked).Error)
				case "account_disabled":
					require.NoError(t, db.Model(&actor).Update("status", common.UserStatusDisabled).Error)
				case "auth_version_advanced":
					require.NoError(t, db.Model(&actor).Update("auth_version", identity.UserAuthVersion+1).Error)
				}
				var err error
				if command == "generate" {
					err = UpdateUserAccessTokenForSessionContext(context.Background(), identity, "attacker-new-token")
				} else {
					_, err = RevokeUserAccessTokenForSessionContext(context.Background(), identity)
				}
				require.ErrorIs(t, err, ErrUserSessionInactive)
				var saved User
				require.NoError(t, db.First(&saved, actor.Id).Error)
				require.Equal(t, actor.GetAccessToken(), saved.GetAccessToken())
			})
		}
	}
}

func TestPersonalTokenMutationAllowsLiveSessionAndRejectsCanceledRequest(t *testing.T) {
	db, actor, _, operator := accountOperatorAttackFixture(t)
	identity := *operator.Session
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, UpdateUserAccessTokenForSessionContext(ctx, identity, "canceled-token"), context.Canceled)
	_, err := RevokeUserAccessTokenForSessionContext(ctx, identity)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, UpdateUserAccessTokenForSessionContext(context.Background(), identity, "authorized-replacement"))
	var saved User
	require.NoError(t, db.First(&saved, actor.Id).Error)
	require.Equal(t, "authorized-replacement", saved.GetAccessToken())
	ref, err := RevokeUserAccessTokenForSessionContext(context.Background(), identity)
	require.NoError(t, err)
	require.Equal(t, AccessTokenFingerprint("authorized-replacement"), ref)
	require.NoError(t, db.First(&saved, actor.Id).Error)
	require.Empty(t, saved.GetAccessToken())
}
