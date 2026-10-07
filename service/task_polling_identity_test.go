package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/stretchr/testify/require"
)

type channelIdentityPollingAdaptor struct {
	taskPollingFetchAdaptor
	channelID int
	seen      *[][2]int
	mutex     *sync.Mutex
}

func (a *channelIdentityPollingAdaptor) Init(info *relaycommon.RelayInfo) {
	a.channelID = info.ChannelId
}
func (a *channelIdentityPollingAdaptor) FetchTask(_ string, _ string, task *model.Task, _ string) (*http.Response, error) {
	a.mutex.Lock()
	*a.seen = append(*a.seen, [2]int{a.channelID, task.ChannelId})
	a.mutex.Unlock()
	body := fmt.Sprintf(`{"code":"success","data":{"task_id":"shared-upstream-id","status":"IN_PROGRESS","progress":"%d%%"}}`, a.channelID-83980)
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestPollingIsolatesSameUpstreamIDAcrossChannelsAndOwners(t *testing.T) {
	truncate(t)
	seedTaskPollingChannel(t, 84011, true)
	seedTaskPollingChannel(t, 84012, true)
	first := seedPollingTask(t, 84011, "poll-owner-a", "shared-upstream-id")
	second := seedPollingTask(t, 84012, "poll-owner-b", "shared-upstream-id")
	second.UserId = 2
	require.NoError(t, model.DB.Save(second).Error)
	previous, previousLimit := GetTaskAdaptorFunc, constant.TaskQueryLimit
	constant.TaskQueryLimit = 100
	var seen [][2]int
	var mutex sync.Mutex
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor {
		return &channelIdentityPollingAdaptor{seen: &seen, mutex: &mutex}
	}
	t.Cleanup(func() { GetTaskAdaptorFunc = previous; constant.TaskQueryLimit = previousLimit })
	summary := RunTaskPollingOnce(context.Background(), nil)
	require.Equal(t, 2, summary.UnfinishedTasks)
	require.ElementsMatch(t, [][2]int{{84011, 84011}, {84012, 84012}}, seen, "never use another account's task with this channel's credentials")
	for _, task := range []*model.Task{first, second} {
		var saved model.Task
		require.NoError(t, model.DB.First(&saved, task.ID).Error)
		require.Equal(t, task.UserId, saved.UserId)
		require.Equal(t, task.ChannelId, saved.ChannelId)
		require.Equal(t, fmt.Sprintf("%d%%", task.ChannelId-83980), saved.Progress)
	}
}
