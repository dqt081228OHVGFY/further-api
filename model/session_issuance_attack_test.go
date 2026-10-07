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

func TestLoginSessionAtomicIssuanceRejectsChangedAuthorityAndRollsBackFailures(t *testing.T) {
	for _, change := range []string{"disabled", "version_advanced", "deleted", "canceled", "insert_failed"} {
		t.Run(change, func(t *testing.T) {
			db, _, user, _ := accountOperatorAttackFixture(t)
			session := &UserSession{SID: "atomic-fixture-login", UserID: user.Id, UserAuthVersion: user.AuthVersion, Version: 1, Status: UserSessionStatusActive, RefreshHash: "fixture-refresh", ExpiresAt: time.Now().Unix() + 3600}
			ctx := context.Background()
			expected := error(ErrUserSessionInactive)
			switch change {
			case "disabled":
				require.NoError(t, db.Model(&user).Update("status", common.UserStatusDisabled).Error)
			case "version_advanced":
				require.NoError(t, db.Model(&user).Update("auth_version", user.AuthVersion+1).Error)
			case "deleted":
				require.NoError(t, db.Delete(&user).Error)
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				expected = context.Canceled
			case "insert_failed":
				expected = errors.New("fixture session insertion failure")
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:atomic-session-insert-failure", func(tx *gorm.DB) {
					if tx.Statement.Table == "user_sessions" {
						tx.AddError(expected)
					}
				}))
			}
			err := CreateUserSessionForLoginContext(ctx, session)
			require.ErrorIs(t, err, expected)
			var count int64
			require.NoError(t, db.Model(&UserSession{}).Where("sid = ?", session.SID).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}
