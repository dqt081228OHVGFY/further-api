package model

import (
	"context"
	"errors"
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/setting"
	"github.com/dqt081228OHVGFY/further-api/setting/ratio_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGatewayOptionsInvalidBatchNeverChangesDatabaseOrRuntime(t *testing.T) {
	db := businessCommandDatabase(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	previousServer, previousMaximum := common.SMTPServer, setting.GetMaxTokenAutoGroups()
	err := UpdateOptionsBulkContext(context.Background(), map[string]string{"SMTPServer": "changed.example.invalid", "MaxTokenAutoGroups": "0"})
	require.Error(t, err)
	require.Equal(t, previousServer, common.SMTPServer)
	require.Equal(t, previousMaximum, setting.GetMaxTokenAutoGroups())
	var count int64
	require.NoError(t, db.Model(&Option{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestGatewayOptionsWriteFailureAndCancellationKeepWholeGeneration(t *testing.T) {
	db := businessCommandDatabase(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	previousServer, previousUnit := common.SMTPServer, common.QuotaPerUnit
	common.OptionMapRWMutex.RLock()
	previousOptions := maps.Clone(common.OptionMap)
	common.OptionMapRWMutex.RUnlock()
	t.Cleanup(func() {
		common.SMTPServer, common.QuotaPerUnit = previousServer, previousUnit
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	failure := errors.New("configuration write unavailable")
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("gateway_configuration_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "options" {
			tx.AddError(failure)
		}
	}))
	values := map[string]string{"SMTPServer": "changed.example.invalid", "QuotaPerUnit": "600000"}
	require.ErrorIs(t, UpdateOptionsBulkContext(context.Background(), values), failure)
	require.Equal(t, previousServer, common.SMTPServer)
	require.Equal(t, previousUnit, common.QuotaPerUnit)
	var count int64
	require.NoError(t, db.Model(&Option{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Callback().Update().Remove("gateway_configuration_failure"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, UpdateOptionsBulkContext(ctx, values), context.Canceled)
	require.NoError(t, UpdateOptionsBulkContext(context.Background(), values))
	require.Equal(t, "changed.example.invalid", common.SMTPServer)
	require.Equal(t, float64(600000), common.QuotaPerUnit)
	require.NoError(t, db.Model(&Option{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
}

func TestGatewayOptionReloadRejectsMalformedGenerationBeforePublishing(t *testing.T) {
	db := businessCommandDatabase(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	previousServer := common.SMTPServer
	previousDiscord := *system_setting.GetDiscordSettings()
	require.NoError(t, db.Create(&[]Option{{Key: "SMTPServer", Value: "must-not-publish.invalid"}, {Key: "discord.enabled", Value: "invalid-boolean"}}).Error)
	require.Error(t, ReloadGatewayOptionsContext(context.Background()))
	require.Equal(t, previousServer, common.SMTPServer)
	require.Equal(t, previousDiscord, *system_setting.GetDiscordSettings())
}

func TestGatewayOptionReloadKeepsConfigurationCommittedWhileRefreshWaits(t *testing.T) {
	for _, test := range []struct {
		name, key, before, after string
		writer                   *sync.Mutex
	}{
		{"gateway", "SMTPServer", "before.example.invalid", "after.example.invalid", &gatewayOptionMutationMu},
		{"request-policy", "RetryTimes", "2", "5", &requestPolicyOptionMutex},
		{"passkey-domain", "ServerAddress", "https://before.example.invalid", "https://after.example.invalid", &passkeyOptionMutex},
		{"model-pricing", "ModelRatio", `{"refresh-test-model":1}`, `{"refresh-test-model":2}`, &modelPricingMutationMu},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := businessCommandDatabase(t)
			require.NoError(t, db.AutoMigrate(&Option{}))
			previousValue := gatewayOptionBindings()[test.key].read()
			previousPolicy := requestPolicySnapshot.Load()
			common.OptionMapRWMutex.RLock()
			previousOptions := maps.Clone(common.OptionMap)
			common.OptionMapRWMutex.RUnlock()
			t.Cleanup(func() {
				require.NoError(t, updateOptionMap(test.key, previousValue))
				requestPolicySnapshot.Store(previousPolicy)
				common.OptionMapRWMutex.Lock()
				common.OptionMap = previousOptions
				common.OptionMapRWMutex.Unlock()
			})
			require.NoError(t, db.Create(&Option{Key: test.key, Value: test.before}).Error)
			require.NoError(t, updateOptionMap(test.key, test.before))
			require.NoError(t, refreshRequestPolicySnapshot())

			// A query already completed by an earlier refresh remains paused
			// until the in-flight writer commits. A correctly ordered refresh
			// cannot take that old snapshot while the writer owns its lock.
			querySeen, releaseRead := make(chan struct{}, 1), make(chan struct{})
			type refreshContextKey struct{}
			ctx := context.WithValue(context.Background(), refreshContextKey{}, true)
			require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:pause-option-refresh", func(tx *gorm.DB) {
				if tx.Statement.Table != "options" || tx.Statement.Context.Value(refreshContextKey{}) != true {
					return
				}
				querySeen <- struct{}{}
				<-releaseRead
			}))
			test.writer.Lock()
			done := make(chan error, 1)
			go func() { done <- ReloadGatewayOptionsContext(ctx) }()
			select {
			case <-querySeen:
			case <-time.After(500 * time.Millisecond):
			}
			// This is the writer's commit and publication while its existing
			// mutation lock is held; the refresh must retain these new values.
			writeErr := db.Save(&Option{Key: test.key, Value: test.after}).Error
			publishErr := updateOptionMap(test.key, test.after)
			policyErr := refreshRequestPolicySnapshot()
			close(releaseRead)
			test.writer.Unlock()
			require.NoError(t, writeErr)
			require.NoError(t, publishErr)
			require.NoError(t, policyErr)
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("configuration refresh did not complete after the writer committed")
			}
			common.OptionMapRWMutex.RLock()
			actual := common.OptionMap[test.key]
			common.OptionMapRWMutex.RUnlock()
			require.Equal(t, test.after, actual, "refresh must not publish a snapshot from before the writer's commit")
			switch test.key {
			case "SMTPServer":
				require.Equal(t, test.after, common.SMTPServer)
			case "RetryTimes":
				require.Equal(t, 5, CurrentRequestPolicy().RetryTimes)
				require.Equal(t, 5, common.RetryTimes)
			case "ServerAddress":
				require.Equal(t, test.after, system_setting.ServerAddress)
			case "ModelRatio":
				ratio, available, _ := ratio_setting.GetModelRatio("refresh-test-model")
				require.True(t, available)
				require.Equal(t, float64(2), ratio)
			}
		})
	}
}
