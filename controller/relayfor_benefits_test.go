package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupRelayForRegistrationController(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB, oldLog := model.DB, model.LOG_DB
	oldRegister, oldPassword, oldEmail, oldLinux, oldRedis := common.RegisterEnabled, common.PasswordRegisterEnabled, common.EmailVerificationEnabled, common.LinuxDOOAuthEnabled, common.RedisEnabled
	oldOptions, oldValidate, oldQuota := common.OptionMap, common.Validate, common.QuotaForNewUser
	oldToken := constant.GenerateDefaultToken
	model.DB, model.LOG_DB = db, db
	common.RegisterEnabled, common.PasswordRegisterEnabled, common.EmailVerificationEnabled, common.LinuxDOOAuthEnabled, common.RedisEnabled = true, true, false, true, false
	common.OptionMap = make(map[string]string)
	common.Validate = validator.New()
	common.QuotaForNewUser = 0
	constant.GenerateDefaultToken = false
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Option{}, &model.Log{}, &model.RelayForRegistrationSettings{}, &model.RelayForRegistrationCode{}))
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLog
		common.RegisterEnabled, common.PasswordRegisterEnabled, common.EmailVerificationEnabled, common.LinuxDOOAuthEnabled, common.RedisEnabled = oldRegister, oldPassword, oldEmail, oldLinux, oldRedis
		common.OptionMap, common.Validate, common.QuotaForNewUser = oldOptions, oldValidate, oldQuota
		constant.GenerateDefaultToken = oldToken
		require.NoError(t, sqlDB.Close())
	})
	gin.SetMode(gin.TestMode)
	return db
}

func TestRelayForRegistrationControllerRetainsSanitizationAndOneUse(t *testing.T) {
	db := setupRelayForRegistrationController(t)
	require.NoError(t, model.SaveRelayForRegistrationSettings(model.RelayForRegistrationSettings{LocalRegistrationEnabled: true, RegistrationCodeRequired: true, PublicInitialQuota: 123}, "shared-password-123"))
	codes, err := model.GenerateRelayForRegistrationCodes("admin issued", "pay", 1, time.Now().UTC())
	require.NoError(t, err)
	engine := gin.New()
	engine.POST("/register", Register)
	engine.GET("/config", RelayForGetRegistrationConfig)
	for attempt := range 2 {
		body := map[string]any{"username": []string{"register-safe", "register-replay"}[attempt], "password": "new-password-123", "registration_code": codes[0].Code, "role": 100, "quota": 9999999, "status": 2}
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(string(encoded)))
		request.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(response, request)
		var payload struct {
			Success bool `json:"success"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
		require.Equal(t, attempt == 0, payload.Success, response.Body.String())
	}
	var user model.User
	require.NoError(t, db.Where("username = ?", "register-safe").First(&user).Error)
	require.Equal(t, common.RoleCommonUser, user.Role)
	require.Equal(t, common.UserStatusEnabled, user.Status)
	require.Equal(t, 123, user.Quota)
	require.Equal(t, "pay", user.Group)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/config", nil))
	require.NotContains(t, response.Body.String(), "shared-password-123")
	require.NotContains(t, response.Body.String(), "hash")
	require.NotContains(t, response.Body.String(), "initial_quota")
}

func TestRelayForLinuxDORegistrationHookRejectsClosedAndConsumesCodeAtomically(t *testing.T) {
	db := setupRelayForRegistrationController(t)
	settings := model.RelayForRegistrationSettings{LocalRegistrationEnabled: true, LinuxdoLoginEnabled: true, RegistrationCodeRequired: true, PublicInitialQuota: 100, LinuxdoExtraQuota: 25}
	require.NoError(t, model.SaveRelayForRegistrationSettings(settings, ""))
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/oauth/linuxdo", nil)
	provider := &oauth.LinuxDOProvider{}
	identity := &oauth.OAuthUser{ProviderUserID: "987654", Username: "linuxdo-test", Extra: map[string]interface{}{}}
	_, _, err := findOrCreateOAuthUser(ctx, provider, identity, &oauth.OAuthToken{}, "")
	require.Error(t, err)
	var count int64
	require.NoError(t, db.Model(&model.User{}).Count(&count).Error)
	require.Zero(t, count)
	settings.LinuxdoRegistrationEnabled = true
	require.NoError(t, model.SaveRelayForRegistrationSettings(settings, ""))
	_, _, err = findOrCreateOAuthUser(ctx, provider, identity, &oauth.OAuthToken{}, "")
	require.Error(t, err, "code requirement also applies to new OAuth accounts")
	codes, err := model.GenerateRelayForRegistrationCodes("OAuth admission", "default", 1, time.Now().UTC())
	require.NoError(t, err)
	admission, err := model.PrepareRelayForRegistrationAdmission(codes[0].Code)
	require.NoError(t, err)
	encoded, err := json.Marshal(oauthFlowPayload{RegistrationAdmission: admission})
	require.NoError(t, err)
	require.NotContains(t, string(encoded), codes[0].Code)
	user, _, err := findOrCreateOAuthUser(ctx, provider, identity, &oauth.OAuthToken{}, "", admission)
	require.NoError(t, err)
	require.Equal(t, 125, user.Quota)
	require.Equal(t, "987654", user.LinuxDOId)
	var code model.RelayForRegistrationCode
	require.NoError(t, db.First(&code, codes[0].ID).Error)
	require.Equal(t, 2, code.Status)
	require.Equal(t, user.Id, code.UsedByUserID)
	// Existing provider identities can log in without another registration code.
	existing, _, err := findOrCreateOAuthUser(ctx, provider, identity, &oauth.OAuthToken{}, "")
	require.NoError(t, err)
	require.Equal(t, user.Id, existing.Id)
	identity.ProviderUserID = "987655"
	identity.Username = "linuxdo-replay"
	_, _, err = findOrCreateOAuthUser(ctx, provider, identity, &oauth.OAuthToken{}, "", admission)
	require.Error(t, err)
	require.NoError(t, db.Model(&model.User{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
