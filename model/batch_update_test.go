package model

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type batchAccountingFixture struct {
	user    User
	token   Token
	channel Channel
}

func prepareBatchAccounting(t *testing.T) batchAccountingFixture {
	t.Helper()
	truncateTables(t)
	resetBatchUpdateTestState(t)
	fixture := batchAccountingFixture{
		user:    createReserveTestUser(t, 100),
		token:   createReserveTestToken(t, 80),
		channel: Channel{Name: "batch-accounting", Key: "test", UsedQuota: 5},
	}
	require.NoError(t, DB.Model(&fixture.user).Updates(map[string]any{
		"used_quota": 3, "request_count": 2,
	}).Error)
	require.NoError(t, DB.Model(&fixture.token).Update("used_quota", 2).Error)
	require.NoError(t, DB.Create(&fixture.channel).Error)
	return fixture
}

func queueBatchAccounting(fixture batchAccountingFixture, charge int) {
	addNewRecord(BatchUpdateTypeUserQuota, fixture.user.Id, -charge)
	addNewRecord(BatchUpdateTypeTokenQuota, fixture.token.Id, -charge)
	addNewRecord(BatchUpdateTypeUsedQuota, fixture.user.Id, charge)
	addNewRecord(BatchUpdateTypeChannelUsedQuota, fixture.channel.Id, charge)
	addNewRecord(BatchUpdateTypeRequestCount, fixture.user.Id, 1)
}

func assertBatchAccounting(t *testing.T, fixture batchAccountingFixture, charge, requests int) {
	t.Helper()
	var user User
	require.NoError(t, DB.First(&user, fixture.user.Id).Error)
	assert.Equal(t, 100-charge, user.Quota)
	assert.Equal(t, 3+charge, user.UsedQuota)
	assert.Equal(t, 2+requests, user.RequestCount)
	token := getTokenFromDB(t, fixture.token.Id)
	assert.Equal(t, 80-charge, token.RemainQuota)
	assert.Equal(t, 2+charge, token.UsedQuota)
	var channel Channel
	require.NoError(t, DB.First(&channel, fixture.channel.Id).Error)
	assert.EqualValues(t, 5+charge, channel.UsedQuota)
}

func assertBatchQueueEmpty(t *testing.T) {
	t.Helper()
	for kind := range BatchUpdateTypeCount {
		batchUpdateLocks[kind].Lock()
		assert.Empty(t, batchUpdateStores[kind], "batch queue %d", kind)
		batchUpdateLocks[kind].Unlock()
	}
}

func TestFlushBatchUpdatesRollsBackAndRetriesEveryAccountingType(t *testing.T) {
	for _, table := range []string{"tokens", "channels", "users"} {
		t.Run(table, func(t *testing.T) {
			fixture := prepareBatchAccounting(t)
			queueBatchAccounting(fixture, 20)
			// Real database failures cover both early and late transaction writes.
			require.NoError(t, DB.Exec(fmt.Sprintf(`CREATE TEMP TRIGGER fail_batch_update
				BEFORE UPDATE ON %s BEGIN SELECT RAISE(ABORT, 'injected batch failure'); END`, table)).Error)
			t.Cleanup(func() { require.NoError(t, DB.Exec("DROP TRIGGER IF EXISTS fail_batch_update").Error) })

			require.ErrorContains(t, FlushBatchUpdates(), "injected batch failure")
			assertBatchAccounting(t, fixture, 0, 0)
			for kind, id := range []int{fixture.user.Id, fixture.token.Id, fixture.user.Id, fixture.channel.Id, fixture.user.Id} {
				batchUpdateLocks[kind].Lock()
				assert.NotZero(t, batchUpdateStores[kind][id], "failed batch type %d must remain queued", kind)
				batchUpdateLocks[kind].Unlock()
			}

			require.NoError(t, DB.Exec("DROP TRIGGER fail_batch_update").Error)
			require.NoError(t, FlushBatchUpdates())
			assertBatchAccounting(t, fixture, 20, 1)
			assertBatchQueueEmpty(t)
			require.NoError(t, FlushBatchUpdates())
			assertBatchAccounting(t, fixture, 20, 1)
		})
	}
}

