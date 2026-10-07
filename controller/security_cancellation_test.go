package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCanceledEnrollmentDoesNotConsumeProofOrCreateCredential(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	operation := service.VerificationOperation{Scope: service.VerificationScopeTwoFASetup}
	proof := issueSecurityEnrollmentProof(t, identity, operation, service.VerificationMethodPassword)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/2fa/setup", strings.NewReader(`{}`)).WithContext(ctx)
	c.Request.Header.Set("X-Security-Proof", proof)
	c.Set("id", identity.UserID)
	c.Set("session_id", identity.SessionID)
	c.Set("auth_version", identity.UserAuthVersion)
	c.Set("session_version", identity.SessionVersion)
	Setup2FA(c)
	require.Contains(t, response.Body.String(), `"success":false`)
	var count int64
	require.NoError(t, model.DB.Model(&model.TwoFA{}).Where("user_id = ?", user.Id).Count(&count).Error)
	require.Zero(t, count)
	_, err := service.ConsumeOperationProof(proof, identity, operation)
	require.NoError(t, err, "request cancellation must leave the one-shot proof unused")

}
