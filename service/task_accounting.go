package service

import (
	"context"
	"fmt"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/model"
)

func settleTaskAccounting(ctx context.Context, task *model.Task, quota int, reason string, clamps []*common.QuotaClamp) bool {
	if task == nil || quota < 0 {
		return false
	}
	result, err := model.ApplyTaskAccountingContext(ctx, task, quota)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("任务 %s 计费事务未提交：%s", task.TaskID, err))
		return false
	}
	if !result.Applied {
		return true
	}
	other := taskBillingOther(task)
	other.SetPublic("task_id", task.TaskID)
	other.SetPublic("reason", reason)
	other.SetPublic("pre_consumed_quota", result.Before)
	other.SetPublic("actual_quota", result.After)
	for _, clamp := range clamps {
		attachQuotaSaturationToOther(other, clamp)
	}
	typeCode, amount := model.LogTypeConsume, result.Delta
	if amount < 0 {
		typeCode, amount = model.LogTypeRefund, -amount
	}
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId: task.UserId, LogType: typeCode, Content: reason, ChannelId: task.ChannelId,
		ModelName: taskModelName(task), Quota: amount, TokenId: task.PrivateData.TokenId,
		Group: task.Group, Other: other, NodeName: task.PrivateData.NodeName,
	})
	return true
}

func RefundTaskQuota(ctx context.Context, task *model.Task, reason string) bool {
	if task == nil {
		return false
	}
	if task.Quota == 0 {
		return true
	}
	return settleTaskAccounting(ctx, task, 0, reason, nil)
}

func RecalculateTaskQuota(ctx context.Context, task *model.Task, actualQuota int, reason string, clamps ...*common.QuotaClamp) {
	settleTaskAccounting(ctx, task, actualQuota, reason, clamps)
}
