package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
)

type FurtherLimitValues struct {
	RequestsPerMinute  int   `json:"requests_per_minute"`
	ConcurrentRequests int   `json:"concurrent_requests"`
	QuotaTotal         int64 `json:"quota_total"`
	QuotaRemaining     int64 `json:"quota_remaining"`
	QuotaReserved      int64 `json:"quota_reserved"`
}

type FurtherChannelPolicy struct {
	ChannelID int `json:"channel_id" gorm:"primaryKey"`
	FurtherLimitValues
}

type FurtherChannelKeyPolicy struct {
	ID           int    `json:"id" gorm:"primaryKey"`
	ChannelID    int    `json:"channel_id" gorm:"uniqueIndex:idx_further_channel_key_digest"`
	Digest       string `json:"-" gorm:"size:64;uniqueIndex:idx_further_channel_key_digest"`
	Label        string `json:"label" gorm:"size:320"`
	Weight       int    `json:"weight"`
	CurrentIndex int    `json:"current_index"`
	Status       int    `json:"status"`
	KeyHint      string `json:"key" gorm:"-"`
	FurtherLimitValues
}

type FurtherChannelQuotaReservation struct {
	ID             string `gorm:"primaryKey;size:128"`
	ChannelID      int    `gorm:"index"`
	KeyID          int    `gorm:"index"`
	ReservedQuota  int64
	ActualQuota    int64
	ChannelCharged bool
	KeyCharged     bool
	State          string `gorm:"size:16"`
}

func defaultFurtherLimits() FurtherLimitValues {
	return FurtherLimitValues{QuotaTotal: -1, QuotaRemaining: -1}
}

func validFurtherLimitValues(values FurtherLimitValues) error {
	if values.RequestsPerMinute < 0 || values.RequestsPerMinute > 1000000 || values.ConcurrentRequests < 0 || values.ConcurrentRequests > 100000 ||
		values.QuotaTotal < -1 || values.QuotaTotal > int64(common.MaxWalletQuota) || values.QuotaRemaining < -1 || values.QuotaRemaining > int64(common.MaxWalletQuota) ||
		(values.QuotaTotal == -1 && values.QuotaRemaining != -1) || (values.QuotaTotal >= 0 && (values.QuotaRemaining < 0 || values.QuotaRemaining > values.QuotaTotal)) {
		return errors.New("RPM、并发或额度超出允许范围；不限额请同时填写 -1")
	}
	return nil
}

func GetFurtherChannelPolicy(channelID int) (FurtherChannelPolicy, error) {
	policy := FurtherChannelPolicy{ChannelID: channelID, FurtherLimitValues: defaultFurtherLimits()}
	if channelID <= 0 {
		return policy, errors.New("无效的渠道 ID")
	}
	var channel Channel
	if err := DB.Select("id").First(&channel, channelID).Error; err != nil {
		return policy, err
	}
	err := DB.Where("channel_id = ?", channelID).Limit(1).Find(&policy).Error
	return policy, err
}

func ListFurtherChannelPolicies() ([]FurtherChannelPolicy, error) {
	policies := []FurtherChannelPolicy{}
	err := DB.Find(&policies).Error
	return policies, err
}

func SaveFurtherChannelPolicy(policy FurtherChannelPolicy) error {
	if err := validFurtherLimitValues(policy.FurtherLimitValues); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var channel Channel
		if err := lockForUpdate(tx).Select("id").First(&channel, policy.ChannelID).Error; err != nil {
			return err
		}
		var current FurtherChannelPolicy
		query := lockForUpdate(tx).Where("channel_id = ?", policy.ChannelID).Limit(1).Find(&current)
		if query.Error != nil {
			return query.Error
		}
		if current.QuotaReserved > 0 && (policy.QuotaTotal != current.QuotaTotal || policy.QuotaRemaining != current.QuotaRemaining) {
			return errors.New("渠道仍有计费预留，不能修改额度余额")
		}
		policy.QuotaReserved = current.QuotaReserved
		return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&policy).Error
	})
}

func FurtherChannelKeyDigest(key string) string {
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:])
}

func furtherActualChannelKeys(channel *Channel) []string {
	if channel.ChannelInfo.IsMultiKey {
		return channel.GetKeys()
	}
	if channel.Key == "" {
		return nil
	}
	return []string{channel.Key}
}

