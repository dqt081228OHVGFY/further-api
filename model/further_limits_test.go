package model

import (
	"sync"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func furtherChannelFixture(t *testing.T) Channel {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&FurtherChannelPolicy{}, &FurtherChannelKeyPolicy{}, &FurtherChannelQuotaReservation{}))
	prior := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = prior
		for _, value := range []any{&FurtherChannelPolicy{}, &FurtherChannelKeyPolicy{}, &FurtherChannelQuotaReservation{}} {
			require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(value).Error)
		}
	})
	channel := Channel{Name: "policy-test", Type: 1, Status: 1, Group: "default", Models: "test-model", Key: "upstream-secret-first\nupstream-secret-second", ChannelInfo: ChannelInfo{IsMultiKey: true, MultiKeyMode: constant.MultiKeyModeRandom}}
	require.NoError(t, DB.Create(&channel).Error)
	return channel
}

func TestFurtherChannelKeyIDsSurviveReorderingAndMetadataNeverContainsRawKeys(t *testing.T) {
	channel := furtherChannelFixture(t)
	initial, err := GetFurtherChannelKeys(channel.Id)
	require.NoError(t, err)
	require.Len(t, initial, 2)
	encoded, err := common.Marshal(initial)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "upstream-secret-first")
	require.NotContains(t, string(encoded), initial[0].Digest)
	require.NoError(t, DB.Model(&channel).Update("key", "upstream-secret-second\nupstream-secret-first").Error)
	reordered, err := GetFurtherChannelKeys(channel.Id)
	require.NoError(t, err)
	require.Equal(t, initial[1].ID, reordered[0].ID)
	require.Equal(t, initial[0].ID, reordered[1].ID)
	_, err = SaveFurtherChannelKey(channel.Id, initial[0].ID, FurtherChannelKeyInput{Label: "disabled-first", Weight: 1, Status: 2, FurtherLimitValues: defaultFurtherLimits()}, false)
	require.NoError(t, err)
	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	require.Equal(t, 2, stored.ChannelInfo.MultiKeyStatusList[1])
	require.Equal(t, 1, stored.ChannelInfo.MultiKeyStatusList[0])
	require.Equal(t, "upstream-secret-second\nupstream-secret-first", stored.Key)
}

func TestFurtherChannelReservationsRollbackBothResourcesAndSettleExactlyOnce(t *testing.T) {
	channel := furtherChannelFixture(t)
	keys, err := GetFurtherChannelKeys(channel.Id)
	require.NoError(t, err)
	require.NoError(t, SaveFurtherChannelPolicy(FurtherChannelPolicy{ChannelID: channel.Id, FurtherLimitValues: FurtherLimitValues{QuotaTotal: 200, QuotaRemaining: 200}}))
	require.NoError(t, DB.Model(&FurtherChannelKeyPolicy{}).Where("id = ?", keys[0].ID).Updates(map[string]any{"quota_total": 100, "quota_remaining": 100}).Error)
	require.NoError(t, ReserveFurtherChannelQuota("request-1:attempt-1", channel.Id, keys[0].ID, 60))
	require.NoError(t, ReserveFurtherChannelQuota("request-1:attempt-1", channel.Id, keys[0].ID, 60))
	require.Error(t, ReserveFurtherChannelQuota("request-2:attempt-1", channel.Id, keys[0].ID, 50))
	policy, err := GetFurtherChannelPolicy(channel.Id)
	require.NoError(t, err)
	require.Equal(t, int64(140), policy.QuotaRemaining)
	require.Equal(t, int64(60), policy.QuotaReserved)
	require.NoError(t, SettleFurtherChannelQuota("request-1:attempt-1", 40))
	require.NoError(t, SettleFurtherChannelQuota("request-1:attempt-1", 40))
	require.Error(t, SettleFurtherChannelQuota("request-1:attempt-1", 0))
	policy, err = GetFurtherChannelPolicy(channel.Id)
	require.NoError(t, err)
	require.Equal(t, int64(160), policy.QuotaRemaining)
	require.Zero(t, policy.QuotaReserved)
	var key FurtherChannelKeyPolicy
	require.NoError(t, DB.First(&key, keys[0].ID).Error)
	require.Equal(t, int64(60), key.QuotaRemaining)
	require.Zero(t, key.QuotaReserved)
}

func TestFurtherChannelConcurrentReservationsCannotOverdraw(t *testing.T) {
	channel := furtherChannelFixture(t)
	require.NoError(t, SaveFurtherChannelPolicy(FurtherChannelPolicy{ChannelID: channel.Id, FurtherLimitValues: FurtherLimitValues{QuotaTotal: 50, QuotaRemaining: 50}}))
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{"attempt-a", "attempt-b"} {
		wg.Go(func() { results <- ReserveFurtherChannelQuota(id, channel.Id, 0, 40) })
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	require.Equal(t, 1, successes)
	policy, err := GetFurtherChannelPolicy(channel.Id)
	require.NoError(t, err)
	require.Equal(t, int64(10), policy.QuotaRemaining)
	require.Equal(t, int64(40), policy.QuotaReserved)
}
