package model

import "context"

// Option preserves the installed database key space. Runtime configuration,
// validation and transaction commands live in the gateway configuration modules.
type Option struct {
	Key   string `json:"key" gorm:"primaryKey"`
	Value string `json:"value"`
}

func AllOption() ([]*Option, error) { return AllOptionsContext(context.Background()) }
func AllOptionsContext(ctx context.Context) ([]*Option, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var rows []*Option
	err := DB.WithContext(ctx).Order(commonKeyCol).Find(&rows).Error
	return rows, err
}