func syncFurtherChannelKeys(tx *gorm.DB, channel *Channel) ([]FurtherChannelKeyPolicy, error) {
	keys := furtherActualChannelKeys(channel)
	result := make([]FurtherChannelKeyPolicy, 0, len(keys))
	seen := map[string]bool{}
	for index, key := range keys {
		digest := FurtherChannelKeyDigest(key)
		if seen[digest] {
			return nil, errors.New("渠道包含重复 Key，请先在渠道编辑中去重")
		}
		seen[digest] = true
		policy := FurtherChannelKeyPolicy{ChannelID: channel.Id, Digest: digest, Weight: 1, Status: common.ChannelStatusEnabled,
			CurrentIndex: index, FurtherLimitValues: defaultFurtherLimits()}
		if status, exists := channel.ChannelInfo.MultiKeyStatusList[index]; exists && channel.ChannelInfo.IsMultiKey {
			policy.Status = status
		}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "channel_id"}, {Name: "digest"}}, DoNothing: true}).Create(&policy).Error; err != nil {
			return nil, err
		}
		if err := tx.Where("channel_id = ? AND digest = ?", channel.Id, digest).First(&policy).Error; err != nil {
			return nil, err
		}
		policy.CurrentIndex = index
		policy.Status = common.ChannelStatusEnabled
		if status, exists := channel.ChannelInfo.MultiKeyStatusList[index]; exists && channel.ChannelInfo.IsMultiKey {
			policy.Status = status
		}
		if !channel.ChannelInfo.IsMultiKey {
			policy.Status = channel.Status
		}
		if err := tx.Model(&policy).Updates(map[string]any{"current_index": index, "status": policy.Status}).Error; err != nil {
			return nil, err
		}
		policy.KeyHint = MaskTokenKey(strings.ReplaceAll(key, "\n", ""))
		result = append(result, policy)
	}
	return result, nil
}

func GetFurtherChannelKeys(channelID int) ([]FurtherChannelKeyPolicy, error) {
	var result []FurtherChannelKeyPolicy
	err := DB.Transaction(func(tx *gorm.DB) error {
		var channel Channel
		if err := lockForUpdate(tx).First(&channel, channelID).Error; err != nil {
			return err
		}
		var err error
		result, err = syncFurtherChannelKeys(tx, &channel)
		return err
	})
	return result, err
}

// furtherKeySelectionWeights maps the current upstream key order to stable
// digest-backed policies. Keys added through the native further-api editor retain
// its ordinary equal-weight behavior until an administrator configures them.
// No credential is persisted or returned by this lookup.
func furtherKeySelectionWeights(channelID int, keys []string) ([]int64, error) {
	weights := make([]int64, len(keys))
	for index := range weights {
		weights[index] = 1
	}
	if DB == nil || channelID <= 0 || !DB.Migrator().HasTable(&FurtherChannelKeyPolicy{}) {
		return weights, nil
	}
	var policies []FurtherChannelKeyPolicy
	if err := DB.Select("digest", "weight").Where("channel_id = ?", channelID).Find(&policies).Error; err != nil {
		return nil, err
	}
	configured := make(map[string]int64, len(policies))
	for _, policy := range policies {
		if policy.Weight > 1 {
			configured[policy.Digest] = int64(policy.Weight)
		}
	}
	for index, key := range keys {
		if weight := configured[FurtherChannelKeyDigest(key)]; weight > 1 {
			weights[index] = weight
		}
	}
	return weights, nil
}

type FurtherChannelKeyInput struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Weight int    `json:"weight"`
	Status int    `json:"status"`
	FurtherLimitValues
}

