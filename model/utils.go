package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"

	"gorm.io/gorm"
)

const (
	BatchUpdateTypeUserQuota = iota
	BatchUpdateTypeTokenQuota
	BatchUpdateTypeUsedQuota
	BatchUpdateTypeChannelUsedQuota
	BatchUpdateTypeRequestCount
	BatchUpdateTypeCount // if you add a new type, you need to add a new map and a new lock
)

var batchUpdateStores []map[int]int
var batchUpdateLocks []sync.Mutex
var batchFlushGate = make(chan struct{}, 1)

func init() {
	for range BatchUpdateTypeCount {
		batchUpdateStores = append(batchUpdateStores, make(map[int]int))
		batchUpdateLocks = append(batchUpdateLocks, sync.Mutex{})
	}
}

func InitBatchUpdater() {
	common.StartBackgroundTask("batch accounting updater", func(ctx context.Context) {
		interval := time.Duration(common.BatchUpdateInterval) * time.Second
		if interval <= 0 {
			interval = 5 * time.Second
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				// The process drains requests and workers before its final flush.
				// Leaving that flush to the owner keeps the database open until
				// every producer has stopped adding accounting records.
				return
			case <-ticker.C:
				if ctx.Err() != nil {
					return
				}
				if err := FlushBatchUpdatesContext(ctx); err != nil && ctx.Err() == nil {
					common.SysError("failed to flush batch accounting updates: " + err.Error())
				}
			}
		}
	})
}

func addNewRecord(type_ int, id int, value int) {
	batchUpdateLocks[type_].Lock()
	defer batchUpdateLocks[type_].Unlock()
	old, ok := batchUpdateStores[type_][id]
	if !ok {
		batchUpdateStores[type_][id] = value
		return
	}

	sum := old + value
	if (value > 0 && sum < old) || (value < 0 && sum > old) {
		common.SysError(fmt.Sprintf("batch update overflow: type=%d id=%d old=%d value=%d", type_, id, old, value))
		if value > 0 {
			sum = math.MaxInt
		} else {
			sum = math.MinInt
		}
	}
	batchUpdateStores[type_][id] = sum
}

func batchUpdate() {
	if err := FlushBatchUpdates(); err != nil {
		common.SysError("failed to flush batch accounting updates: " + err.Error())
	}
}

// FlushBatchUpdates persists the currently queued accounting records. Flushes
// are serialized so shutdown waits for an active periodic flush, while request
// producers can continue enqueueing during the transaction. A failed batch is
// restored for retry; none of its writes may be retained independently.
func FlushBatchUpdates() error {
	return FlushBatchUpdatesContext(context.Background())
}

// FlushBatchUpdatesContext bounds both waiting for another flush and database
// work. Cancelled transactions retain their full snapshot for a later drain.
func FlushBatchUpdatesContext(ctx context.Context) error {
	select {
	case batchFlushGate <- struct{}{}:
		defer func() { <-batchFlushGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	stores := make([]map[int]int, BatchUpdateTypeCount)
	hasData := false
	for i := range BatchUpdateTypeCount {
		batchUpdateLocks[i].Lock()
		stores[i] = batchUpdateStores[i]
		hasData = hasData || len(stores[i]) > 0
		batchUpdateStores[i] = make(map[int]int)
		batchUpdateLocks[i].Unlock()
	}
	if !hasData {
		return nil
	}

	// Restore also runs if GORM or a driver panics, leaving accounting available
	// for the owner's final flush rather than silently dropping the snapshot.
	committed := false
	defer func() {
		if !committed {
			for kind, store := range stores {
				for id, delta := range store {
					addNewRecord(kind, id, delta)
				}
			}
		}
	}()
	if DB == nil {
		return gorm.ErrInvalidDB
	}

	common.SysLog("batch update started")
	if err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return persistBatchUpdates(tx, stores)
	}); err != nil {
		return fmt.Errorf("persist batch accounting: %w", err)
	}
	committed = true
	common.SysLog("batch update finished")
	return nil
}

func persistBatchUpdates(tx *gorm.DB, stores []map[int]int) error {
	// Stable ordering reduces lock inversions when several gateway processes
	// flush against the same database. Expressions preserve concurrent changes.
	for _, id := range sortedBatchUpdateIDs(stores[BatchUpdateTypeTokenQuota]) {
		delta := stores[BatchUpdateTypeTokenQuota][id]
		if err := tx.Model(&Token{}).Where("id = ?", id).Updates(map[string]any{
			"remain_quota":  gorm.Expr("remain_quota + ?", delta),
			"used_quota":    gorm.Expr("used_quota - ?", delta),
			"accessed_time": common.GetTimestamp(),
		}).Error; err != nil {
			return fmt.Errorf("update token %d accounting: %w", id, err)
		}
	}
	for _, id := range sortedBatchUpdateIDs(stores[BatchUpdateTypeChannelUsedQuota]) {
		if err := tx.Model(&Channel{}).Where("id = ?", id).
			Update("used_quota", gorm.Expr("used_quota + ?", stores[BatchUpdateTypeChannelUsedQuota][id])).Error; err != nil {
			return fmt.Errorf("update channel %d accounting: %w", id, err)
		}
	}

	userQuotaStore := stores[BatchUpdateTypeUserQuota]
	usedQuotaStore := stores[BatchUpdateTypeUsedQuota]
	requestCountStore := stores[BatchUpdateTypeRequestCount]
	userIDs := make(map[int]int, len(userQuotaStore)+len(usedQuotaStore)+len(requestCountStore))
	for key := range userQuotaStore {
		userIDs[key] = 0
	}
	for key := range usedQuotaStore {
		userIDs[key] = 0
	}
	for key := range requestCountStore {
		userIDs[key] = 0
	}
	for _, id := range sortedBatchUpdateIDs(userIDs) {
		quota, used, requests := userQuotaStore[id], usedQuotaStore[id], requestCountStore[id]
		if quota == 0 && used == 0 && requests == 0 {
			continue
		}
		if err := tx.Model(&User{}).Where("id = ?", id).Updates(map[string]any{
			"quota":         gorm.Expr("quota + ?", quota),
			"used_quota":    gorm.Expr("used_quota + ?", used),
			"request_count": gorm.Expr("request_count + ?", requests),
		}).Error; err != nil {
			return fmt.Errorf("update user %d accounting: %w", id, err)
		}
	}
	return nil
}

func sortedBatchUpdateIDs(store map[int]int) []int {
	ids := make([]int, 0, len(store))
	for id := range store {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func RecordExist(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

func shouldUpdateRedis(fromDB bool, err error) bool {
	return common.RedisEnabled && fromDB && err == nil
}
