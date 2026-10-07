package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestReadOnlyTokenQueriesRespectIPRestrictionsInEveryAllowedState(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "")
	setupDashboardAuthMiddlewareTest(t)
	previousMaster, previousLogType := common.IsMasterNode, common.LogDatabaseType()
	common.IsMasterNode = false
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitLogDB())
	t.Cleanup(func() { common.IsMasterNode = previousMaster; common.SetLogDatabaseType(previousLogType) })
	require.NoError(t, model.DB.AutoMigrate(&model.Token{}))
	user := createMiddlewarePATUser(t, "readonly-owner", "readonly-owner-pat")
	allowed := "198.51.100.0/24"
	for _, status := range []int{common.TokenStatusEnabled, common.TokenStatusExpired, common.TokenStatusExhausted} {
		t.Run(fmt.Sprintf("status=%d", status), func(t *testing.T) {
			key := fmt.Sprintf("readonlykeyfixture%d", status)
			token := model.Token{UserId: user.Id, Key: key, Name: "read-only fixture", Status: status, ExpiredTime: time.Now().Unix() - 1, AllowIps: &allowed}
			require.NoError(t, model.DB.Create(&token).Error)
			engine := gin.New()
			require.NoError(t, ConfigureTrustedProxies(engine))
			engine.GET("/usage", TokenAuthReadOnly(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
			for _, peer := range []string{"198.51.100.42:1234", "203.0.113.17:1234", "10.2.3.4:1234", "172.20.0.5:1234", "192.168.1.5:1234", "[fd12:3456::2]:1234", "127.0.0.2:1234"} {
				request := httptest.NewRequest(http.MethodGet, "/usage", nil)
				request.RemoteAddr = peer
				request.Header.Set("Authorization", "Bearer sk-"+key)
				request.Header.Set("X-Forwarded-For", "198.51.100.42")
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				if peer == "198.51.100.42:1234" {
					require.Equal(t, http.StatusNoContent, response.Code)
				} else {
					require.Equal(t, http.StatusForbidden, response.Code)
					require.Contains(t, response.Body.String(), "TOKEN_IP_FORBIDDEN")
				}
			}
		})
	}
}

func TestGeminiQueryCredentialIsRemovedBeforeHandlersAndPreservesHeaderPriority(t *testing.T) {
	setupDashboardAuthMiddlewareTest(t)
	previousMaster, previousLogType := common.IsMasterNode, common.LogDatabaseType()
	common.IsMasterNode = false
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitLogDB())
	t.Cleanup(func() { common.IsMasterNode = previousMaster; common.SetLogDatabaseType(previousLogType) })
	require.NoError(t, model.DB.AutoMigrate(&model.Token{}))
	user := createMiddlewarePATUser(t, "gemini-key-owner", "gemini-owner-pat")
	queryKey, headerKey := "geminiqueryfixturekey", "geminiheaderfixturekey"
	for _, key := range []string{queryKey, headerKey} {
		require.NoError(t, model.DB.Create(&model.Token{UserId: user.Id, Key: key, Name: "gemini fixture", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
	}
	for _, explicitHeader := range []string{"", "sk-" + headerKey} {
		engine := gin.New()
		engine.GET("/v1/models", TokenAuth(), func(c *gin.Context) {
			require.Empty(t, c.Request.URL.Query().Get("key"))
			require.NotContains(t, c.Request.RequestURI, queryKey)
			require.Equal(t, "keep", c.Request.URL.Query().Get("page"))
			if explicitHeader == "" {
				require.Equal(t, "sk-"+queryKey, c.GetHeader("x-goog-api-key"))
				require.Equal(t, queryKey, c.GetString("token_key"))
			} else {
				require.Equal(t, explicitHeader, c.GetHeader("x-goog-api-key"))
				require.Equal(t, headerKey, c.GetString("token_key"))
			}
			c.Status(http.StatusNoContent)
		})
		request := httptest.NewRequest(http.MethodGet, "/v1/models?key=sk-"+queryKey+"&page=keep", nil)
		if explicitHeader != "" {
			request.Header.Set("x-goog-api-key", explicitHeader)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		require.Equal(t, http.StatusNoContent, response.Code)
	}
}
