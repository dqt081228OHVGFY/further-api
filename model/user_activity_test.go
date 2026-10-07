package model

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestInactiveUserFilterUsesCallLogsBeforePaginationAcrossSeparateDatabases(t *testing.T) {
	setupFurtherStatsTest(t)
	insertUsersForPaginationTest(t, 7)
	logDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "separate-logs.db")), &gorm.Config{})
	require.NoError(t, err)
	previousLogDB := LOG_DB
	LOG_DB = logDB
	t.Cleanup(func() { LOG_DB = previousLogDB; db, _ := logDB.DB(); _ = db.Close() })
	require.NoError(t, logDB.AutoMigrate(&Log{}))
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 6).Update("group", "other").Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 7).Update("role", 10).Error)
	const since = int64(1700000000)
	require.NoError(t, logDB.Create(&[]Log{
		{UserId: 1, Type: LogTypeConsume, CreatedAt: since - 1},
		{UserId: 2, Type: LogTypeConsume, CreatedAt: since},
		{UserId: 4, Type: LogTypeError, CreatedAt: since + 1},
		{UserId: 5, Type: LogTypeTopup, CreatedAt: since + 1},
	}).Error)
	role, status := 1, 1
	users, total, err := SearchInactiveUsers("user", "default", &role, &status, 1, 1, since, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	require.Len(t, users, 1)
	assert.Equal(t, 3, users[0].Id)
	assert.Equal(t, "user03", users[0].Username)
	assert.Empty(t, users[0].Password)
	users, total, err = SearchInactiveUsers("user", "default", &role, &status, 0, 100, since, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	assert.Equal(t, []int{1, 3, 5}, collectUserIDs(users))
	users, total, err = SearchInactiveUsers("user", "default", &role, &status, 3, 100, since)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	assert.Empty(t, users)
}

func TestInactiveUserFilterDoesNotHideUnavailableActivityDatabase(t *testing.T) {
	setupFurtherStatsTest(t)
	insertUsersForPaginationTest(t, 1)
	previous := LOG_DB
	LOG_DB = nil
	t.Cleanup(func() { LOG_DB = previous })
	_, _, err := SearchInactiveUsers("user", "", nil, nil, 0, 25, 1700000000)
	require.ErrorContains(t, err, "调用日志数据库")
}
