package model

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
)

func GetByTaskIdContext(ctx context.Context, userID int, taskID string) (*Task, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if userID <= 0 || strings.TrimSpace(taskID) == "" {
		return nil, false, nil
	}
	var task Task
	err := DB.WithContext(ctx).Where("user_id = ? AND task_id = ?", userID, taskID).First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &task, true, nil
}

func (task *Task) UpdateWithStatusContext(ctx context.Context, from TaskStatus) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if task == nil || task.ID <= 0 {
		return false, errors.New("任务持久化编号无效。")
	}
	updated := DB.WithContext(ctx).Model(task).Where("status = ?", from).Select("*").Updates(task)
	return updated.RowsAffected > 0, updated.Error
}
