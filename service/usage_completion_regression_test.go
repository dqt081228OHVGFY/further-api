package service

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/model"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRealtimeUsageReservationsAreNotChargedAgainAtCompletion(t *testing.T) {
	truncate(t)
	seedUser(t, 95, 10000)
	seedToken(t, 95, 95, "realtime-completion", 10000)
	previous := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString("{\"test-model\":1}"))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previous)) })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/realtime", nil)
	info := &relaycommon.RelayInfo{UserId: 95, TokenId: 95, TokenKey: "realtime-completion", OriginModelName: "test-model",
		UserGroup: "default", UsingGroup: "default", ForcePreConsume: true, UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}}
	session, failure := NewBillingSession(c, info, 80)
	require.Nil(t, failure)
	info.Billing = session
	usage := &dto.RealtimeUsage{TotalTokens: 100, InputTokens: 100, InputTokenDetails: dto.InputTokenDetails{TextTokens: 100}}
	require.NoError(t, PreWssConsumeQuota(c, info, usage))
	require.NoError(t, PreWssConsumeQuota(c, info, usage))
	require.Equal(t, 200, session.GetPreConsumedQuota())
	require.Equal(t, 9800, getUserQuota(t, 95))
	require.Equal(t, 9800, getTokenRemainQuota(t, 95))
	charged, err := SettleBillingWithFinalQuota(c, info, 200)
	require.NoError(t, err)
	require.Equal(t, 200, charged)
	require.Equal(t, 9800, getUserQuota(t, 95))
	require.Equal(t, 9800, getTokenRemainQuota(t, 95))
}

func TestInteractiveFailureObservationRefundsOnlyWinningTransition(t *testing.T) {
	truncate(t)
	seedUser(t, 96, 10000)
	seedToken(t, 96, 96, "observed-failure", 10000)
	seedChannel(t, 96)
	seedChargedAccounting(t, 96, 96, 96, 2000, 1)
	task := makeTask(96, 96, 2000, 96, BillingSourceWallet, 0)
	require.NoError(t, model.DB.Create(task).Error)
	stale := *task
	result := &relaycommon.TaskInfo{Status: model.TaskStatusFailure, Reason: "upstream rejected task"}
	require.NoError(t, CommitTaskObservationContext(context.Background(), task, result, nil, &mockAdaptor{}))
	require.NoError(t, CommitTaskObservationContext(context.Background(), &stale, result, nil, &mockAdaptor{}))
	require.Zero(t, getTaskQuota(t, task.ID))
	require.Equal(t, 12000, getUserQuota(t, 96))
	require.Equal(t, 12000, getTokenRemainQuota(t, 96))
	require.EqualValues(t, 1, countLogs(t))
	require.Equal(t, 0, stale.Quota)
}
