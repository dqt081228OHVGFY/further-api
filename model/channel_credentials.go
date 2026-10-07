package model

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/rand/v2"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"gorm.io/gorm"
)

func (channel *Channel) GetKeys() []string {
	if channel == nil || channel.Key == "" {
		return []string{}
	}
	if len(channel.Keys) > 0 {
		return append([]string(nil), channel.Keys...)
	}
	raw := strings.TrimSpace(channel.Key)
	if strings.HasPrefix(raw, "[") {
		var entries []json.RawMessage
		if json.Unmarshal([]byte(raw), &entries) == nil {
			keys := make([]string, len(entries))
			for index, entry := range entries {
				keys[index] = string(entry)
			}
			return keys
		}
	}
	return strings.Split(strings.Trim(channel.Key, "\n"), "\n")
}

func cloneCredentialState(info ChannelInfo) ChannelInfo {
	copy := info
	copy.MultiKeyStatusList = make(map[int]int, len(info.MultiKeyStatusList))
	for index, status := range info.MultiKeyStatusList {
		copy.MultiKeyStatusList[index] = status
	}
	copy.MultiKeyDisabledReason = make(map[int]string, len(info.MultiKeyDisabledReason))
	for index, reason := range info.MultiKeyDisabledReason {
		copy.MultiKeyDisabledReason[index] = reason
	}
	copy.MultiKeyDisabledTime = make(map[int]int64, len(info.MultiKeyDisabledTime))
	for index, timestamp := range info.MultiKeyDisabledTime {
		copy.MultiKeyDisabledTime[index] = timestamp
	}
	return copy
}

func credentialPolicyWeights(ctx context.Context, db *gorm.DB, channelID int, keys []string) ([]int64, error) {
	weights := make([]int64, len(keys))
	for index := range weights {
		weights[index] = 1
	}
	if db == nil || channelID <= 0 {
		return weights, nil
	}
	installed := db.WithContext(ctx).Migrator().HasTable(&FurtherChannelKeyPolicy{})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !installed {
		return weights, nil
	}
	var policies []FurtherChannelKeyPolicy
	if err := db.WithContext(ctx).Select("digest", "weight").Where("channel_id = ?", channelID).Find(&policies).Error; err != nil {
		return nil, err
	}
	byDigest := make(map[string]int64, len(policies))
	for _, policy := range policies {
		if policy.Weight > 1 {
			byDigest[policy.Digest] = int64(policy.Weight)
		}
	}
	for index, key := range keys {
		if weight := byDigest[FurtherChannelKeyDigest(key)]; weight > 1 {
			weights[index] = weight
		}
	}
	return weights, nil
}

func weightedFurtherKeyIndex(enabled []int, weights []int64, draw int64) int {
	for _, index := range enabled {
		if index < 0 || index >= len(weights) || weights[index] <= 0 {
			continue
		}
		if draw < weights[index] {
			return index
		}
		draw -= weights[index]
	}
	if len(enabled) == 0 {
		return -1
	}
	return enabled[len(enabled)-1]
}

func chooseCredential(info ChannelInfo, keys []string, weights []int64) (int, int, error) {
	enabled := make([]int, 0, len(keys))
	weighted := false
	var total int64
	for index, key := range keys {
		status, present := info.MultiKeyStatusList[index]
		if strings.TrimSpace(key) == "" || present && status != common.ChannelStatusEnabled {
			continue
		}
		weight := weights[index]
		if weight <= 0 || total > math.MaxInt64-weight {
			return -1, 0, errors.New("渠道凭证权重无效")
		}
		total += weight
		weighted = weighted || weight != 1
		enabled = append(enabled, index)
	}
	if len(enabled) == 0 {
		return -1, 0, errors.New("渠道没有可用凭证")
	}
	if weighted || info.MultiKeyMode == constant.MultiKeyModeRandom {
		index := weightedFurtherKeyIndex(enabled, weights, rand.Int64N(total))
		return index, info.MultiKeyPollingIndex, nil
	}
	if info.MultiKeyMode == constant.MultiKeyModePolling {
		start := info.MultiKeyPollingIndex
		if start < 0 || start >= len(keys) {
			start = 0
		}
		for _, index := range enabled {
			if index >= start {
				return index, (index + 1) % len(keys), nil
			}
		}
		index := enabled[0]
		return index, (index + 1) % len(keys), nil
	}
	return enabled[0], info.MultiKeyPollingIndex, nil
}

func (channel *Channel) GetNextEnabledKeyContext(ctx context.Context) (string, int, *types.GatewayError) {
	if err := ctx.Err(); err != nil {
		return "", 0, types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if !channel.ChannelInfo.IsMultiKey {
		return channel.Key, 0, nil
	}
	if channel.Id <= 0 {
		keys := channel.GetKeys()
		weights := make([]int64, len(keys))
		for index := range weights {
			weights[index] = 1
		}
		selected, next, err := chooseCredential(channel.ChannelInfo, keys, weights)
		if err != nil {
			return "", 0, types.NewError(err, types.ErrorCodeChannelNoAvailableKey)
		}
		channel.ChannelInfo.MultiKeyPollingIndex = next
		return keys[selected], selected, nil
	}
	lock := GetChannelPollingLock(channel.Id)
	lock.Lock()
	defer lock.Unlock()
	var selected, next int
	var key string
	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		current, exists := channelsIDM[channel.Id]
		if !exists {
			channelSyncLock.RUnlock()
			return "", 0, types.NewError(errors.New("渠道缓存不存在"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		}
		snapshot := *current
		snapshot.ChannelInfo = cloneCredentialState(current.ChannelInfo)
		snapshot.Keys = append([]string(nil), current.Keys...)
		channelSyncLock.RUnlock()
		keys := snapshot.GetKeys()
		weights, err := credentialPolicyWeights(ctx, DB, channel.Id, keys)
		if err != nil {
			return "", 0, types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		}
		selected, next, err = chooseCredential(snapshot.ChannelInfo, keys, weights)
		if err != nil {
			return "", 0, types.NewError(err, types.ErrorCodeChannelNoAvailableKey)
		}
		key = keys[selected]
		channelSyncLock.Lock()
		if latest := channelsIDM[channel.Id]; latest != nil && latest.Key == snapshot.Key {
			latest.ChannelInfo.MultiKeyPollingIndex = next
		}
		channelSyncLock.Unlock()
	} else {
		err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var snapshot Channel
			if err := lockForUpdate(tx).Select("id", "key", "channel_info").First(&snapshot, channel.Id).Error; err != nil {
				return err
			}
			keys := snapshot.GetKeys()
			weights, err := credentialPolicyWeights(ctx, tx, channel.Id, keys)
			if err != nil {
				return err
			}
			selected, next, err = chooseCredential(snapshot.ChannelInfo, keys, weights)
			if err != nil {
				return err
			}
			key = keys[selected]
			if snapshot.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling && next != snapshot.ChannelInfo.MultiKeyPollingIndex {
				snapshot.ChannelInfo.MultiKeyPollingIndex = next
				return tx.Model(&Channel{}).Where("id = ?", channel.Id).Update("channel_info", snapshot.ChannelInfo).Error
			}
			return nil
		})
		if err != nil {
			return "", 0, types.NewError(err, types.ErrorCodeChannelNoAvailableKey)
		}
	}
	channel.ChannelInfo.MultiKeyPollingIndex = next
	return key, selected, nil
}

func (channel *Channel) GetNextEnabledKey() (string, int, *types.GatewayError) {
	return channel.GetNextEnabledKeyContext(context.Background())
}
