package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFurtherChannelReadRoutesHonorDeniedPermission(t *testing.T) {
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
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AuditLog{}, &model.CasbinRule{}, &model.AuthzRole{}, &model.FurtherChannelPolicy{}))
	require.NoError(t, authz.Init(db))
	pat := "channel-read-denied-test-pat"
	admin := model.User{Username: "restricted-channel-admin", AffCode: "restricted-channel-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, AccessToken: &pat}
	require.NoError(t, db.Create(&admin).Error)
	require.NoError(t, authz.SetUserPermissions(admin.Id, authz.PermissionsMap{"channel": {"read": false}}))
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetFurtherLimitsRouter(engine.Group("/api"))
	request := func(path string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+pat)
		engine.ServeHTTP(response, req)
		return response
	}
	for _, path := range []string{"/api/further/channels/policies", "/api/further/channels/1/policy", "/api/further/channels/1/keys"} {
		t.Run(path, func(t *testing.T) {
			response := request(path)
			require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
		})
	}
	require.NoError(t, authz.ClearUserPermissions(admin.Id))
	response := request("/api/further/channels/policies")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"success":true`)
}