func SaveFurtherChannelKey(channelID, keyID int, input FurtherChannelKeyInput, remove bool) (FurtherChannelKeyPolicy, error) {
	var result FurtherChannelKeyPolicy
	if !remove {
		if err := validFurtherLimitValues(input.FurtherLimitValues); err != nil {
			return result, err
		}
		if input.Weight < 1 || input.Weight > 100000 || len([]rune(input.Label)) > 80 || input.Status < 1 || input.Status > 3 {
			return result, errors.New("Key 标签、权重或状态无效")
		}
	}
	var modified Channel
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Never print the credential-bearing UPDATE statement on SQL errors.
		tx = tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)})
		if err := lockForUpdate(tx).First(&modified, channelID).Error; err != nil {
			return err
		}
		policies, err := syncFurtherChannelKeys(tx, &modified)
		if err != nil {
			return err
		}
		keys := furtherActualChannelKeys(&modified)
		index := -1
		for _, policy := range policies {
			if policy.ID == keyID {
				result, index = policy, policy.CurrentIndex
				break
			}
		}
		if keyID > 0 && index < 0 {
			return errors.New("Key 不属于此渠道或已被移除")
		}
		if result.QuotaReserved > 0 && (remove || (input.Key != "" && FurtherChannelKeyDigest(input.Key) != result.Digest) || input.QuotaTotal != result.QuotaTotal || input.QuotaRemaining != result.QuotaRemaining) {
			return errors.New("Key 仍有计费预留，不能删除、替换或修改额度")
		}
		if remove {
			if len(keys) <= 1 {
				return errors.New("不能删除渠道最后一个 Key，请停用渠道")
			}
			keys = append(keys[:index], keys[index+1:]...)
		} else {
			input.Key = strings.TrimSpace(input.Key)
			if keyID == 0 {
				if input.Key == "" {
					return errors.New("新增 Key 不能为空")
				}
				index = len(keys)
				keys = append(keys, input.Key)
				result = FurtherChannelKeyPolicy{ChannelID: channelID, Digest: FurtherChannelKeyDigest(input.Key)}
			} else if input.Key != "" {
				keys[index] = input.Key
				result.Digest = FurtherChannelKeyDigest(input.Key)
			}
			result.Label, result.Weight, result.Status, result.CurrentIndex = input.Label, input.Weight, input.Status, index
			reserved := result.QuotaReserved
			result.FurtherLimitValues = input.FurtherLimitValues
			result.QuotaReserved = reserved
			if err := tx.Save(&result).Error; err != nil {
				return errors.New("保存 Key 策略失败，可能存在重复 Key")
			}
		}
		seen := map[string]bool{}
		newStatus := map[int]int{}
		for nextIndex, key := range keys {
			digest := FurtherChannelKeyDigest(key)
			if seen[digest] {
				return errors.New("渠道不能包含重复 Key")
			}
			seen[digest] = true
			status := common.ChannelStatusEnabled
			for _, policy := range policies {
				if policy.Digest == digest {
					status = policy.Status
					break
				}
			}
			if !remove && nextIndex == index {
				status = input.Status
			}
			newStatus[nextIndex] = status
		}
		// Preserve structured service-account credentials when combining keys.
		structured := true
		jsonKeys := make([]json.RawMessage, len(keys))
		for index, key := range keys {
			var decoded any
			if common.Unmarshal([]byte(key), &decoded) != nil {
				structured = false
				break
			}
			jsonKeys[index] = json.RawMessage(key)
		}
		if len(keys) == 1 {
			modified.Key = keys[0]
		} else if structured {
			encoded, err := common.Marshal(jsonKeys)
			if err != nil {
				return err
			}
			modified.Key = string(encoded)
		} else {
			for _, key := range keys {
				if strings.ContainsAny(key, "\r\n") {
					return errors.New("多行凭据必须使用有效 JSON 格式")
				}
			}
			modified.Key = strings.Join(keys, "\n")
		}
		modified.Keys = nil
		modified.ChannelInfo.IsMultiKey = len(keys) > 1
		if len(keys) > 1 && modified.ChannelInfo.MultiKeyMode == "" {
			modified.ChannelInfo.MultiKeyMode = constant.MultiKeyModeRandom
		}
		modified.ChannelInfo.MultiKeySize = len(keys)
		modified.ChannelInfo.MultiKeyStatusList = newStatus
		modified.ChannelInfo.MultiKeyDisabledReason = nil
		modified.ChannelInfo.MultiKeyDisabledTime = nil
		if len(keys) == 1 {
			modified.Status = newStatus[0]
		}
		if err := tx.Model(&Channel{}).Where("id = ?", channelID).Updates(map[string]any{"key": modified.Key, "channel_info": modified.ChannelInfo, "status": modified.Status}).Error; err != nil {
			return err
		}
		if err := modified.UpdateAbilities(tx); err != nil {
			return err
		}
		_, err = syncFurtherChannelKeys(tx, &modified)
		return err
	})
	if err == nil {
		CacheUpdateChannel(&modified)
		if common.MemoryCacheEnabled {
			InitChannelCache()
		}
		result.KeyHint = ""
		if !remove {
			keys := furtherActualChannelKeys(&modified)
			result.KeyHint = MaskTokenKey(strings.ReplaceAll(keys[result.CurrentIndex], "\n", ""))
		}
	}
	return result, err
}

// GetFurtherSelectedResourcePolicies resolves only the selected real upstream
// key. The digest remains internal, and policy IDs survive list reordering.
func GetFurtherSelectedResourcePolicies(channelID int, key string) (FurtherChannelPolicy, *FurtherChannelKeyPolicy, error) {
	channel := FurtherChannelPolicy{ChannelID: channelID, FurtherLimitValues: defaultFurtherLimits()}
	if !DB.Migrator().HasTable(&FurtherChannelPolicy{}) {
		return channel, nil, nil
	}
	if err := DB.Where("channel_id = ?", channelID).Limit(1).Find(&channel).Error; err != nil {
		return channel, nil, err
	}
	var policy FurtherChannelKeyPolicy
	query := DB.Where("channel_id = ? AND digest = ?", channelID, FurtherChannelKeyDigest(key)).Limit(1).Find(&policy)
	if query.Error != nil {
		return channel, nil, query.Error
	}
	if query.RowsAffected == 0 {
		return channel, nil, nil
	}
	return channel, &policy, nil
}

