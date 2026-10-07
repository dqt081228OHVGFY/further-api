package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelReadOmitsCredentialBearingConfigurationAndPreservesItOnRoutingEdit(t *testing.T) {
	previousDB, previousLog := model.DB, model.LOG_DB
	previousRedis, previousMemory, previousMaster := common.RedisEnabled, common.MemoryCacheEnabled, common.IsMasterNode
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	connection.SetMaxOpenConns(1)
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled, common.MemoryCacheEnabled, common.IsMasterNode = false, false, true
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLog
		common.RedisEnabled, common.MemoryCacheEnabled, common.IsMasterNode = previousRedis, previousMemory, previousMaster
		require.NoError(t, connection.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AuditLog{}, &model.Log{}, &model.CasbinRule{}, &model.AuthzRole{}, &model.Channel{}, &model.Ability{}))
	require.NoError(t, authz.Init(db))
	pat := "channel-configuration-fixture-pat"
	admin := model.User{Username: "channel-config-reader", AffCode: "channel-config-reader", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, AccessToken: &pat}
	require.NoError(t, db.Create(&admin).Error)
	require.NoError(t, authz.SetUserPermissions(admin.Id, authz.PermissionsMap{"channel": {"read": true, "write": true, "sensitive_write": false}}))
	base := "https://proxy:fixture-base-secret@provider.invalid/v1?key=fixture-url-secret"
	header := `{"Authorization":"Bearer fixture-header-secret"}`
	param := `{"api_key":"fixture-param-secret"}`
	setting := `{"proxy":"http://proxy:fixture-proxy-secret@proxy.invalid:8080"}`
	tag := "routing-channel"
	channel := model.Channel{Name: "routing-channel", Tag: &tag, Type: 1, Key: "fixture-upstream-key", Group: "default", Models: "public,keep", Status: 1, BaseURL: &base, HeaderOverride: &header, ParamOverride: &param, Setting: &setting, Other: "fixture-other-secret", OtherSettings: `{"tenant_token":"fixture-settings-secret"}`}
	require.NoError(t, db.Create(&channel).Error)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerChannelRoutes(engine.Group("/api"))
	invoke := func(method, path string, body any) *httptest.ResponseRecorder {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
		request.Header.Set("Authorization", "Bearer "+pat)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response
	}
	assertRedacted := func(response *httptest.ResponseRecorder) {
		t.Helper()
		require.Equal(t, http.StatusOK, response.Code)
		for _, secret := range []string{"fixture-base-secret", "fixture-url-secret", "fixture-header-secret", "fixture-param-secret", "fixture-proxy-secret", "fixture-other-secret", "fixture-settings-secret", "fixture-upstream-key"} {
			require.NotContains(t, response.Body.String(), secret)
		}
		for _, field := range []string{"base_url", "header_override", "param_override", "setting", "settings", "other", "openai_organization", "key"} {
			require.NotContains(t, response.Body.String(), `"`+field+`":`, "hidden fields must be omitted, not returned as writable mask/null placeholders")
		}
	}
	path := "/api/channel/" + strconv.Itoa(channel.Id)
	for _, endpoint := range []string{path, "/api/channel/", "/api/channel/?tag_mode=true", "/api/channel/search?keyword=routing-channel", "/api/channel/search?keyword=routing-channel&tag_mode=true"} {
		assertRedacted(invoke(http.MethodGet, endpoint, nil))
	}
	response := invoke(http.MethodPut, "/api/channel/", map[string]any{"id": channel.Id, "type": channel.Type, "priority": 9, "models": "public,keep,added"})
	assertRedacted(response)
	require.Contains(t, response.Body.String(), `"success":true`)
	stored, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.Equal(t, channel.Key, stored.Key)
	require.Equal(t, base, *stored.BaseURL)
	require.Equal(t, header, *stored.HeaderOverride)
	require.Equal(t, param, *stored.ParamOverride)
	require.Equal(t, setting, *stored.Setting)
	require.Equal(t, channel.Other, stored.Other)
	require.Equal(t, channel.OtherSettings, stored.OtherSettings)
	require.Equal(t, int64(9), stored.GetPriority())
	require.Equal(t, "public,keep,added", stored.Models)
	response = invoke(http.MethodPut, "/api/channel/", map[string]any{"id": channel.Id, "header_override": `{"Authorization":"Bearer replacement-fixture-secret"}`})
	require.Contains(t, response.Body.String(), `"success":false`)
	stored, err = model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.Equal(t, header, *stored.HeaderOverride)

	vertexSettings := `{"default":"us-central1","tenant_token":"fixture-vertex-secret"}`
	vertex := model.Channel{Name: "vertex-routing", Type: constant.ChannelTypeVertexAi, Key: "fixture-vertex-key", Other: vertexSettings, Group: "default", Models: "vertex", Status: 1}
	require.NoError(t, db.Create(&vertex).Error)
	response = invoke(http.MethodPut, "/api/channel/", map[string]any{"id": vertex.Id, "type": vertex.Type, "priority": 4, "models": "vertex,added"})
	require.Contains(t, response.Body.String(), `"success":true`, "stored provider settings validate omitted configuration without resubmitting it")
	require.NotContains(t, response.Body.String(), "fixture-vertex-secret")
	stored, err = model.GetChannelById(vertex.Id, true)
	require.NoError(t, err)
	require.Equal(t, vertexSettings, stored.Other)
	require.Equal(t, vertex.Key, stored.Key)
	require.Equal(t, int64(4), stored.GetPriority())

	require.NoError(t, authz.SetUserPermissions(admin.Id, authz.PermissionsMap{"channel": {"sensitive_write": true}}))
	response = invoke(http.MethodGet, path, nil)
	require.Contains(t, response.Body.String(), "fixture-header-secret")
	require.Contains(t, response.Body.String(), "fixture-proxy-secret")
	require.NotContains(t, response.Body.String(), channel.Key)
	require.NoError(t, authz.SetUserPermissions(admin.Id, authz.PermissionsMap{"channel": {"sensitive_write": false}}))
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", admin.Id).Update("role", common.RoleRootUser).Error)
	response = invoke(http.MethodGet, path, nil)
	require.Contains(t, response.Body.String(), "fixture-header-secret")
	require.Contains(t, response.Body.String(), "fixture-proxy-secret")
	require.NotContains(t, response.Body.String(), channel.Key)
}
