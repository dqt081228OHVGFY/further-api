package controller

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestConcurrentRealPasswordLoginsCannotOverrunSessionCaps(t *testing.T) {
	for _, cap := range []string{"active", "issuance"} {
		t.Run(cap, func(t *testing.T) {
			user, _ := setupSecurityEnrollmentTest(t)
			require.NoError(t, model.DB.Where("user_id = ?", user.Id).Delete(&model.UserSession{}).Error)
			connection, err := model.DB.DB()
			require.NoError(t, err)
			connection.SetMaxOpenConns(1)
			oldActive, oldIssued, oldWindow, oldPassword := common.UserSessionActiveLimit, common.UserSessionIssuanceLimit, common.UserSessionIssuanceWindowSeconds, common.PasswordLoginEnabled
			common.UserSessionActiveLimit, common.UserSessionIssuanceLimit, common.UserSessionIssuanceWindowSeconds, common.PasswordLoginEnabled = 100, 100, 3600, true
			if cap == "active" {
				common.UserSessionActiveLimit = 1
			} else {
				common.UserSessionIssuanceLimit = 1
			}
			t.Cleanup(func() {
				common.UserSessionActiveLimit, common.UserSessionIssuanceLimit, common.UserSessionIssuanceWindowSeconds, common.PasswordLoginEnabled = oldActive, oldIssued, oldWindow, oldPassword
			})
			// Force the vulnerable check-before-transaction interleaving if it is
			// reintroduced. Atomic counting inside a transaction never waits here.
			var reads atomic.Int32
			gate := make(chan struct{})
			openGate := sync.OnceFunc(func() { close(gate) })
			defer openGate()
			callback := "test:non-transactional-session-count-race"
			require.NoError(t, model.DB.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
				_, transactional := tx.Statement.ConnPool.(*sql.Tx)
				if !transactional && tx.Statement.Table == "user_sessions" && strings.Contains(tx.Statement.SQL.String(), "count(") && strings.Contains(tx.Statement.SQL.String(), "status =") {
					if reads.Add(1) == 2 {
						openGate()
					}
					<-gate
				}
			}))
			t.Cleanup(func() { _ = model.DB.Callback().Query().Remove(callback) })
			engine := gin.New()
			engine.POST("/api/user/login", middleware.SessionCookieOriginGuard(), Login)
			server := httptest.NewServer(engine)
			t.Cleanup(server.Close)
			type loginResult struct {
				status  int
				success bool
				code    string
				err     error
			}
			results := make(chan loginResult, 2)
			start := make(chan struct{})
			for index := range 2 {
				go func(index int) {
					<-start
					request, err := http.NewRequest(http.MethodPost, server.URL+"/api/user/login", strings.NewReader(fmt.Sprintf(`{"username":"enrollment-user","password":"enrollment-password","request_nonce":%d}`, index)))
					if err != nil {
						results <- loginResult{err: err}
						return
					}
					request.Header.Set("Content-Type", "application/json")
					request.Header.Set("Origin", server.URL)
					response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
					if err != nil {
						results <- loginResult{err: err}
						return
					}
					defer response.Body.Close()
					body, err := io.ReadAll(response.Body)
					var data struct {
						Success bool   `json:"success"`
						Code    string `json:"code"`
					}
					if err == nil {
						err = json.Unmarshal(body, &data)
					}
					results <- loginResult{response.StatusCode, data.Success, data.Code, err}
				}(index)
			}
			close(start)
			accepted := 0
			for range 2 {
				result := <-results
				require.NoError(t, result.err)
				if result.success {
					require.Equal(t, http.StatusOK, result.status)
					accepted++
				} else {
					if cap == "active" {
						require.Equal(t, "AUTH_SESSION_LIMIT", result.code)
					} else {
						require.Equal(t, "AUTH_SESSION_ISSUANCE_LIMIT", result.code)
					}
				}
			}
			require.Equal(t, 1, accepted)
			var count int64
			require.NoError(t, model.DB.Model(&model.UserSession{}).Where("user_id = ?", user.Id).Count(&count).Error)
			require.EqualValues(t, 1, count)
		})
	}
}
