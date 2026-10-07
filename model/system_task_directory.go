package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

func GetSystemTaskByTaskIDContext(ctx context.Context, taskID string) (*SystemTask, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var rows []SystemTask
	if err := DB.WithContext(ctx).Where("task_id = ?", taskID).Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func GetActiveSystemTaskContext(ctx context.Context, taskType string) (*SystemTask, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var rows []SystemTask
	err := DB.WithContext(ctx).Where("type = ? AND status IN ?", taskType, activeSystemTaskStatuses()).Order("id DESC").Limit(1).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func ValidateSystemTaskFilter(filter SystemTaskFilter) error {
	if len(filter.Type) > 64 || strings.TrimSpace(filter.Type) != filter.Type {
		return errors.New("系统任务类型无效")
	}
	switch filter.Scope {
	case "", "active", "history":
	default:
		return errors.New("系统任务范围无效")
	}
	switch filter.Status {
	case "", SystemTaskStatusPending, SystemTaskStatusRunning, SystemTaskStatusSucceeded, SystemTaskStatusFailed:
	default:
		return errors.New("系统任务状态无效")
	}
	return nil
}

func systemTaskDirectoryQuery(tx *gorm.DB, filter SystemTaskFilter) *gorm.DB {
	query := tx.Model(&SystemTask{})
	if filter.Scope == "active" {
		query = query.Where("status IN ?", activeSystemTaskStatuses())
	}
	if filter.Scope == "history" {
		query = query.Where("status IN ?", []SystemTaskStatus{SystemTaskStatusSucceeded, SystemTaskStatusFailed})
	}
	if filter.Type != "" {
		query = query.Where("type = ?", filter.Type)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	return query
}

func ListSystemTasksContext(ctx context.Context, filter SystemTaskFilter, offset, limit int) ([]*SystemTask, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if err := ValidateSystemTaskFilter(filter); err != nil {
		return nil, 0, err
	}
	if DB == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	if limit <= 0 {
		limit = 20
	}
	limit = min(limit, 100)
	offset = max(0, offset)
	query := systemTaskDirectoryQuery(DB.WithContext(ctx), filter)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []*SystemTask{}
	err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&rows).Error
	return rows, total, err
}

func DeleteSystemTaskHistoryContext(ctx context.Context, filter SystemTaskFilter) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := ValidateSystemTaskFilter(filter); err != nil {
		return 0, err
	}
	if DB == nil {
		return 0, gorm.ErrInvalidDB
	}
	filter.Scope = "history"
	var deleted int64
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var retained []int64
		if err := tx.Model(&SystemTask{}).Select("MAX(id)").Group("type").Pluck("MAX(id)", &retained).Error; err != nil {
			return err
		}
		if len(retained) == 0 {
			return nil
		}
		var upper int64
		for _, id := range retained {
			upper = max(upper, id)
		}
		if upper <= 0 {
			return fmt.Errorf("系统任务历史快照无效")
		}
		result := systemTaskDirectoryQuery(tx, filter).Where("id < ? AND id NOT IN ?", upper, retained).Delete(&SystemTask{})
		deleted = result.RowsAffected
		return result.Error
	})
	return deleted, err
}
