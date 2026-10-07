package model

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var channelRoutingRepairLock sync.Mutex

func canonicalChannelEntries(raw string) []string {
	entries := make([]string, 0)
	seen := make(map[string]bool)
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry != "" && !seen[entry] {
			seen[entry] = true
			entries = append(entries, entry)
		}
	}
	return entries
}

func channelRoutingEntries(channel *Channel) []Ability {
	groups, models := channel.GetGroups(), channel.GetModels()
	entries := make([]Ability, 0, len(groups)*len(models))
	for _, group := range groups {
		for _, name := range models {
			entries = append(entries, Ability{Group: group, Model: name, ChannelId: channel.Id, Enabled: channel.Status == common.ChannelStatusEnabled, Priority: channel.Priority, Weight: uint(channel.GetWeight()), Tag: channel.Tag})
		}
	}
	return entries
}

func writeChannelRoutingIndex(tx *gorm.DB, channel *Channel, replace bool) error {
	if channel == nil || channel.Id <= 0 {
		return errors.New("渠道编号无效")
	}
	if replace {
		if err := tx.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error; err != nil {
			return err
		}
	}
	entries := channelRoutingEntries(channel)
	if len(entries) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&entries, 50).Error
}

func (channel *Channel) AddAbilities(tx *gorm.DB) error {
	if tx != nil {
		return writeChannelRoutingIndex(tx, channel, false)
	}
	return DB.Transaction(func(tx *gorm.DB) error { return writeChannelRoutingIndex(tx, channel, false) })
}

func (channel *Channel) UpdateAbilities(tx *gorm.DB) error {
	if tx != nil {
		return writeChannelRoutingIndex(tx, channel, true)
	}
	return DB.Transaction(func(tx *gorm.DB) error { return writeChannelRoutingIndex(tx, channel, true) })
}

func (channel *Channel) DeleteAbilities() error {
	if channel == nil || channel.Id <= 0 {
		return errors.New("渠道编号无效")
	}
	return DB.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
}

