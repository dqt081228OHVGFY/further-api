package controller

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupLegacyBrowserSession(t *testing.T) (sid, refresh string) {
	t.Helper()
	previousDB, previousRedis, previousSecret := model.DB, common.RedisEnabled, common.SessionSecret
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}))
	model.DB, common.RedisEnabled, common.SessionSecret = db, false, "legacy-browser-session-test-secret"
	t.Cleanup(func() {
		model.DB, common.RedisEnabled, common.SessionSecret = previousDB, previousRedis, previousSecret
		_ = sqlDB.Close()
	})
	user := &model.User{
		Username: "legacy-browser-user", Password: "unused", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(user).Error)
	sid, secret := uuid.NewString(), strings.Repeat("a", 64)
	mac := hmac.New(sha256.New, []byte(common.SessionSecret))
	_, _ = mac.Write([]byte("new-api/auth/refresh/v1"))
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.UserSession{
		SID: sid, UserID: user.Id, Version: 1, UserAuthVersion: 1, Status: model.UserSessionStatusActive,
		RefreshHash: common.GenerateHMACWithKey(mac.Sum(nil), secret), LoginMethod: "password",
		CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 3600,
	}).Error)
	return sid, sid + "." + secret
}

func legacyBrowserRequest(t *testing.T, path, sid, refresh string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	c.Request.Header.Set("X-Auth-Session", sid)
	c.Request.AddCookie(&http.Cookie{Name: "new_api_refresh", Value: refresh})
	handler(c)
	return recorder
}

func TestRefreshAuthMigratesLegacyBrowserSession(t *testing.T) {
	sid, refresh := setupLegacyBrowserSession(t)
	previousSession, err := model.GetUserSessionBySID(sid)
	require.NoError(t, err)
	recorder := legacyBrowserRequest(t, "/api/user/auth/refresh", sid, refresh, RefreshAuth)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			AccessToken string                   `json:"access_token"`
			Session     service.LoginSessionView `json:"session"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, sid, response.Data.Session.SID)
	assert.True(t, response.Data.Session.Current)
	assert.Equal(t, previousSession.ExpiresAt, response.Data.Session.ExpiresAt)
	identity, err := service.ParseAccessToken(response.Data.AccessToken)
	require.NoError(t, err)
	_, _, err = service.ValidateLoginSession(identity)
	require.NoError(t, err)
	claims := &jwt.RegisteredClaims{}
	_, _, err = jwt.NewParser().ParseUnverified(response.Data.AccessToken, claims)
	require.NoError(t, err)
	assert.Equal(t, "further-api", claims.Issuer)
	assert.Equal(t, jwt.ClaimStrings{"further-api-dashboard"}, claims.Audience)
	cookies := make(map[string]*http.Cookie)
	for _, cookie := range recorder.Result().Cookies() {
		cookies[cookie.Name] = cookie
	}
	require.Contains(t, cookies, "further_api_refresh")
	assert.NotEqual(t, refresh, cookies["further_api_refresh"].Value)
	assert.True(t, cookies["further_api_refresh"].HttpOnly)
	assert.Equal(t, "/api/user/auth", cookies["further_api_refresh"].Path)
	assert.Equal(t, previousSession.ExpiresAt, cookies["further_api_refresh"].Expires.Unix(),
		"migration must preserve the existing session expiry")
	for _, name := range []string{"further_api_has_session"} {
		require.Contains(t, cookies, name)
		assert.Equal(t, "1", cookies[name].Value)
		assert.False(t, cookies[name].HttpOnly)
		assert.Equal(t, cookies["further_api_refresh"].Expires, cookies[name].Expires)
	}
	require.Contains(t, cookies, "new_api_has_session")
	assert.Less(t, cookies["new_api_has_session"].MaxAge, 0)
	require.Contains(t, cookies, "new_api_refresh")
	assert.Less(t, cookies["new_api_refresh"].MaxAge, 0)

	// An old tab racing the migration can still recover the same rotation from
	// its old cookie; it must not revoke the newly migrated browser session.
	replayed := legacyBrowserRequest(t, "/api/user/auth/refresh", sid, refresh, RefreshAuth)
	require.Equal(t, http.StatusOK, replayed.Code)
	for _, cookie := range replayed.Result().Cookies() {
		if cookie.Name == "further_api_refresh" {
			assert.Equal(t, cookies[cookie.Name].Value, cookie.Value)
		}
	}
	logout := legacyBrowserRequest(t, "/api/user/auth/logout", sid, refresh, AuthLogout)
	require.Equal(t, http.StatusOK, logout.Code)
	stored, err := model.GetUserSessionBySID(sid)
	require.NoError(t, err)
	assert.Equal(t, model.UserSessionStatusRevoked, stored.Status)
	assert.Equal(t, "logout", stored.RevokedReason)
	cleared := make(map[string]*http.Cookie)
	for _, cookie := range logout.Result().Cookies() {
		cleared[cookie.Name] = cookie
	}
	for _, name := range []string{"further_api_refresh", "new_api_refresh", "further_api_has_session", "new_api_has_session"} {
		require.Contains(t, cleared, name)
		assert.Less(t, cleared[name].MaxAge, 0)
	}
	_, _, err = service.ValidateLoginSession(identity)
	assert.ErrorIs(t, err, service.ErrLoginSessionRevoked)
}

func TestRefreshAuthKeepsLegacyCookieDuringDatabaseFailure(t *testing.T) {
	sid, refresh := setupLegacyBrowserSession(t)
	callbackName := "test:legacy_refresh_database_failure"
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_sessions" {
			tx.AddError(errors.New("database unavailable"))
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Query().Remove(callbackName) })
	recorder := legacyBrowserRequest(t, "/api/user/auth/refresh", sid, refresh, RefreshAuth)
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Empty(t, recorder.Result().Cookies(), "a temporary failure must preserve the browser's refresh credential")
	var response struct {
		Code string `json:"code"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "AUTH_INTERNAL_ERROR", response.Code)
}
