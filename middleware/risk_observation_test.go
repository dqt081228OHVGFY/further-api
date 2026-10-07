package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectRiskObservationKeepsOnlyRequestMetadata(t *testing.T) {
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "risk-observation-test-secret"
	t.Cleanup(func() { common.CryptoSecret = previousSecret })

	body := `{"model":"gpt-4o","stream":true,"messages":[{"role":"system","content":"private system text"},{"role":"user","content":"ping"}]}`
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.RemoteAddr = "192.0.2.15:3456"
	storage, err := common.CreateBodyStorage([]byte(body))
	require.NoError(t, err)
	t.Cleanup(func() { _ = storage.Close() })
	c.Set(common.KeyBodyStorage, storage)
	c.Set(RouteTagKey, "relay")
	c.Set("id", 42)
	c.Set("token_id", 7)
	c.Writer.WriteHeader(http.StatusPaymentRequired)
	startedAt := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)

	observation, ok := collectRiskObservation(c, startedAt)
	require.True(t, ok)
	assert.Equal(t, 42, observation.UserID)
	assert.Equal(t, 7, observation.TokenID)
	assert.Equal(t, startedAt, observation.OccurredAt)
	assert.Equal(t, "gpt-4o", observation.Model)
	assert.True(t, observation.Stream)
	assert.Equal(t, http.StatusPaymentRequired, observation.StatusCode)
	assert.True(t, observation.QuotaExhausted)
	assert.True(t, observation.LivenessKeywordHit)
	assert.Regexp(t, `^[0-9a-f]{64}$`, observation.IPFingerprint)
	assert.Regexp(t, `^[0-9a-f]{64}$`, observation.InputSignature)
	assert.NotContains(t, observation.IPFingerprint, "192.0.2.15")
	assert.NotContains(t, observation.InputSignature, "ping")
}

type unreadRiskBody struct{}

func (unreadRiskBody) Read([]byte) (int, error) { panic("early rejection body must not be read") }
func (unreadRiskBody) Close() error             { return nil }

func TestCollectRiskObservationEarlyPaymentRequiredDoesNotReadBody(t *testing.T) {
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Body = unreadRiskBody{}
	c.Request.ContentLength = 1024
	c.Set(RouteTagKey, "relay")
	c.Set("id", 9)
	c.Writer.WriteHeader(http.StatusPaymentRequired)

	observation, ok := collectRiskObservation(c, time.Now())
	require.True(t, ok)
	assert.Equal(t, http.StatusPaymentRequired, observation.StatusCode)
	assert.Empty(t, observation.InputSignature)
	assert.False(t, observation.LivenessKeywordHit)
}

func TestCollectRiskObservationUsesEarlyChannelFailureMarker(t *testing.T) {
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(RouteTagKey, "relay")
	c.Set("id", 9)
	c.Set(riskModelContextKey, "requested-model")
	c.Set(riskChannelUnavailableContextKey, true)
	c.Writer.WriteHeader(http.StatusServiceUnavailable)

	observation, ok := collectRiskObservation(c, time.Now())
	require.True(t, ok)
	assert.Equal(t, "requested-model", observation.Model)
	assert.True(t, observation.ChannelUnavailable)
	assert.Equal(t, http.StatusServiceUnavailable, observation.StatusCode)
	assert.Empty(t, observation.InputSignature)
}

func TestRiskLivenessKeywordRequiresAProbeLikePrompt(t *testing.T) {
	assert.True(t, riskLivenessKeyword(" Ping? "))
	assert.True(t, riskLivenessKeyword("测活"))
	assert.False(t, riskLivenessKeyword("Please write a ping health check in Go"))
	assert.False(t, riskLivenessKeyword(""))
}

func riskContextWithBody(t *testing.T, userID int, body string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.RemoteAddr = "192.0.2.15:3456"
	storage, err := common.CreateBodyStorage([]byte(body))
	require.NoError(t, err)
	t.Cleanup(func() { _ = storage.Close() })
	c.Set(common.KeyBodyStorage, storage)
	c.Set(RouteTagKey, "relay")
	c.Set("id", userID)
	return c
}

func TestRiskInputSignatureCoversFullRequestAndIsScopedToUser(t *testing.T) {
	firstBody := `{"model":"gpt-4o","messages":[{"role":"system","content":"translate"},{"role":"user","content":"ping"}]}`
	differentHistory := `{"model":"gpt-4o","messages":[{"role":"system","content":"explain"},{"role":"user","content":"ping"}]}`
	first, ok := collectRiskObservation(riskContextWithBody(t, 42, firstBody), time.Now())
	require.True(t, ok)
	same, _ := collectRiskObservation(riskContextWithBody(t, 42, firstBody), time.Now())
	different, _ := collectRiskObservation(riskContextWithBody(t, 42, differentHistory), time.Now())
	otherUser, _ := collectRiskObservation(riskContextWithBody(t, 43, firstBody), time.Now())
	assert.Equal(t, first.InputSignature, same.InputSignature)
	assert.Equal(t, first.IPFingerprint, same.IPFingerprint)
	assert.NotEqual(t, first.InputSignature, different.InputSignature)
	assert.NotEqual(t, first.InputSignature, otherUser.InputSignature)
	assert.NotEqual(t, first.IPFingerprint, otherUser.IPFingerprint)
}