// Reserve is an atomic, idempotent two-resource budget debit. The caller must
// use a distinct internal ID per upstream attempt and settle/refund that ID.
func ReserveFurtherChannelQuota(id string, channelID, keyID int, quota int64) error {
	if id == "" || len(id) > 128 || quota < 0 || quota > int64(common.MaxWalletQuota) {
		return errors.New("无效的渠道额度预留")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var existing FurtherChannelQuotaReservation
		query := lockForUpdate(tx).Where("id = ?", id).Limit(1).Find(&existing)
		if query.Error != nil {
			return query.Error
		}
		if query.RowsAffected > 0 {
			if existing.ChannelID == channelID && existing.KeyID == keyID && existing.ReservedQuota == quota && existing.State == "reserved" {
				return nil
			}
			return errors.New("渠道额度预留标识已使用")
		}
		reservation := FurtherChannelQuotaReservation{ID: id, ChannelID: channelID, KeyID: keyID, ReservedQuota: quota, State: "reserved"}
		// Insert the unique identity first: losing concurrent attempts cannot debit.
		if err := tx.Create(&reservation).Error; err != nil {
			return err
		}
		for _, scope := range []string{"channel", "key"} {
			var query *gorm.DB
			var remaining int64
			if scope == "channel" {
				var policy FurtherChannelPolicy
				found := lockForUpdate(tx).Where("channel_id = ?", channelID).Limit(1).Find(&policy)
				if found.Error != nil {
					return found.Error
				}
				if found.RowsAffected == 0 || policy.QuotaTotal < 0 {
					continue
				}
				remaining, reservation.ChannelCharged = policy.QuotaRemaining, true
				query = tx.Model(&FurtherChannelPolicy{}).Where("channel_id = ?", channelID)
			} else {
				if keyID == 0 {
					continue
				}
				var policy FurtherChannelKeyPolicy
				found := lockForUpdate(tx).Where("id = ? AND channel_id = ?", keyID, channelID).First(&policy)
				if found.Error != nil {
					return found.Error
				}
				if policy.QuotaTotal < 0 {
					continue
				}
				remaining, reservation.KeyCharged = policy.QuotaRemaining, true
				query = tx.Model(&FurtherChannelKeyPolicy{}).Where("id = ? AND channel_id = ?", keyID, channelID)
			}
			if remaining < quota {
				return errors.New("渠道或上游 Key 剩余额度不足")
			}
			if quota == 0 {
				continue
			}
			updated := query.Where("quota_remaining >= ?", quota).Updates(map[string]any{"quota_remaining": gorm.Expr("quota_remaining - ?", quota), "quota_reserved": gorm.Expr("quota_reserved + ?", quota)})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return errors.New("渠道额度已被并发请求预留")
			}
		}
		return tx.Model(&reservation).Updates(map[string]any{"channel_charged": reservation.ChannelCharged, "key_charged": reservation.KeyCharged}).Error
	})
}

func SettleFurtherChannelQuota(id string, actualQuota int64) error {
	if actualQuota < 0 || actualQuota > int64(common.MaxWalletQuota) {
		return errors.New("无效的渠道结算额度")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var reservation FurtherChannelQuotaReservation
		if err := lockForUpdate(tx).First(&reservation, "id = ?", id).Error; err != nil {
			return err
		}
		if reservation.State == "settled" {
			if reservation.ActualQuota == actualQuota {
				return nil
			}
			return errors.New("渠道预留已经按另一金额结算")
		}
		changed := tx.Model(&reservation).Where("state = ?", "reserved").Updates(map[string]any{"state": "settled", "actual_quota": actualQuota})
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return errors.New("渠道预留状态已变更")
		}
		delta := actualQuota - reservation.ReservedQuota
		updates := map[string]any{"quota_remaining": gorm.Expr("quota_remaining - ?", delta), "quota_reserved": gorm.Expr("quota_reserved - ?", reservation.ReservedQuota)}
		if reservation.ChannelCharged {
			if err := tx.Model(&FurtherChannelPolicy{}).Where("channel_id = ?", reservation.ChannelID).Updates(updates).Error; err != nil {
				return err
			}
		}
		if reservation.KeyCharged {
			if err := tx.Model(&FurtherChannelKeyPolicy{}).Where("id = ? AND channel_id = ?", reservation.KeyID, reservation.ChannelID).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func FurtherResourceSubject(channelID, keyID int) string {
	if keyID > 0 {
		return fmt.Sprintf("channel:%d:key:%d", channelID, keyID)
	}
	return fmt.Sprintf("channel:%d", channelID)
}
