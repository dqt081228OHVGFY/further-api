package relay

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	relaytypes "github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMidjourneySubmissionAndExistingJobChargeWalletOnlyOnce(t *testing.T) {
	db := setupRelayChannelDB(t)
	previousLog, previousRedis, previousBatch, previousConsume := model.LOG_DB, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled
	model.LOG_DB, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled = db, false, false, true
	t.Cleanup(func() {
		model.LOG_DB, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled = previousLog, previousRedis, previousBatch, previousConsume
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}, &model.Midjourney{}))
	owner := &model.User{Id: 97, Username: "mj-owner", Status: common.UserStatusEnabled, Group: "default", Quota: 1_000_000}
	token := &model.Token{Id: 97, UserId: 97, Key: "mj-submit-token", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1_000_000}
	require.NoError(t, db.Create(owner).Error)
	require.NoError(t, db.Create(token).Error)
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("mj-api-secret") != "merchant-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) == 1 {
			_, _ = w.Write([]byte("{\"code\":1,\"result\":\"job-97\",\"description\":\"accepted\"}"))
		} else {
			_, _ = w.Write([]byte("{\"code\":21,\"result\":\"job-97\",\"properties\":{\"status\":\"SUCCESS\",\"imageUrl\":\"https://images.example/result.png\"}}"))
		}
	}))
	defer upstream.Close()
	ch := &model.Channel{Id: 97, Name: "mj-local", Type: constant.ChannelTypeMidjourney, Status: common.ChannelStatusEnabled, Key: "merchant-key", BaseURL: &upstream.URL}
	require.NoError(t, db.Create(ch).Error)
	makeRequest := func() (*gin.Context, *relaycommon.RelayInfo, *httptest.ResponseRecorder) {
		writer := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(writer)
		c.Request = httptest.NewRequest(http.MethodPost, "/mj/submit/imagine", strings.NewReader("{\"prompt\":\"landscape\"}"))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("id", 97)
		common.SetContextKey(c, constant.ContextKeyChannelId, 97)
		common.SetContextKey(c, constant.ContextKeyChannelType, ch.Type)
		common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
		common.SetContextKey(c, constant.ContextKeyChannelKey, ch.Key)
		return c, &relaycommon.RelayInfo{UserId: 97, TokenId: 97, TokenKey: token.Key, UserQuota: owner.Quota,
			UserGroup: "default", UsingGroup: "default", StartTime: time.Now(), RelayMode: relayconstant.RelayModeMidjourneyImagine,
			RelayFormat: relaytypes.RelayFormatMjProxy, UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}}, writer
	}
	c, info, writer := makeRequest()
	require.Nil(t, RelayMidjourneySubmit(c, info))
	require.Equal(t, http.StatusOK, writer.Code)
	require.True(t, service.RequestPolicy(c).Successful)
	charge := info.PriceData.Quota
	require.Positive(t, charge)
	var savedOwner model.User
	require.NoError(t, db.First(&savedOwner, 97).Error)
	require.Equal(t, owner.Quota-charge, savedOwner.Quota)
	var savedToken model.Token
	require.NoError(t, db.First(&savedToken, 97).Error)
	require.Equal(t, token.RemainQuota-charge, savedToken.RemainQuota)
	second, secondInfo, response := makeRequest()
	require.Nil(t, RelayMidjourneySubmit(second, secondInfo))
	require.Contains(t, response.Body.String(), "\"code\":1")
	require.Eventually(t, func() bool {
		return db.First(&savedOwner, 97).Error == nil && db.First(&savedToken, 97).Error == nil &&
			savedOwner.Quota == owner.Quota-charge && savedToken.RemainQuota == token.RemainQuota-charge
	}, 2*time.Second, 10*time.Millisecond)
	var taskCount, logCount int64
	require.NoError(t, db.Model(&model.Midjourney{}).Count(&taskCount).Error)
	require.NoError(t, db.Model(&model.Log{}).Count(&logCount).Error)
	require.EqualValues(t, 1, taskCount)
	require.EqualValues(t, 1, logCount)
}
