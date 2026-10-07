package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTaskAccountingQuotaWriteFailureRollsBackFundsAndToken(t *testing.T) {
	truncate(t)
	seedUser(t, 91, 10000)
	seedToken(t, 91, 91, "task-atomic-token", 10000)
	seedChannel(t, 91)
	seedChargedAccounting(t, 91, 91, 91, 2000, 1)
	task := makeTask(91, 91, 2000, 91, BillingSourceWallet, 0)
	require.NoError(t, model.DB.Create(task).Error)
	callback := "test:task_quota_write_failure"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "tasks" {
			tx.AddError(errors.New("forced task marker failure"))
		}
	}))
	_, err := model.ApplyTaskAccountingContext(context.Background(), task, 0)
	require.Error(t, err)
	require.Equal(t, 10000, getUserQuota(t, 91))
	require.Equal(t, 10000, getTokenRemainQuota(t, 91))
	require.Equal(t, 2000, getTaskQuota(t, task.ID))
	require.NoError(t, model.DB.Callback().Update().Remove(callback))
	require.True(t, RefundTaskQuota(context.Background(), task, "retry after rollback"))
	require.Equal(t, 12000, getUserQuota(t, 91))
}

func TestTaskAccountingConcurrentStaleSnapshotsRefundOnce(t *testing.T) {
	truncate(t)
	seedUser(t, 92, 10000)
	seedToken(t, 92, 92, "task-concurrent-token", 10000)
	seedChannel(t, 92)
	seedChargedAccounting(t, 92, 92, 92, 2000, 1)
	task := makeTask(92, 92, 2000, 92, BillingSourceWallet, 0)
	require.NoError(t, model.DB.Create(task).Error)
	var callers sync.WaitGroup
	for range 20 {
		copy := *task
		callers.Add(1)
		go func() { defer callers.Done(); RefundTaskQuota(context.Background(), &copy, "concurrent failure") }()
	}
	callers.Wait()
	require.Equal(t, 12000, getUserQuota(t, 92))
	require.Equal(t, 12000, getTokenRemainQuota(t, 92))
	require.Zero(t, getTaskQuota(t, task.ID))
	require.EqualValues(t, 1, countLogs(t))
}

func TestTaskAccountingRefundDoesNotMutateDeletedToken(t *testing.T) {
	truncate(t)
	seedUser(t, 98, 10000)
	seedToken(t, 98, 98, "deleted-task-token", 10000)
	seedChannel(t, 98)
	seedChargedAccounting(t, 98, 98, 98, 2000, 1)
	task := makeTask(98, 98, 2000, 98, BillingSourceWallet, 0)
	require.NoError(t, model.DB.Create(task).Error)
	require.NoError(t, model.DB.Delete(&model.Token{}, 98).Error)
	require.True(t, RefundTaskQuota(context.Background(), task, "removed token task failed"))
	var removed model.Token
	require.NoError(t, model.DB.Unscoped().First(&removed, 98).Error)
	require.True(t, removed.DeletedAt.Valid)
	require.Equal(t, 10000, removed.RemainQuota)
	require.Equal(t, 2000, removed.UsedQuota)
	require.Equal(t, 12000, getUserQuota(t, 98))
	require.Zero(t, getTaskQuota(t, task.ID))
}
