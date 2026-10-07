package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPeriodicTaskStopsBetweenIterations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		runPeriodicTask(ctx, time.Hour, true, func(context.Context) { close(started) })
		close(stopped)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("initial maintenance pass did not run")
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("maintenance worker did not stop before its next tick")
	}
}

func TestPeriodicTaskSkipsWorkAfterShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runPeriodicTask(ctx, time.Hour, true, func(context.Context) { t.Fatal("work ran after shutdown") })
	require.False(t, startPeriodicTask("invalid timer", 0, true, func(context.Context) {}))
}
