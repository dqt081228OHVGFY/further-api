package router

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRelayForSeasonalHTMLBeforeFirstPaint(t *testing.T) {
	previous := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	t.Cleanup(func() { model.DB = previous; require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&model.RelayForHolidayCampaign{}))
	page := []byte(`<!doctype html><html lang="zh-CN"><head><link rel="stylesheet" href="/app.css"></head><body>hello</body></html>`)
	active := time.Date(2026, 10, 2, 12, 0, 0, 0, model.RelayForActivityLocation)
	output := string(relayForSeasonalHTML(page, active))
	require.Contains(t, output, `<html data-holiday="national_day" lang="zh-CN">`)
	require.Contains(t, output, `href="/app.css"`)
	require.NotContains(t, string(page), "data-holiday")
	require.Equal(t, string(page), string(relayForSeasonalHTML(page, time.Date(2026, 10, 8, 0, 0, 0, 0, model.RelayForActivityLocation))))
	campaign := model.RelayForHolidayCampaign{Key: "national_day", Title: "国庆", Enabled: false, DaysAfter: 6}
	require.NoError(t, db.Create(&campaign).Error)
	require.Equal(t, string(page), string(relayForSeasonalHTML(page, active)))
}