func TestFlushBatchUpdatesFailureMergesRecordsAddedDuringFlush(t *testing.T) {
	fixture := prepareBatchAccounting(t)
	queueBatchAccounting(fixture, 20)
	const callback = "test:batch_enqueue_during_failed_flush"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			// Producers write into the new queue while the detached snapshot is
			// being persisted. Retrying must keep both sets of accounting.
			queueBatchAccounting(fixture, 7)
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Update().Remove(callback) })
	require.NoError(t, DB.Exec(`CREATE TEMP TRIGGER fail_batch_update
		BEFORE UPDATE ON users BEGIN SELECT RAISE(ABORT, 'injected batch failure'); END`).Error)
	t.Cleanup(func() { require.NoError(t, DB.Exec("DROP TRIGGER IF EXISTS fail_batch_update").Error) })

	require.Error(t, FlushBatchUpdates())
	assertBatchAccounting(t, fixture, 0, 0)
	require.NoError(t, DB.Callback().Update().Remove(callback))
	require.NoError(t, DB.Exec("DROP TRIGGER fail_batch_update").Error)
	require.NoError(t, FlushBatchUpdates())
	assertBatchAccounting(t, fixture, 27, 2)
	assertBatchQueueEmpty(t)
}

func TestFlushBatchUpdatesSerializesConcurrentFlushes(t *testing.T) {
	fixture := prepareBatchAccounting(t)
	queueBatchAccounting(fixture, 20)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	const callback = "test:batch_pause_first_flush"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			once.Do(func() {
				close(started)
				<-release
			})
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Update().Remove(callback) })
	first := make(chan error, 1)
	go func() { first <- FlushBatchUpdates() }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("first batch flush did not reach the database")
	}
	queueBatchAccounting(fixture, 7)
	second := make(chan error, 1)
	go func() { second <- FlushBatchUpdates() }()
	close(release)
	require.NoError(t, <-first)
	require.NoError(t, <-second)
	assertBatchAccounting(t, fixture, 27, 2)
	assertBatchQueueEmpty(t)
}

func TestFlushBatchUpdatesPreservesQueueWhenDatabaseIsUnavailable(t *testing.T) {
	fixture := prepareBatchAccounting(t)
	queueBatchAccounting(fixture, 20)
	previousDB := DB
	DB = nil
	t.Cleanup(func() { DB = previousDB })
	require.ErrorIs(t, FlushBatchUpdates(), gorm.ErrInvalidDB)
	DB = previousDB
	require.NoError(t, FlushBatchUpdates())
	assertBatchAccounting(t, fixture, 20, 1)
	assertBatchQueueEmpty(t)
}

func TestFlushBatchUpdatesCancellationRollsBackAndPreservesQueue(t *testing.T) {
	fixture := prepareBatchAccounting(t)
	queueBatchAccounting(fixture, 20)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const callback = "test:cancel_batch_transaction"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(*gorm.DB) { cancel() }))
	t.Cleanup(func() { _ = DB.Callback().Update().Remove(callback) })
	require.ErrorIs(t, FlushBatchUpdatesContext(ctx), context.Canceled)
	require.NoError(t, DB.Callback().Update().Remove(callback))
	assertBatchAccounting(t, fixture, 0, 0)
	require.NoError(t, FlushBatchUpdates())
	assertBatchAccounting(t, fixture, 20, 1)
	assertBatchQueueEmpty(t)
}

func TestFlushBatchUpdatesDeadlineBoundsLockWait(t *testing.T) {
	fixture := prepareBatchAccounting(t)
	queueBatchAccounting(fixture, 20)
	batchFlushGate <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := FlushBatchUpdatesContext(ctx)
	<-batchFlushGate
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, FlushBatchUpdates())
	assertBatchAccounting(t, fixture, 20, 1)
}

func TestInitBatchUpdaterCancellationAllowsFinalShutdownFlush(t *testing.T) {
	fixture := prepareBatchAccounting(t)
	previousContext := common.BackgroundContext()
	previousInterval := common.BatchUpdateInterval
	ctx, cancel := context.WithCancel(context.Background())
	common.SetBackgroundContext(ctx)
	common.BatchUpdateInterval = 3600
	t.Cleanup(func() {
		cancel()
		wait, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		require.NoError(t, common.WaitBackgroundTasks(wait))
		common.SetBackgroundContext(previousContext)
		common.BatchUpdateInterval = previousInterval
	})
	InitBatchUpdater()
	queueBatchAccounting(fixture, 20)
	cancel()
	wait, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	require.NoError(t, common.WaitBackgroundTasks(wait), "the updater must stop without waiting for its one-hour timer")
	assertBatchAccounting(t, fixture, 0, 0)
	// This matches main.go: cancel workers, await their final writes, then flush
	// accounting while the database is still available.
	require.NoError(t, FlushBatchUpdates())
	assertBatchAccounting(t, fixture, 20, 1)
	assertBatchQueueEmpty(t)
}
