package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Derive the deployed profiles independently so a signing-domain change cannot
// accidentally update both production verification and the migration fixtures.
func migrationSigningKey(profile, purpose string) []byte {
	mac := hmac.New(sha256.New, []byte(common.SessionSecret))
	_, _ = mac.Write([]byte(profile + "/auth/" + purpose + "/v1"))
	return mac.Sum(nil)
}

func TestDashboardMigrationKeepsSigningProfilesIsolated(t *testing.T) {
	useTestSessionSecret(t)
	for _, profile := range []struct{ issuer, audience string }{
		{"further-api", "further-api-dashboard"},
		{"new-api", "new-api-dashboard"},
	} {
		t.Run(profile.issuer, func(t *testing.T) {
			now := time.Now()
			claims := authClaims{
				TokenUse: "access", SessionID: "migration-session", UserAuthVersion: 2, SessionVersion: 3,
				RegisteredClaims: jwt.RegisteredClaims{
					Issuer: profile.issuer, Subject: "42", Audience: jwt.ClaimStrings{profile.audience},
					ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)), IssuedAt: jwt.NewNumericDate(now),
					NotBefore: jwt.NewNumericDate(now.Add(-time.Second)), ID: "migration-access",
				},
			}
			raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(migrationSigningKey(profile.issuer, "access"))
			require.NoError(t, err)
			identity, internal, err := ParseDashboardAccessToken(raw)
			require.NoError(t, err)
			assert.True(t, internal)
			assert.Equal(t, AuthIdentity{UserID: 42, SessionID: "migration-session", UserAuthVersion: 2, SessionVersion: 3}, identity)

			otherProfile := "new-api"
			if profile.issuer == otherProfile {
				otherProfile = "further-api"
			}
			for _, key := range [][]byte{
				migrationSigningKey(otherProfile, "access"),
				migrationSigningKey(profile.issuer, "security_proof"),
			} {
				raw, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)
				require.NoError(t, err)
				_, internal, err = ParseDashboardAccessToken(raw)
				assert.True(t, internal, "an invalid internal credential must never fall through to opaque-token authentication")
				assert.ErrorIs(t, err, ErrAuthTokenInvalid)
			}

			claims.Audience = jwt.ClaimStrings{"another-dashboard"}
			raw, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(migrationSigningKey(profile.issuer, "access"))
			require.NoError(t, err)
			_, err = ParseAccessToken(raw)
			assert.ErrorIs(t, err, ErrAuthTokenInvalid)
		})
	}
}

func TestLegacySecurityProofSurvivesMigrationAndRemainsSingleUse(t *testing.T) {
	user := setupAuthSessionTestDB(t)
	useTestSessionSecret(t)
	require.NoError(t, model.DB.AutoMigrate(&model.TwoFA{}, &model.PasskeyCredential{}))
	bundle, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "migration-test")
	require.NoError(t, err)
	identity, err := ParseAccessToken(bundle.AccessToken)
	require.NoError(t, err)
	now := time.Now()
	proofID, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose: model.AuthFlowPurposeSecurityProof, UserId: user.Id, SessionId: identity.SessionID,
		ExpiresAt: now.Add(time.Minute),
	})
	require.NoError(t, err)
	contextHash := common.GenerateHMACWithKey(migrationSigningKey("new-api", "verification-context"), `{"scope":"access_token.generate","context":{}}`)
	claims := authClaims{
		TokenUse: "security_proof", SessionID: identity.SessionID,
		UserAuthVersion: identity.UserAuthVersion, SessionVersion: identity.SessionVersion,
		Method: "password", Scopes: []string{"access_token.generate"}, ContextHash: contextHash,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "new-api", Subject: strconv.Itoa(user.Id), Audience: jwt.ClaimStrings{"new-api-dashboard"},
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)), IssuedAt: jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Second)), ID: proofID,
		},
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(migrationSigningKey("new-api", "security_proof"))
	require.NoError(t, err)
	_, internal, err := ParseDashboardAccessToken(raw)
	assert.True(t, internal)
	assert.ErrorIs(t, err, ErrAuthTokenInvalid, "a proof must never authenticate a dashboard request")
	operation := VerificationOperation{Scope: VerificationScopeAccessTokenGenerate}
	authorization, err := ConsumeOperationProof(raw, identity, operation)
	require.NoError(t, err)
	assert.Equal(t, contextHash, authorization.ContextHash)
	_, err = ConsumeOperationProof(raw, identity, operation)
	assert.ErrorIs(t, err, ErrProofConsumed)
}

func TestRefreshPreservesSessionWhenDatabaseIsUnavailable(t *testing.T) {
	user := setupAuthSessionTestDB(t)
	useTestSessionSecret(t)
	bundle, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "migration-test")
	require.NoError(t, err)
	forcedErr := errors.New("session database is temporarily unavailable")
	callbackName := "test:refresh_database_unavailable"
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_sessions" {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Query().Remove(callbackName) })
	_, _, err = RefreshLoginSession(bundle.RefreshToken, bundle.Session.SID, "127.0.0.1", "migration-test")
	assert.ErrorIs(t, err, forcedErr)
	status, code := AuthSessionErrorCode(err)
	assert.Equal(t, 500, status)
	assert.Equal(t, "AUTH_INTERNAL_ERROR", code)
}

func TestRefreshRevocationDuringRotationReturnsUnauthorized(t *testing.T) {
	user := setupAuthSessionTestDB(t)
	useTestSessionSecret(t)
	bundle, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "migration-test")
	require.NoError(t, err)
	reads := 0
	callbackName := "test:revoke_before_refresh_rotation"
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "user_sessions" {
			return
		}
		reads++
		if reads == 2 {
			tx.AddError(model.DB.Model(&model.UserSession{}).Where("sid = ?", bundle.Session.SID).
				Updates(map[string]any{"status": model.UserSessionStatusRevoked, "revoked_at": time.Now().Unix()}).Error)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Query().Remove(callbackName) })
	_, _, err = RefreshLoginSession(bundle.RefreshToken, bundle.Session.SID, "127.0.0.1", "migration-test")
	assert.ErrorIs(t, err, ErrLoginSessionRevoked)
	status, code := AuthSessionErrorCode(err)
	assert.Equal(t, 401, status)
	assert.Equal(t, "AUTH_SESSION_REVOKED", code)
}
