package model

import (
	"context"
	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
)

type TopUpDirectoryInput struct {
	UserID        int
	Admin         bool
	Keyword       string
	Offset, Limit int
}

func ListTopUpDirectoryContext(ctx context.Context, input TopUpDirectoryInput) ([]*TopUp, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if !input.Admin && input.UserID <= 0 {
		return nil, 0, gorm.ErrRecordNotFound
	}
	rows := make([]*TopUp, 0)
	var total int64
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&TopUp{})
		if !input.Admin {
			query = query.Where("user_id = ? AND create_time >= ?", input.UserID, topUpQueryCutoff())
		}
		if input.Keyword != "" {
			pattern, err := sanitizeLikePattern(input.Keyword)
			if err != nil {
				return err
			}
			query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
		}
		if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return err
		}
		return query.Order("id DESC").Offset(max(input.Offset, 0)).Limit(input.Limit).Find(&rows).Error
	})
	return rows, total, err
}

func GetUserTopUps(id int, page *common.PageInfo) ([]*TopUp, int64, error) {
	return ListTopUpDirectoryContext(context.Background(), TopUpDirectoryInput{UserID: id, Offset: page.GetStartIdx(), Limit: page.GetPageSize()})
}
func GetAllTopUps(page *common.PageInfo) ([]*TopUp, int64, error) {
	return ListTopUpDirectoryContext(context.Background(), TopUpDirectoryInput{Admin: true, Offset: page.GetStartIdx(), Limit: page.GetPageSize()})
}
func SearchUserTopUps(id int, keyword string, page *common.PageInfo) ([]*TopUp, int64, error) {
	return ListTopUpDirectoryContext(context.Background(), TopUpDirectoryInput{UserID: id, Keyword: keyword, Offset: page.GetStartIdx(), Limit: page.GetPageSize()})
}
func SearchAllTopUps(keyword string, page *common.PageInfo) ([]*TopUp, int64, error) {
	return ListTopUpDirectoryContext(context.Background(), TopUpDirectoryInput{Admin: true, Keyword: keyword, Offset: page.GetStartIdx(), Limit: page.GetPageSize()})
}
