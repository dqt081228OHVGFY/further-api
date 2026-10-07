package relay

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	relaytypes "github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting/ratio_setting"
	"github.com/dqt081228OHVGFY/further-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func realtimeCompletionFixture(t *testing.T) (*gin.Context, *relaycommon.RelayInfo, *service.BillingSession) {
	t.Helper()
	db := setupRelayChannelDB(t)
	previousLog, previousRedis, previousBatch, previousConsume := model.LOG_DB, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled
	previousRatios := ratio_setting.ModelRatio2JSONString()
	model.LOG_DB, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled = db, false, false, true
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString("{\"realtime-bridge\":1}"))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		require.NoError(t, common.WaitBackgroundTasks(ctx))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousRatios))
		model.LOG_DB, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled = previousLog, previousRedis, previousBatch, previousConsume
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}))
	require.NoError(t, db.Create(&model.User{Id: 99, Username: "realtime-owner", Quota: 10000, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 99, UserId: 99, Key: "realtime-bridge-key", RemainQuota: 10000, ExpiredTime: -1, Status: common.TokenStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 99, Name: "realtime-local", Type: 1, Status: common.ChannelStatusEnabled}).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
	info := &relaycommon.RelayInfo{UserId: 99, TokenId: 99, TokenKey: "realtime-bridge-key", UserQuota: 10000,
		UserGroup: "default", UsingGroup: "default", StartTime: time.Now(), OriginModelName: "realtime-bridge", ForcePreConsume: true,
		UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 99},
		PriceData: types.PriceData{ModelRatio: 1, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}}
	session, failure := service.NewBillingSession(c, info, 80)
	require.Nil(t, failure)
	info.Billing = session
	return c, info, session
}

func TestRealtimeErrorRetainsConsumedRoundSettlementExactlyOnce(t *testing.T) {
	c, info, session := realtimeCompletionFixture(t)
	usage := &dto.RealtimeUsage{InputTokens: 100, TotalTokens: 100, InputTokenDetails: dto.InputTokenDetails{TextTokens: 100}}
	require.NoError(t, service.PreWssConsumeQuota(c, info, usage))
	failure := relaytypes.NewErrorWithStatusCode(errors.New("upstream disconnected after completed response"), relaytypes.ErrorCodeBadResponse, 502)
	require.Same(t, failure, finishRealtimeAttempt(c, info, usage, failure))
	require.Same(t, failure, finishRealtimeAttempt(c, info, usage, failure))
	require.False(t, session.NeedsRefund())
	quota, known := session.FinalQuota()
	require.True(t, known)
	require.Equal(t, 100, quota)
	var owner model.User
	var token model.Token
	require.NoError(t, model.DB.First(&owner, 99).Error)
	require.NoError(t, model.DB.First(&token, 99).Error)
	require.Equal(t, 9900, owner.Quota)
	require.Equal(t, 9900, token.RemainQuota)
	require.Equal(t, 100, owner.UsedQuota)
	require.Equal(t, 1, owner.RequestCount)
	var logs int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Count(&logs).Error)
	require.EqualValues(t, 1, logs)
}

func TestRealtimeErrorWithoutConsumedUsageKeepsFullRefundAvailable(t *testing.T) {
	c, info, session := realtimeCompletionFixture(t)
	failure := relaytypes.NewErrorWithStatusCode(errors.New("upstream disconnected before any usage"), relaytypes.ErrorCodeBadResponse, 502)
	require.Same(t, failure, finishRealtimeAttempt(c, info, &dto.RealtimeUsage{}, failure))
	require.True(t, session.NeedsRefund())
	session.Refund(c)
	require.Eventually(t, func() bool {
		var owner model.User
		var token model.Token
		return model.DB.First(&owner, 99).Error == nil && model.DB.First(&token, 99).Error == nil && owner.Quota == 10000 && token.RemainQuota == 10000
	}, 2*time.Second, 10*time.Millisecond)
}
