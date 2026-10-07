package service

import (
	"context"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
)

func startPeriodicTask(name string, interval time.Duration, immediate bool, task func(context.Context)) bool {
	if interval <= 0 || task == nil {
		return false
	}
	return common.StartBackgroundTask(name, func(ctx context.Context) {
		runPeriodicTask(ctx, interval, immediate, task)
	})
}

func runPeriodicTask(ctx context.Context, interval time.Duration, immediate bool, task func(context.Context)) {
	if ctx.Err() != nil {
		return
	}
	if immediate {
		task(ctx)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ctx.Err() == nil {
				task(ctx)
			}
		}
	}
}
