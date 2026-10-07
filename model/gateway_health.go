package model

import (
	"context"
	"gorm.io/gorm"
	"time"
)

func PingDBContext(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	database, err := DB.DB()
	if err != nil {
		return err
	}
	return database.PingContext(ctx)
}

func PingDB() error { return PingDBContext(context.Background()) }
