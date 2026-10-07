package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func assertWalletBalanceEventually(t *testing.T, userID, want int) {
	t.Helper()
	require.Eventually(t, func() bool {
		quota, err := model.GetUserQuota(userID, false)
		return err == nil && quota == want
	}, 2*time.Second, 10*time.Millisecond)
}

func TestWalletFundingRefundIncludesEverySuccessfulReservationOnce(t *testing.T) {
	db := setupRiskAssessmentDatabase(t)
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedis })
	require.NoError(t, db.Create(&model.User{Id: 11, Username: "wallet", Quota: 1000}).Error)
	funding := &WalletFunding{userId: 11}
	require.NoError(t, funding.PreConsume(100))
	require.NoError(t, funding.PreConsume(200))
	quota, err := model.GetUserQuota(11, false)
	require.NoError(t, err)
	require.Equal(t, 700, quota)
	require.NoError(t, funding.Refund())
	require.NoError(t, funding.Refund())
	quota, err = model.GetUserQuota(11, false)
	require.NoError(t, err)
	require.Equal(t, 1000, quota)
	require.ErrorIs(t, funding.PreConsume(100), ErrFundingFinalized)
	require.ErrorIs(t, funding.Settle(100), ErrFundingFinalized)
}

func TestImageReservationRefundCannotChargeAfterFailure(t *testing.T) {
	db := setupRiskAssessmentDatabase(t)
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedis })
	require.NoError(t, db.Create(&model.User{Id: 12, Username: "image", Quota: 1000}).Error)
	funding := &WalletFunding{userId: 12}
	require.NoError(t, funding.PreConsume(100))
	info := &relaycommon.RelayInfo{UserId: 12, IsPlayground: true, ImageRequestCount: 2}
	session := &BillingSession{relayInfo: info, funding: funding, preConsumedQuota: 100, tokenConsumed: 100}
	require.NoError(t, session.Reserve(250))
	require.NoError(t, session.Reserve(400))
	require.Equal(t, 400, session.GetPreConsumedQuota())
	session.Refund(nil)
	session.Refund(nil)
	require.ErrorIs(t, session.Settle(300), ErrFundingFinalized)
	require.ErrorIs(t, session.Reserve(500), ErrFundingFinalized)
	assertWalletBalanceEventually(t, 12, 1000)
}

func TestInvalidActualQuotaCannotCreditMoreThanReservation(t *testing.T) {
	db := setupRiskAssessmentDatabase(t)
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedis })
	require.NoError(t, db.Create(&model.User{Id: 13, Username: "invalid-charge", Quota: 1000}).Error)
	funding := &WalletFunding{userId: 13}
	require.NoError(t, funding.PreConsume(100))
	session := &BillingSession{relayInfo: &relaycommon.RelayInfo{UserId: 13, IsPlayground: true}, funding: funding, preConsumedQuota: 100}
	require.ErrorIs(t, session.Settle(-50), ErrFundingAmountInvalid)
	require.True(t, session.NeedsRefund())
	quota, err := model.GetUserQuota(13, false)
	require.NoError(t, err)
	require.Equal(t, 900, quota)
	session.Refund(nil)
	assertWalletBalanceEventually(t, 13, 1000)
}

func TestBillingAdmissionStopsCanceledRequestsBeforeDatabaseAccess(t *testing.T) {
	previous := model.DB
	model.DB = nil
	t.Cleanup(func() { model.DB = previous })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	_, err := NewBillingSession(c, &relaycommon.RelayInfo{UserId: 1}, 100)
	require.ErrorIs(t, err, context.Canceled)
	_, err = NewBillingSession(nil, &relaycommon.RelayInfo{UserId: 1}, -1)
	require.ErrorIs(t, err, ErrFundingAmountInvalid)
}

func TestSubscriptionFailureReleasesSavingsWithoutTokenDebit(t *testing.T) {
	db := setupRiskAssessmentDatabase(t)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.UserSubscription{},
		&model.SubscriptionPreConsumeRecord{}, &model.FurtherSubscriptionPolicy{},
		&model.FurtherUserSubscriptionPolicy{}, &model.FurtherSubscriptionPreference{}, &model.FurtherDiscountReservation{}))
	require.NoError(t, db.Create(&model.User{Id: 17, Username: "zero-charge"}).Error)
	planID, err := model.SaveFurtherSubscriptionPlan(0, model.FurtherPlanInput{
		Code: "small-charge", Name: "Small charge", DurationDays: 30, Quota: 1000,
		DiscountMultiplier: 0.1, DiscountSavingsLimit: 1000, Status: 1,
	})
	require.NoError(t, err)
	var subscription *model.UserSubscription
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		subscription, err = model.GrantFurtherSubscriptionTx(tx, 17, planID, "test")
		return err
	}))
	funding := &SubscriptionFunding{requestId: "zero-charge-failure", userId: 17, modelName: "test", amount: 10}
	require.NoError(t, funding.PreConsume(10))
	require.EqualValues(t, 1, funding.preConsumed)
	session := &BillingSession{funding: funding, preConsumedQuota: int(funding.preConsumed), relayInfo: &relaycommon.RelayInfo{UserId: 17, IsPlayground: true, SubscriptionId: subscription.Id}}
	require.True(t, session.NeedsRefund())
	session.Refund(nil)
	require.Eventually(t, func() bool {
		reservation, err := model.GetFurtherDiscountReservation("zero-charge-failure")
		return err == nil && reservation != nil && reservation.Status == "refunded"
	}, 2*time.Second, 10*time.Millisecond)
	var policy model.FurtherUserSubscriptionPolicy
	require.NoError(t, db.First(&policy, "subscription_id = ?", subscription.Id).Error)
	require.Zero(t, policy.DiscountSavingsReserved)
}
