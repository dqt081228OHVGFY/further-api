package model

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAccountDeletionCancellationWhileWaitingForSQLiteWriterPreservesState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "account-deletion.db") + "?_pragma=busy_timeout(1000)"
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousRedis, previousType := DB, common.RedisEnabled, common.MainDatabaseType()
	DB, common.RedisEnabled = db, false
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	connection, err := db.DB()
	require.NoError(t, err)
	connection.SetMaxOpenConns(4)
	t.Cleanup(func() {
		DB, common.RedisEnabled = previousDB, previousRedis
		common.SetMainDatabaseType(previousType)
		require.NoError(t, connection.Close())
	})
	require.NoError(t, db.AutoMigrate(&User{}, &UserSession{}))
	account := &User{Username: "cancel-delete", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1}
	require.NoError(t, db.Create(account).Error)
	session := &UserSession{SID: "cancel-delete-session", UserID: account.Id, Version: 1, UserAuthVersion: 1, Status: UserSessionStatusActive, ExpiresAt: time.Now().Add(time.Hour).Unix()}
	require.NoError(t, db.Create(session).Error)
	identity := AuthSessionIdentity{UserID: account.Id, SessionID: session.SID, UserAuthVersion: 1, SessionVersion: 1}
	held := db.Begin()
	require.NoError(t, held.Error)
	defer held.Rollback()
	require.NoError(t, held.Model(&User{}).Where("id = ?", account.Id).UpdateColumn("auth_version", gorm.Expr("auth_version")).Error)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{}, 1)
	callback := "test:account-delete-writer-entered"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Context == ctx && tx.Statement.Table == "users" {
			select {
			case entered <- struct{}{}:
			default:
			}
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove(callback) })
	completed := make(chan error, 1)
	go func() { completed <- DeleteUserForSessionContext(ctx, identity) }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("deletion did not reach its SQLite writer reservation")
	}
	select {
	case err := <-completed:
		t.Fatalf("deletion returned while another writer held its reservation: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	cancel()
	require.NoError(t, held.Rollback().Error)
	select {
	case err := <-completed:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled deletion did not release its transaction")
	}
	var saved User
	require.NoError(t, db.Unscoped().First(&saved, account.Id).Error)
	require.False(t, saved.DeletedAt.Valid)
	require.EqualValues(t, 1, saved.AuthVersion)
	require.NoError(t, db.First(session, "sid = ?", identity.SessionID).Error)
	require.Equal(t, UserSessionStatusActive, session.Status)
	require.Zero(t, session.RevokedAt)
}
