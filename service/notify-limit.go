package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
)

type notificationWindow struct {
	count   int
	expires time.Time
}

var notificationWindows = struct {
	sync.Mutex
	items     map[string]notificationWindow
	lastSweep time.Time
}{items: make(map[string]notificationWindow)}

// Counter and TTL are updated in the same Redis operation. Concurrent workers
// can never all admit the same last available notification slot.
const notificationWindowScript = `
local maximum = tonumber(ARGV[1])
if maximum <= 0 then return 0 end
local count = tonumber(redis.call('GET', KEYS[1]) or '0')
if count >= maximum then return 0 end
count = redis.call('INCR', KEYS[1])
if count == 1 or redis.call('PTTL', KEYS[1]) < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
end
return 1
`

func getDuration() time.Duration {
	minutes := int64(constant.NotificationLimitDurationMinute)
	if minutes <= 0 {
		return time.Hour
	}
	minutes = min(minutes, math.MaxInt64/int64(time.Minute))
	return time.Duration(minutes) * time.Minute
}

func notificationLimitKey(userID int, purpose string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(purpose)))
	return fmt.Sprintf("further-api:notification:limit:v1:%d:%s", userID, hex.EncodeToString(digest[:12]))
}

func CheckNotificationLimit(userID int, purpose string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return CheckNotificationLimitContext(ctx, userID, purpose)
}

func CheckNotificationLimitContext(ctx context.Context, userID int, purpose string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if constant.NotifyLimitCount <= 0 {
		return false, nil
	}
	key := notificationLimitKey(userID, purpose)
	if common.RedisEnabled {
		if common.RDB == nil {
			return false, errors.New("通知计数服务尚未初始化。")
		}
		admitted, err := common.RDB.Eval(ctx, notificationWindowScript, []string{key}, constant.NotifyLimitCount, getDuration().Milliseconds()).Int()
		if err != nil {
			return false, fmt.Errorf("通知频率检查失败：%w", err)
		}
		return admitted == 1, nil
	}
	now := time.Now()
	notificationWindows.Lock()
	defer notificationWindows.Unlock()
	if now.Sub(notificationWindows.lastSweep) >= time.Minute {
		for key, entry := range notificationWindows.items {
			if !now.Before(entry.expires) {
				delete(notificationWindows.items, key)
			}
		}
		notificationWindows.lastSweep = now
	}
	entry := notificationWindows.items[key]
	if !now.Before(entry.expires) {
		entry = notificationWindow{expires: now.Add(getDuration())}
	}
	if entry.count >= constant.NotifyLimitCount {
		return false, nil
	}
	entry.count++
	notificationWindows.items[key] = entry
	return true, nil
}

func checkMemoryLimit(userID int, purpose string) (bool, error) {
	return CheckNotificationLimitContext(context.Background(), userID, purpose)
}
func checkRedisLimit(userID int, purpose string) (bool, error) {
	return CheckNotificationLimitContext(context.Background(), userID, purpose)
}
