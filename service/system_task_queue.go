package service

import (
	"context"
	"errors"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/model"
)

func StartLogCleanupTask(target int64) (*model.SystemTask, error) {
	return StartLogCleanupTaskContext(context.Background(), target)
}

func StartLogCleanupTaskContext(ctx context.Context, target int64) (*model.SystemTask, error) {
	if target <= 0 {
		return nil, errors.New("请指定有效的日志清理截止时间。")
	}
	task, _, err := enqueueSystemTaskRecord(ctx, model.SystemTaskTypeLogCleanup,
		LogCleanupPayload{TargetTimestamp: target, BatchSize: logCleanupBatchSize}, LogCleanupState{})
	return task, err
}

func EnqueueSystemTask(kind string, payload any) (*model.SystemTask, bool, error) {
	return EnqueueSystemTaskContext(context.Background(), kind, payload)
}

func EnqueueSystemTaskContext(ctx context.Context, kind string, payload any) (*model.SystemTask, bool, error) {
	return enqueueSystemTaskRecord(ctx, kind, payload, nil)
}

func enqueueSystemTaskRecord(ctx context.Context, kind string, payload, state any) (*model.SystemTask, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(kind) == "" || len(kind) > 64 {
		return nil, false, errors.New("系统任务类型无效。")
	}
	// The unique active key arbitrates admission. A failed concurrent insert
	// joins the existing job and wakes the runner just like a fresh admission.
	task, creationErr := model.CreateSystemTaskContext(ctx, kind, payload, state)
	if creationErr == nil {
		notifySystemTaskRunner()
		return task, true, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	active, lookupErr := model.GetActiveSystemTaskContext(ctx, kind)
	if lookupErr != nil {
		return nil, false, errors.Join(creationErr, lookupErr)
	}
	if active == nil {
		return nil, false, creationErr
	}
	notifySystemTaskRunner()
	return active, false, nil
}
