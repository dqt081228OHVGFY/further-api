package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/task/taskcommon"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
)

// Interactive refresh and background polling use the same terminal barrier.
// Only the winning persisted transition may publish task metrics and billing.
func CommitTaskObservationContext(ctx context.Context, task *model.Task, result *relaycommon.TaskInfo, body []byte, adapter TaskPollingAdaptor) error {
	return commitTaskObservation(ctx, task, result, body, adapter, nil)
}

func commitTaskObservation(ctx context.Context, task *model.Task, result *relaycommon.TaskInfo, body []byte, adapter TaskPollingAdaptor, prepare func()) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if task == nil || result == nil || adapter == nil {
		return errors.New("任务状态观察上下文无效。")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	status := model.TaskStatus(result.Status)
	if !knownPollStatus(status) {
		return errors.New("上游任务状态无效。")
	}
	before := task.Snapshot()
	if before.Status == model.TaskStatusSuccess || before.Status == model.TaskStatusFailure {
		return nil
	}
	if prepare != nil {
		prepare()
	}
	task.Status, task.Progress = status, result.Progress
	if task.Progress == "" {
		switch status {
		case model.TaskStatusSubmitted:
			task.Progress = taskcommon.ProgressSubmitted
		case model.TaskStatusQueued:
			task.Progress = taskcommon.ProgressQueued
		case model.TaskStatusInProgress:
			task.Progress = taskcommon.ProgressInProgress
		default:
			task.Progress = taskcommon.ProgressComplete
		}
	}
	if status == model.TaskStatusInProgress && task.StartTime == 0 {
		task.StartTime = time.Now().Unix()
	}
	if len(body) > 0 {
		task.Data = redactVideoResponseBody(body)
	}
	if len(result.PluginState) > 0 {
		task.PrivateData.PluginState = append([]byte(nil), result.PluginState...)
	}
	if result.Url != "" && !strings.HasPrefix(result.Url, "data:") {
		task.PrivateData.ResultURL = result.Url
	} else if status == model.TaskStatusSuccess {
		task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
	}
	terminal := status == model.TaskStatusSuccess || status == model.TaskStatusFailure
	if terminal {
		if task.FinishTime == 0 {
			task.FinishTime = time.Now().Unix()
		}
		task.FailReason = result.Reason
	} else {
		task.PrivateData.PollFailures = 0
	}
	if before.Equal(task.Snapshot()) {
		return nil
	}
	winner, err := task.UpdateWithStatusContext(ctx, before.Status)
	if err != nil {
		return err
	}
	if !winner {
		latest, exists, err := model.GetByTaskIdContext(ctx, task.UserId, task.TaskID)
		if err == nil && exists {
			*task = *latest
		}
		return err
	}
	if terminal {
		// Once the terminal row commits, client disconnection must not cancel
		// the corresponding accounting cleanup.
		settlement, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		finalizeTerminalTask(settlement, adapter, task, result)
	}
	return nil
}

func fetchPollingTask(ctx context.Context, adapter TaskPollingAdaptor, base, key string, task *model.Task, proxy string) (*http.Response, error) {
	if bound, ok := adapter.(interface {
		FetchTaskContext(context.Context, string, string, *model.Task, string) (*http.Response, error)
	}); ok {
		return bound.FetchTaskContext(ctx, base, key, task, proxy)
	}
	return adapter.FetchTask(base, key, task, proxy)
}

func fetchPollingBatch(ctx context.Context, adapter BatchTaskPollingAdaptor, base, key string, tasks []*model.Task, proxy string) (*http.Response, error) {
	if bound, ok := adapter.(interface {
		FetchBatchTasksContext(context.Context, string, string, []*model.Task, string) (*http.Response, error)
	}); ok {
		return bound.FetchBatchTasksContext(ctx, base, key, tasks, proxy)
	}
	return adapter.FetchBatchTasks(base, key, tasks, proxy)
}

func readTaskPollBody(response *http.Response) ([]byte, error) { return ReadTaskPollBody(response) }

func ReadTaskPollBody(response *http.Response) ([]byte, error) {
	if response == nil || response.Body == nil {
		return nil, errors.New("任务轮询响应内容缺失。")
	}
	limit := TaskPollResponseLimitBytes()
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err == nil && int64(len(body)) > limit {
		err = errors.New("任务轮询响应超过容量限制。")
	}
	return body, err
}
