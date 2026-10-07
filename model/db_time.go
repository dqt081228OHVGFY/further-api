package model

import (
	"context"

	"github.com/dqt081228OHVGFY/further-api/common"
)

// GetDBTimestamp returns a UNIX timestamp from database time.
// Falls back to application time on error.
func GetDBTimestamp() int64 {
	return GetDBTimestampContext(context.Background())
}

func GetDBTimestampContext(ctx context.Context) int64 {
	var ts int64
	var err error
	switch {
	case common.UsingMainDatabase(common.DatabaseTypePostgreSQL):
		err = DB.WithContext(ctx).Raw("SELECT EXTRACT(EPOCH FROM NOW())::bigint").Scan(&ts).Error
	case common.UsingMainDatabase(common.DatabaseTypeSQLite):
		err = DB.WithContext(ctx).Raw("SELECT strftime('%s','now')").Scan(&ts).Error
	default:
		err = DB.WithContext(ctx).Raw("SELECT UNIX_TIMESTAMP()").Scan(&ts).Error
	}
	if err != nil || ts <= 0 {
		return common.GetTimestamp()
	}
	return ts
}
