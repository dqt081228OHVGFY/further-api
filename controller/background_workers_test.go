package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestControllerSyncWorkersStopOnCancellation(t *testing.T) {
	for _, worker := range []struct {
		name string
		run  func()
	}{{"channel balance", func() { AutomaticallyUpdateChannels(60) }}, {"task plugins", SyncTaskPlugins}} {
		t.Run(worker.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			previous := common.BackgroundContext()
			common.SetBackgroundContext(ctx)
			defer common.SetBackgroundContext(previous)
			done := make(chan struct{})
			go func() { worker.run(); close(done) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("worker did not stop on cancellation")
			}
		})
	}
}

func TestChannelBalanceRequestStopsOnCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := getResponseBodyContext(ctx, http.MethodGet, server.URL, &model.Channel{}, nil)
		finished <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("balance HTTP request did not start")
	}
	cancel()
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("balance HTTP request ignored cancellation")
	}
}

func TestChannelBalanceSpacingStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() { done <- waitChannelBalanceInterval(ctx, time.Hour) }()
	cancel()
	select {
	case waited := <-done:
		require.False(t, waited)
	case <-time.After(time.Second):
		t.Fatal("channel spacing timer ignored cancellation")
	}
}

func TestCancelledScheduledPollingDoesNotReportSuccess(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	previous := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previous
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
	for _, handler := range []service.SystemTaskHandler{modelUpdateHandler{}, midjourneyPollHandler{}, asyncTaskPollHandler{}} {
		t.Run(handler.Type(), func(t *testing.T) {
			task, err := model.CreateSystemTask(handler.Type(), nil, nil)
			require.NoError(t, err)
			task, claimed, err := model.ClaimSystemTask(task.ID, task.Type, "shutdown-runner", time.Now().Add(time.Minute).Unix())
			require.NoError(t, err)
			require.True(t, claimed)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			handler.Run(ctx, task, "shutdown-runner")
			finished, err := model.GetSystemTaskByTaskID(task.TaskID)
			require.NoError(t, err)
			require.Equal(t, model.SystemTaskStatusFailed, finished.Status)
			require.Contains(t, finished.Error, context.Canceled.Error())
		})
	}
}