func BatchInsertChannels(channels []Channel) error {
	if len(channels) == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		for start := 0; start < len(channels); start += 50 {
			end := min(start+50, len(channels))
			batch := channels[start:end]
			if err := tx.Create(&batch).Error; err != nil {
				return err
			}
			for index := range batch {
				if err := writeChannelRoutingIndex(tx, &batch[index], false); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (channel *Channel) Insert() error {
	if channel == nil {
		return errors.New("渠道参数无效")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(channel).Error; err != nil {
			return err
		}
		return writeChannelRoutingIndex(tx, channel, false)
	})
}

func normalizeChannelKeyState(channel *Channel) {
	channel.Keys = nil
	if !channel.ChannelInfo.IsMultiKey {
		return
	}
	info := &channel.ChannelInfo
	info.MultiKeySize = len(channel.GetKeys())
	for index := range info.MultiKeyStatusList {
		if index < 0 || index >= info.MultiKeySize {
			delete(info.MultiKeyStatusList, index)
		}
	}
	for index := range info.MultiKeyDisabledReason {
		if index < 0 || index >= info.MultiKeySize {
			delete(info.MultiKeyDisabledReason, index)
		}
	}
	for index := range info.MultiKeyDisabledTime {
		if index < 0 || index >= info.MultiKeySize {
			delete(info.MultiKeyDisabledTime, index)
		}
	}
	if info.MultiKeySize == 0 || info.MultiKeyPollingIndex < 0 {
		info.MultiKeyPollingIndex = 0
	} else {
		info.MultiKeyPollingIndex %= info.MultiKeySize
	}
}

// UpdateChannelCatalog writes configuration and its routing index in one
// transaction. The HTTP path supplies field presence, allowing explicit empty
// values while excluding accounting and separately managed status fields.
func UpdateChannelCatalog(ctx context.Context, desired *Channel, provided map[string]any) error {
	if desired == nil || desired.Id <= 0 {
		return errors.New("渠道编号无效")
	}
	var stored Channel
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&stored, desired.Id).Error; err != nil {
			return err
		}
		updates := make(map[string]any)
		for _, field := range []struct {
			json, column string
			value        any
			nonzero      bool
		}{
			{"type", "type", desired.Type, desired.Type != 0},
			{"name", "name", desired.Name, desired.Name != ""},
			{"key", "key", desired.Key, desired.Key != ""},
			{"openai_organization", "openai_organization", desired.OpenAIOrganization, desired.OpenAIOrganization != nil},
			{"test_model", "test_model", desired.TestModel, desired.TestModel != nil},
			{"weight", "weight", desired.Weight, desired.Weight != nil},
			{"base_url", "base_url", desired.BaseURL, desired.BaseURL != nil},
			{"other", "other", desired.Other, desired.Other != ""},
			{"models", "models", desired.Models, desired.Models != ""},
			{"group", "group", desired.Group, desired.Group != ""},
			{"model_mapping", "model_mapping", desired.ModelMapping, desired.ModelMapping != nil},
			{"status_code_mapping", "status_code_mapping", desired.StatusCodeMapping, desired.StatusCodeMapping != nil},
			{"priority", "priority", desired.Priority, desired.Priority != nil},
			{"auto_ban", "auto_ban", desired.AutoBan, desired.AutoBan != nil},
			{"other_info", "other_info", desired.OtherInfo, desired.OtherInfo != ""},
			{"tag", "tag", desired.Tag, desired.Tag != nil},
			{"setting", "setting", desired.Setting, desired.Setting != nil},
			{"settings", "settings", desired.OtherSettings, desired.OtherSettings != ""},
			{"param_override", "param_override", desired.ParamOverride, desired.ParamOverride != nil},
			{"header_override", "header_override", desired.HeaderOverride, desired.HeaderOverride != nil},
			{"remark", "remark", desired.Remark, desired.Remark != nil},
		} {
			_, present := provided[field.json]
			if (provided != nil && present) || (provided == nil && field.nonzero) {
				if field.json == "key" && desired.Key == "" {
					continue
				}
				updates[field.column] = field.value
			}
		}
		if provided == nil && desired.Status != 0 {
			updates["status"] = desired.Status
		}
		// General edits preserve the live key status map. Key-management
		// operations deliberately supply their own state through Update().
		if provided == nil && desired.ChannelInfo.IsMultiKey {
			stored.ChannelInfo = desired.ChannelInfo
		} else if provided != nil && desired.ChannelInfo.MultiKeyMode != "" {
			stored.ChannelInfo.MultiKeyMode = desired.ChannelInfo.MultiKeyMode
		}
		if key, present := updates["key"]; present {
			stored.Key = key.(string)
		}
		normalizeChannelKeyState(&stored)
		if stored.ChannelInfo.IsMultiKey {
			updates["channel_info"] = stored.ChannelInfo
		}
		if err := tx.Model(&Channel{}).Where("id = ?", stored.Id).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.First(&stored, stored.Id).Error; err != nil {
			return err
		}
		return writeChannelRoutingIndex(tx, &stored, true)
	})
	if err == nil {
		*desired = stored
	}
	return err
}

func (channel *Channel) Update() error {
	return UpdateChannelCatalog(context.Background(), channel, nil)
}

func BatchDeleteChannels(ids []int) (int64, error) {
	unique, seen := make([]int, 0, len(ids)), make(map[int]bool, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return 0, errors.New("渠道编号无效")
		}
		if !seen[id] {
			unique, seen[id] = append(unique, id), true
		}
	}
	var deleted int64
	sort.Ints(unique)
	err := DB.Transaction(func(tx *gorm.DB) error {
		for start := 0; start < len(unique); start += 200 {
			batch := unique[start:min(start+200, len(unique))]
			var locked []Channel
			if err := lockForUpdate(tx).Select("id").Where("id IN ?", batch).Order("id").Find(&locked).Error; err != nil {
				return err
			}
			if err := tx.Where("channel_id IN ?", batch).Delete(&Ability{}).Error; err != nil {
				return err
			}
			result := tx.Where("id IN ?", batch).Delete(&Channel{})
			if result.Error != nil {
				return result.Error
			}
			deleted += result.RowsAffected
		}
		return nil
	})
	return deleted, err
}

func (channel *Channel) Delete() error {
	if channel == nil || channel.Id <= 0 {
		return errors.New("渠道编号无效")
	}
	_, err := BatchDeleteChannels([]int{channel.Id})
	return err
}

func FixAbility() (int, int, error) {
	if !channelRoutingRepairLock.TryLock() {
		return 0, 0, errors.New("已有渠道索引修复任务正在运行")
	}
	defer channelRoutingRepairLock.Unlock()
	count, targets := 0, 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		var channels []Channel
		if err := lockForUpdate(tx).Order("id").Find(&channels).Error; err != nil {
			return err
		}
		targets = len(channels)
		for index := range channels {
			if err := writeChannelRoutingIndex(tx, &channels[index], true); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	if err != nil {
		return 0, targets, err
	}
	InitChannelCache()
	return count, 0, nil
}
