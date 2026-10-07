package controller

import (
	"errors"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAccountDeletionSessionWriteFailureRollsBackAccountAndConsumesProof(t *testing.T) {
	account, identity := setupSecurityEnrollmentTest(t)
	proof := issueSecurityEnrollmentProof(t, identity, service.VerificationOperation{Scope: service.VerificationScopeAccountDelete}, "password")
	callback := "test:account-delete-session-revoke-failure"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_sessions" {
			tx.AddError(errors.New("forced account session revocation failure"))
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(callback) })
	response := securityEnrollmentRequest("DELETE", "/api/user/self", "", proof, identity, DeleteSelf)
	require.Contains(t, response.Body.String(), "\"success\":false")
	require.NoError(t, model.DB.Callback().Update().Remove(callback))
	var saved model.User
	require.NoError(t, model.DB.Unscoped().First(&saved, account.Id).Error)
	require.False(t, saved.DeletedAt.Valid)
	require.Equal(t, identity.UserAuthVersion, saved.AuthVersion)
	var session model.UserSession
	require.NoError(t, model.DB.First(&session, "sid = ?", identity.SessionID).Error)
	require.Equal(t, model.UserSessionStatusActive, session.Status)
	require.Zero(t, session.RevokedAt)
	_, _, err := service.ValidateLoginSession(identity)
	require.NoError(t, err)
	replay := securityEnrollmentRequest("DELETE", "/api/user/self", "", proof, identity, DeleteSelf)
	require.Contains(t, replay.Body.String(), "SECURITY_PROOF_CONSUMED")
}
