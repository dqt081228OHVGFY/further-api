package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFurtherWorkspaceBootstrapAnonymousAndAuthenticatedIsolation(t *testing.T) {
	previousDB, previousLog := model.DB, model.LOG_DB
	previousSetup, previousRedis, previousMemory := constant.Setup, common.RedisEnabled, common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	connection.SetMaxOpenConns(1)
	model.DB, model.LOG_DB, constant.Setup = db, db, true
	common.RedisEnabled, common.MemoryCacheEnabled = false, false
	t.Cleanup(func() {
		model.DB, model.LOG_DB, constant.Setup = previousDB, previousLog, previousSetup
		common.RedisEnabled, common.MemoryCacheEnabled = previousRedis, previousMemory
		require.NoError(t, connection.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AuditLog{}))
	memberToken, rootToken, bannedToken := "workspace-member-pat", "workspace-root-pat", "workspace-banned-pat"
	member := model.User{Username: "bootstrap-member", AffCode: "bootstrap-member", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AccessToken: &memberToken}
	root := model.User{Username: "bootstrap-root", AffCode: "bootstrap-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AccessToken: &rootToken}
	banned := model.User{Username: "bootstrap-banned", AffCode: "bootstrap-banned", Role: common.RoleCommonUser, Status: common.UserStatusDisabled, AccessToken: &bannedToken}
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&banned).Error)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetFurtherWorkspaceRouter(engine.Group("/api"))
	request := func(token string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/further/workspace/bootstrap?user_id="+strconv.Itoa(root.Id), nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		engine.ServeHTTP(recorder, req)
		return recorder
	}
	var anonymous struct {
		Success bool                       `json:"success"`
		Data    service.WorkspaceBootstrap `json:"data"`
	}
	response := request("")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &anonymous))
	require.True(t, anonymous.Success)
	require.Equal(t, 1, anonymous.Data.SchemaVersion)
	require.False(t, anonymous.Data.Authenticated)
	require.Nil(t, anonymous.Data.User)
	require.Contains(t, response.Header().Get("Cache-Control"), "no-store")
	for _, token := range []string{"invalid-pat", bannedToken} {
		response := request(token)
		require.Equal(t, http.StatusUnauthorized, response.Code, response.Body.String())
		require.NotContains(t, response.Body.String(), rootToken)
		require.NotContains(t, response.Body.String(), root.Username)
	}
	response = request(memberToken)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &anonymous))
	require.True(t, anonymous.Data.Authenticated)
	require.Equal(t, member.Id, anonymous.Data.User.ID, "subject cannot be changed with query parameters")
	require.NotContains(t, response.Body.String(), memberToken)
	require.NotContains(t, response.Body.String(), root.Username)
	for _, actions := range anonymous.Data.User.Permissions.AdminPermissions {
		for _, allowed := range actions {
			require.False(t, allowed)
		}
	}
	response = request(rootToken)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &anonymous))
	require.Equal(t, root.Id, anonymous.Data.User.ID)
	for _, actions := range anonymous.Data.User.Permissions.AdminPermissions {
		for _, allowed := range actions {
			require.True(t, allowed)
		}
	}
}
