package model

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRiskStorageMigratesAndFiltersWithoutRawIdentifiers(t *testing.T) {
	previous := DB
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "risk.db")), &gorm.Config{})
	require.NoError(t, err)
	DB = database
	t.Cleanup(func() {
		DB = previous
		sqlDB, err := database.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})
	for range 2 {
		require.NoError(t, DB.AutoMigrate(&RiskEvent{}, &RiskSnapshot{}, &RiskReview{}))
	}
	require.NoError(t, DB.Exec("CREATE TABLE users (id integer primary key, username text, deleted_at datetime)").Error)
	require.NoError(t, DB.Exec("INSERT INTO users (id, username) VALUES (?, ?)", 7, "risk_user").Error)

	now := time.Now().UTC().Unix()
	bad := &RiskEvent{UserID: 7, OccurredAt: now, IPFingerprint: "192.0.2.1"}
	require.Error(t, CreateRiskEvent(bad))
	event := &RiskEvent{UserID: 7, TokenID: 3, OccurredAt: now,
		Model: "gpt-4.1", IPFingerprint: strings.Repeat("a", 64),
		InputSignature: strings.Repeat("b", 64), RequestID: "req_1"}
	require.NoError(t, CreateRiskEvent(event))
	var stored RiskEvent
	require.NoError(t, DB.First(&stored, event.ID).Error)
	require.Len(t, stored.Model, 64)
	require.NotEqual(t, "gpt-4.1", stored.Model)
	require.Equal(t, strings.Repeat("a", 64), stored.IPFingerprint)

	snapshot := &RiskSnapshot{UserID: 7, WindowDays: 7, Score: 65,
		Level: "high", LastRequestAt: now, RequestCount: 1, UpdatedAt: now}
	require.NoError(t, UpsertRiskSnapshot(snapshot))
	items, total, err := ListRiskSnapshots(7, now-1, 1, 20, "risk_", "attention", "unreviewed")
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, 65, items[0].Score)

	snapshot.Score = 82
	snapshot.Level = "critical"
	require.NoError(t, UpsertRiskSnapshot(snapshot))
	require.NoError(t, UpsertRiskReview(&RiskReview{UserID: 7, Status: "confirmed", UpdatedAt: now}))
	items, total, err = ListRiskSnapshots(7, now-1, 1, 20, "", "attention", "confirmed")
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, 82, items[0].Score)
	_, total, err = ListRiskSnapshots(7, now-1, 1, 20, "", "", "unreviewed")
	require.NoError(t, err)
	require.Zero(t, total)
	_, total, err = ListRiskSnapshots(7, now-1, 1, 20, "risk%", "", "")
	require.NoError(t, err)
	require.Zero(t, total, "search wildcards must be treated as literal input")
	require.NoError(t, DB.Exec("UPDATE users SET deleted_at = ? WHERE id = ?", time.Now(), 7).Error)
	_, total, err = ListRiskSnapshots(7, now-1, 1, 20, "", "", "")
	require.NoError(t, err)
	require.Zero(t, total, "deleted accounts must not appear in risk lists")
}

func TestRiskRetentionPreservesCurrentEventsAndReview(t *testing.T) {
	previous := DB
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "risk-retention.db")), &gorm.Config{})
	require.NoError(t, err)
	DB = database
	t.Cleanup(func() {
		DB = previous
		sqlDB, err := database.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})
	require.NoError(t, DB.AutoMigrate(&RiskEvent{}, &RiskSnapshot{}, &RiskReview{}))
	now := time.Now().Unix()
	for _, timestamp := range []int64{now - 8*24*3600, now - 7*24*3600, now} {
		require.NoError(t, CreateRiskEvent(&RiskEvent{UserID: 7, OccurredAt: timestamp}))
	}
	removed, err := PruneRiskEvents(now-7*24*3600, 1000)
	require.NoError(t, err)
	require.Equal(t, 1, removed)
	events, err := ListRiskEvents(7, 0, 100)
	require.NoError(t, err)
	require.Len(t, events, 2)
	for _, days := range []int{3, 7} {
		require.NoError(t, UpsertRiskSnapshot(&RiskSnapshot{UserID: 7, WindowDays: days, LastRequestAt: now - 4*24*3600}))
	}
	require.NoError(t, UpsertRiskReview(&RiskReview{UserID: 7, Status: "watching", ReviewedBy: 1, UpdatedAt: now}))
	require.NoError(t, PruneRiskSnapshots(now))
	_, err = GetRiskSnapshot(7, 3)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = GetRiskSnapshot(7, 7)
	require.NoError(t, err)
	statuses, err := RiskReviewStatuses([]int{7})
	require.NoError(t, err)
	require.Equal(t, "watching", statuses[7])
}
