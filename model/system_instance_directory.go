package model

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
)

func ListSystemInstancesContext(ctx context.Context) ([]*SystemInstance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	rows := []*SystemInstance{}
	err := DB.WithContext(ctx).Order("last_seen_at DESC").Order("node_name ASC").Find(&rows).Error
	return rows, err
}

func DeleteStaleSystemInstancesContext(ctx context.Context, now int64) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if now <= 0 {
		return 0, errors.New("节点清理时间无效")
	}
	if DB == nil {
		return 0, gorm.ErrInvalidDB
	}
	result := DB.WithContext(ctx).Where("last_seen_at < ?", now-SystemInstanceStaleAfterSeconds).Delete(&SystemInstance{})
	return result.RowsAffected, result.Error
}

func DeleteStaleSystemInstanceContext(ctx context.Context, node string, now int64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if strings.TrimSpace(node) == "" || len(node) > 128 || now <= 0 {
		return false, errors.New("节点清理参数无效")
	}
	if DB == nil {
		return false, gorm.ErrInvalidDB
	}
	result := DB.WithContext(ctx).Where("node_name = ? AND last_seen_at < ?", node, now-SystemInstanceStaleAfterSeconds).Delete(&SystemInstance{})
	return result.RowsAffected == 1, result.Error
}