func TestRiskLargeRequestRetainsTokenFallbackWithoutReadingBodyAgain(t *testing.T) {
	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"` + strings.Repeat("x", riskMaxBodyBytes) + `"}]}`
	c := riskContextWithBody(t, 42, body)
	common.SetContextKey(c, constant.ContextKeyPromptTokens, 12345)
	c.Set(riskModelContextKey, "gpt-4o")
	observation, ok := collectRiskObservation(c, time.Now())
	require.True(t, ok)
	assert.Equal(t, 12345, observation.PromptTokens)
	assert.Equal(t, "gpt-4o", observation.Model)
	assert.Empty(t, observation.InputSignature)
	assert.False(t, observation.LivenessKeywordHit)
}

func TestRiskObservationSkipsModelDiscoveryAndPolling(t *testing.T) {
	for _, path := range []string{"/v1/models", "/v1/responses/response-id", "/v1/videos/task-id"} {
		t.Run(path, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, path, nil)
			c.Set(RouteTagKey, "relay")
			c.Set("id", 42)
			_, ok := collectRiskObservation(c, time.Now())
			assert.False(t, ok)
		})
	}
}

func TestRiskObservationCapturesChannelExhaustionAfterRetry(t *testing.T) {
	c := riskContextWithBody(t, 42, `{"model":"gpt-4o","input":"hello"}`)
	c.Writer.WriteHeader(http.StatusServiceUnavailable)
	service.RecordRequestPolicyTermination(c, types.NewErrorWithStatusCode(
		errors.New("no remaining channel"), types.ErrorCodeGetChannelFailed, http.StatusServiceUnavailable,
	))
	observation, ok := collectRiskObservation(c, time.Now())
	require.True(t, ok)
	assert.True(t, observation.ChannelUnavailable)
}

func TestRiskObservationWebsocketIsStreaming(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime?model=gpt-realtime", nil)
	c.Request.Header.Set("Upgrade", "websocket")
	c.Set(RouteTagKey, "relay")
	c.Set("id", 42)
	c.Set(riskModelContextKey, "gpt-realtime")
	c.Writer.WriteHeader(http.StatusSwitchingProtocols)
	observation, ok := collectRiskObservation(c, time.Now())
	require.True(t, ok)
	assert.True(t, observation.Stream)
}

func TestRiskObservationRecognizesExplicitQuotaFailureOnly(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		code      types.ErrorCode
		exhausted bool
	}{
		{"wallet exhausted", types.ErrorCodeInsufficientUserQuota, true},
		{"permission failure", types.ErrorCodeAccessDenied, false},
		{"generic token reservation failure", types.ErrorCodePreConsumeTokenQuotaFailed, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			c := riskContextWithBody(t, 42, `{"model":"gpt-4o","input":"hello"}`)
			c.Writer.WriteHeader(http.StatusForbidden)
			service.RecordRequestPolicyTermination(c, types.NewErrorWithStatusCode(
				errors.New("request rejected"), testCase.code, http.StatusForbidden,
			))
			observation, ok := collectRiskObservation(c, time.Now())
			require.True(t, ok)
			assert.Equal(t, testCase.exhausted, observation.QuotaExhausted)
		})
	}
}

func TestRiskObservationTokenQuotaRejectionPreservesAuthentication(t *testing.T) {
	setupDashboardAuthMiddlewareTest(t)
	previousMaster, previousLogType := common.IsMasterNode, common.LogDatabaseType()
	common.IsMasterNode = false
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitLogDB()) // initializes the database-specific quoted key column
	t.Cleanup(func() {
		common.IsMasterNode = previousMaster
		common.SetLogDatabaseType(previousLogType)
	})
	require.NoError(t, model.DB.AutoMigrate(&model.Token{}))
	for _, testCase := range []struct {
		name      string
		status    int
		expiredAt int64
		exhausted bool
	}{
		{"empty", common.TokenStatusEnabled, -1, true},
		{"exhausted", common.TokenStatusExhausted, -1, true},
		{"disabled", common.TokenStatusDisabled, -1, false},
		{"expired", common.TokenStatusEnabled, time.Now().Add(-time.Hour).Unix(), false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			token := &model.Token{UserId: 42, Key: "risk" + testCase.name + "key",
				Status: testCase.status, ExpiredTime: testCase.expiredAt, RemainQuota: 0}
			require.NoError(t, model.DB.Create(token).Error)
			var observation service.RiskObservationInput
			var observed bool
			engine := gin.New()
			engine.POST("/v1/chat/completions", RouteTag("relay"), func(c *gin.Context) {
				c.Next()
				observation, observed = collectRiskObservation(c, time.Now())
			}, TokenAuth(), func(*gin.Context) { t.Error("rejected token reached relay") })
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			request.Body = unreadRiskBody{}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer sk-"+token.Key)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			require.Equal(t, http.StatusUnauthorized, response.Code)
			require.True(t, observed)
			assert.Equal(t, token.Id, observation.TokenID)
			assert.Equal(t, token.UserId, observation.UserID)
			assert.Equal(t, testCase.exhausted, observation.QuotaExhausted)
			assert.Empty(t, observation.InputSignature)
		})
	}
}
