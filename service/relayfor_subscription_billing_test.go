package service

import (
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRelayForBillingSessionChargesActualDiscountAndReleasesFailedReservation(t *testing.T) {
	oldDB, oldLog := model.DB, model.LOG_DB
	oldMain, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldMaster, oldRedis := common.IsMasterNode, common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "billing.db")), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.IsMasterNode, common.RedisEnabled = false, false
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitLogDB())
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLog
		common.SetDatabaseTypes(oldMain, oldLogType)
		common.IsMasterNode, common.RedisEnabled = oldMaster, oldRedis
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}, &model.RelayForSubscriptionPolicy{}, &model.RelayForUserSubscriptionPolicy{}, &model.RelayForSubscriptionPreference{}, &model.RelayForDiscountReservation{}))
	user := model.User{Username: "billing-owner", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "relayfor-billing-test", RemainQuota: 1000, Status: common.TokenStatusEnabled, ExpiredTime: -1}
	require.NoError(t, db.Create(&token).Error)
	planID, err := model.SaveRelayForSubscriptionPlan(0, model.RelayForPlanInput{Code: "billing-half", Name: "Half", DurationDays: 30, Quota: 1000, DiscountMultiplier: 0.5, DiscountSavingsLimit: 10000, Status: 1, ConcurrentRequests: 1})
	require.NoError(t, err)
	var sub *model.UserSubscription
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var e error
		sub, e = model.GrantRelayForSubscriptionTx(tx, user.Id, planID, "test")
		return e
	}))
	newRequest := func(requestID string) (*gin.Context, *relaycommon.RelayInfo) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
		return c, &relaycommon.RelayInfo{UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, RequestId: requestID, OriginModelName: "test-model", UserSetting: dto.UserSetting{BillingPreference: "subscription_only"}}
	}
	c, info := newRequest("session-discount-one")
	session, apiErr := NewBillingSession(c, info, 80)
	require.Nil(t, apiErr)
	info.Billing = session
	assert.Equal(t, 40, session.GetPreConsumedQuota())
	require.NoError(t, session.Reserve(200))
	assert.Equal(t, 100, session.GetPreConsumedQuota())
	charged, err := SettleBillingWithFinalQuota(c, info, 160)
	require.NoError(t, err)
	assert.Equal(t, 80, charged)
	require.NoError(t, session.Settle(160))
	session.Refund(c)
	var storedToken model.Token
	require.NoError(t, db.First(&storedToken, token.Id).Error)
	assert.Equal(t, 920, storedToken.RemainQuota)
	var storedSub model.UserSubscription
	require.NoError(t, db.First(&storedSub, sub.Id).Error)
	assert.EqualValues(t, 80, storedSub.AmountUsed)
	c2, info2 := newRequest("session-discount-fail")
	failed, apiErr := NewBillingSession(c2, info2, 100)
	require.Nil(t, apiErr)
	failed.Refund(c2)
	failed.Refund(c2)
	require.Eventually(t, func() bool {
		var freshToken model.Token
		var freshSub model.UserSubscription
		var policy model.RelayForUserSubscriptionPolicy
		if db.First(&freshToken, token.Id).Error != nil || db.First(&freshSub, sub.Id).Error != nil || db.First(&policy, "subscription_id = ?", sub.Id).Error != nil {
			return false
		}
		return freshToken.RemainQuota == 920 && freshSub.AmountUsed == 80 && policy.DiscountSavingsReserved == 0
	}, 3*time.Second, 10*time.Millisecond)
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Update("remain_quota", 0).Error)
	c3, info3 := newRequest("session-token-rejected")
	_, apiErr = NewBillingSession(c3, info3, 100)
	require.NotNil(t, apiErr)
	var policy model.RelayForUserSubscriptionPolicy
	require.NoError(t, db.First(&policy, "subscription_id = ?", sub.Id).Error)
	assert.Zero(t, policy.DiscountSavingsReserved)
	assert.EqualValues(t, 80, policy.DiscountSavingsUsed)

	t.Run("discounted async submission rejects before token debit and releases savings", func(t *testing.T) {
		require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Update("remain_quota", 1000).Error)
		ctx, taskInfo := newRequest("session-async-rejected")
		taskInfo.ForcePreConsume = true
		_, taskErr := NewBillingSession(ctx, taskInfo, 100)
		require.NotNil(t, taskErr)
		assert.Contains(t, taskErr.Error(), "异步任务")
		require.NoError(t, db.First(&storedToken, token.Id).Error)
		assert.Equal(t, 1000, storedToken.RemainQuota)
		require.NoError(t, db.First(&storedSub, sub.Id).Error)
		assert.EqualValues(t, 80, storedSub.AmountUsed)
		require.NoError(t, db.First(&policy, "subscription_id = ?", sub.Id).Error)
		assert.Zero(t, policy.DiscountSavingsReserved)
		reservation, err := model.GetRelayForDiscountReservation(taskInfo.RequestId)
		require.NoError(t, err)
		assert.Equal(t, "refunded", reservation.Status)
	})

	t.Run("realtime accumulates discounted reservations with savings cap and zero wallet", func(t *testing.T) {
		previousRatios := ratio_setting.ModelRatio2JSONString()
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"test-model":1}`))
		t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousRatios)) })
		require.NoError(t, db.Model(&model.RelayForUserSubscriptionPolicy{}).Where("subscription_id = ?", sub.Id).Update("discount_savings_limit", 100).Error)
		ctx, realtimeInfo := newRequest("session-realtime-discount")
		realtimeInfo.UserGroup, realtimeInfo.UsingGroup = "default", "default"
		realtimeSession, realtimeErr := NewBillingSession(ctx, realtimeInfo, 80)
		require.Nil(t, realtimeErr)
		realtimeInfo.Billing = realtimeSession
		usage := &dto.RealtimeUsage{TotalTokens: 100, InputTokens: 100, InputTokenDetails: dto.InputTokenDetails{TextTokens: 100}}
		require.NoError(t, PreWssConsumeQuota(ctx, realtimeInfo, usage))
		require.NoError(t, PreWssConsumeQuota(ctx, realtimeInfo, usage))
		assert.Equal(t, 180, realtimeSession.GetPreConsumedQuota())
		charged, err := SettleBillingWithFinalQuota(ctx, realtimeInfo, 200)
		require.NoError(t, err)
		assert.Equal(t, 180, charged)
		require.NoError(t, realtimeSession.Settle(200))
		require.NoError(t, db.First(&storedToken, token.Id).Error)
		assert.Equal(t, 820, storedToken.RemainQuota)
		require.NoError(t, db.First(&storedSub, sub.Id).Error)
		assert.EqualValues(t, 260, storedSub.AmountUsed)
		require.NoError(t, db.First(&policy, "subscription_id = ?", sub.Id).Error)
		assert.Zero(t, policy.DiscountSavingsReserved)
		assert.EqualValues(t, 100, policy.DiscountSavingsUsed)
	})

	t.Run("native New API async subscription remains available", func(t *testing.T) {
		nativeUser := model.User{Username: "native-billing-owner", AffCode: "native-billing", Status: common.UserStatusEnabled}
		require.NoError(t, db.Create(&nativeUser).Error)
		nativeToken := model.Token{UserId: nativeUser.Id, Key: "native-billing-test", RemainQuota: 1000, Status: common.TokenStatusEnabled, ExpiredTime: -1}
		require.NoError(t, db.Create(&nativeToken).Error)
		nativePlan := model.SubscriptionPlan{Title: "Native New API", Enabled: true}
		require.NoError(t, db.Create(&nativePlan).Error)
		now := common.GetTimestamp()
		nativeSub := model.UserSubscription{UserId: nativeUser.Id, PlanId: nativePlan.Id, Status: "active", StartTime: now - 1, EndTime: now + 3600, AmountTotal: 1000}
		require.NoError(t, db.Create(&nativeSub).Error)
		ctx, nativeInfo := newRequest("session-native-async")
		nativeInfo.UserId, nativeInfo.TokenId, nativeInfo.TokenKey = nativeUser.Id, nativeToken.Id, nativeToken.Key
		nativeInfo.ForcePreConsume = true
		nativeSession, nativeErr := NewBillingSession(ctx, nativeInfo, 100)
		require.Nil(t, nativeErr)
		assert.Equal(t, 100, nativeSession.GetPreConsumedQuota())
		require.NoError(t, nativeSession.Settle(100))
		require.NoError(t, db.First(&nativeToken, nativeToken.Id).Error)
		assert.Equal(t, 900, nativeToken.RemainQuota)
		reservation, err := model.GetRelayForDiscountReservation(nativeInfo.RequestId)
		require.NoError(t, err)
		assert.Nil(t, reservation)
	})
}
