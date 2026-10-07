package common

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeConfigPreservesStreamsAndRejectsInvalidProcessSettings(t *testing.T) {
	config, err := readRuntimeConfig(func(string) string { return "" }, 3000)
	require.NoError(t, err)
	assert.Equal(t, ":3000", config.Address)
	assert.Equal(t, 15*time.Second, config.ReadHeaderTimeout)
	assert.Equal(t, 120*time.Second, config.ShutdownTimeout)
	for _, values := range []map[string]string{
		{"PORT": "70000"}, {"PORT": "invalid"}, {"BIND_HOST": "host:3000"},
		{"SHUTDOWN_TIMEOUT_SECONDS": "0"}, {"HTTP_READ_HEADER_TIMEOUT_SECONDS": "-1"},
		{"HTTP_IDLE_TIMEOUT_SECONDS": "9223372036854775807"},
		{"HTTP_MAX_HEADER_BYTES": "1"}, {"BATCH_UPDATE_ENABLED": "invalid"},
		{"CHANNEL_UPDATE_FREQUENCY": "0"},
	} {
		_, err := readRuntimeConfig(func(name string) string { return values[name] }, 3000)
		assert.Error(t, err, values)
	}
	values := map[string]string{"BIND_HOST": "::1", "PORT": "3001", "HTTP_READ_HEADER_TIMEOUT_SECONDS": "0"}
	config, err = readRuntimeConfig(func(name string) string { return values[name] }, 3000)
	require.NoError(t, err)
	assert.Equal(t, "[::1]:3001", config.Address)
	assert.Zero(t, config.ReadHeaderTimeout)
}

func TestBackgroundLifecycleWaitsForFinalWritesAndRejectsLateStarts(t *testing.T) {
	previous := BackgroundContext()
	ctx, cancel := context.WithCancel(context.Background())
	SetBackgroundContext(ctx)
	release, started, wrote := make(chan struct{}), make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		cancel()
		SetBackgroundContext(previous)
	})
	require.True(t, StartBackgroundTask("test final write", func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		<-release
		close(wrote)
	}))
	<-started
	cancel()
	assert.False(t, StartBackgroundTask("late task", func(context.Context) { t.Error("late task ran") }))
	deadline, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer stop()
	assert.ErrorIs(t, WaitBackgroundTasks(deadline), context.DeadlineExceeded)
	close(release)
	wait, finish := context.WithTimeout(context.Background(), time.Second)
	defer finish()
	require.NoError(t, WaitBackgroundTasks(wait))
	select {
	case <-wrote:
	default:
		t.Fatal("wait returned before worker final write")
	}
}

func TestBackgroundLifecyclePanicReleasesWorker(t *testing.T) {
	previous := BackgroundContext()
	ctx, cancel := context.WithCancel(context.Background())
	SetBackgroundContext(ctx)
	t.Cleanup(func() { cancel(); SetBackgroundContext(previous) })
	require.True(t, StartBackgroundTask("panic test", func(context.Context) { panic(errors.New("test")) }))
	cancel()
	wait, finish := context.WithTimeout(context.Background(), time.Second)
	defer finish()
	assert.NoError(t, WaitBackgroundTasks(wait))
}
