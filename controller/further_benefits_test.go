package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/oauth"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupFurtherRegistrationController(t *testing.T) *gorm.DB {
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
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Option{}, &model.Log{}, &model.FurtherRegistrationSettings{}, &model.FurtherRegistrationCode{}))
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

func TestFurtherRegistrationControllerRetainsSanitizationAndOneUse(t *testing.T) {
	db := setupFurtherRegistrationController(t)
	require.NoError(t, model.SaveFurtherRegistrationSettings(model.FurtherRegistrationSettings{LocalRegistrationEnabled: true, RegistrationCodeRequired: true, PublicInitialQuota: 123}, "shared-password-123"))
	codes, err := model.GenerateFurtherRegistrationCodes("admin issued", "pay", 1, time.Now().UTC())
	require.NoError(t, err)
	engine := gin.New()
	engine.POST("/register", Register)
	engine.GET("/config", FurtherGetRegistrationConfig)
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

func TestFurtherLinuxDORegistrationHookRejectsClosedAndConsumesCodeAtomically(t *testing.T) {
	db := setupFurtherRegistrationController(t)
	settings := model.FurtherRegistrationSettings{LocalRegistrationEnabled: true, LinuxdoLoginEnabled: true, RegistrationCodeRequired: true, PublicInitialQuota: 100, LinuxdoExtraQuota: 25}
	require.NoError(t, model.SaveFurtherRegistrationSettings(settings, ""))
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
	require.NoError(t, model.SaveFurtherRegistrationSettings(settings, ""))
	_, _, err = findOrCreateOAuthUser(ctx, provider, identity, &oauth.OAuthToken{}, "")
	require.Error(t, err, "code requirement also applies to new OAuth accounts")
	codes, err := model.GenerateFurtherRegistrationCodes("OAuth admission", "default", 1, time.Now().UTC())
	require.NoError(t, err)
	admission, err := model.PrepareFurtherRegistrationAdmission(codes[0].Code)
	require.NoError(t, err)
	encoded, err := json.Marshal(oauthFlowPayload{RegistrationAdmission: admission})
	require.NoError(t, err)
	require.NotContains(t, string(encoded), codes[0].Code)
	user, _, err := findOrCreateOAuthUser(ctx, provider, identity, &oauth.OAuthToken{}, "", admission)
	require.NoError(t, err)
	require.Equal(t, 125, user.Quota)
	require.Equal(t, "987654", user.LinuxDOId)
	var code model.FurtherRegistrationCode
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

func TestFurtherRegistrationPatchPreservesSiteLinuxDOSetting(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[enabled], func(t *testing.T) {
			setupFurtherRegistrationController(t)
			require.NoError(t, model.SaveFurtherRegistrationSettings(model.FurtherRegistrationSettings{
				LocalRegistrationEnabled:   true,
				LinuxdoLoginEnabled:        !enabled,
				LinuxdoRegistrationEnabled: true,
			}, ""))
			// The single provider editor changes the live option independently of
			// the old registration-table copy.
			common.LinuxDOOAuthEnabled = enabled
			engine := gin.New()
			engine.GET("/settings", FurtherGetRegistrationSettings)
			engine.PATCH("/settings", FurtherPatchRegistrationSettings)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/settings", nil))
			var result struct {
				Success bool `json:"success"`
				Data    struct {
					Settings struct {
						LinuxdoLoginEnabled bool `json:"linuxdo_login_enabled"`
					} `json:"settings"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
			require.True(t, result.Success, response.Body.String())
			require.Equal(t, enabled, result.Data.Settings.LinuxdoLoginEnabled)
			response = httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPatch, "/settings", strings.NewReader(`{"public_initial_quota":250}`))
			request.Header.Set("Content-Type", "application/json")
			engine.ServeHTTP(response, request)
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
			require.True(t, result.Success, response.Body.String())
			require.Equal(t, enabled, common.LinuxDOOAuthEnabled)
			stored, err := model.GetFurtherRegistrationSettings()
			require.NoError(t, err)
			require.Equal(t, enabled, stored.LinuxdoLoginEnabled)
			require.Equal(t, 250, stored.PublicInitialQuota)
		})
	}
}

func TestFurtherLinuxDORegistrationUsesSiteProviderSetting(t *testing.T) {
	setupFurtherRegistrationController(t)
	require.NoError(t, model.SaveFurtherRegistrationSettings(model.FurtherRegistrationSettings{
		LocalRegistrationEnabled:   true,
		LinuxdoLoginEnabled:        false,
		LinuxdoRegistrationEnabled: true,
		PublicInitialQuota:         100,
	}, ""))
	common.LinuxDOOAuthEnabled = true
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/oauth/linuxdo", nil)
	provider := &oauth.LinuxDOProvider{}
	identity := &oauth.OAuthUser{ProviderUserID: "site-enabled", Username: "site-enabled", Extra: map[string]interface{}{}}
	user, _, err := findOrCreateOAuthUser(ctx, provider, identity, &oauth.OAuthToken{}, "")
	require.NoError(t, err)
	require.Equal(t, 100, user.Quota)
	common.LinuxDOOAuthEnabled = false
	identity = &oauth.OAuthUser{ProviderUserID: "site-disabled", Username: "site-disabled", Extra: map[string]interface{}{}}
	_, _, err = findOrCreateOAuthUser(ctx, provider, identity, &oauth.OAuthToken{}, "")
	require.Error(t, err)
}
