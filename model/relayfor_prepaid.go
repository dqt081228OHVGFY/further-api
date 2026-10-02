package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
)

// A prepaid grant uses New API's expiring subscription ledger. It never adds
// permanent wallet credit; its Token is a normal, owner-scoped New API token.
type RelayForPrepaidCard struct {
	ID             int            `json:"id" gorm:"primaryKey"`
	BatchID        string         `json:"batch_id" gorm:"size:64;index"`
	Code           string         `json:"code" gorm:"size:80;uniqueIndex"`
	CodeHash       string         `json:"-" gorm:"size:64;uniqueIndex"`
	Name           string         `json:"name" gorm:"size:320"`
	UserGroup      string         `json:"user_group" gorm:"size:64"`
	Quota          int            `json:"quota"`
	ValidityDays   int            `json:"validity_days"`
	Status         int            `json:"status"`
	CreatedBy      int            `json:"-"`
	CreatedAt      int64          `json:"-" gorm:"bigint"`
	ClaimExpiresAt int64          `json:"-" gorm:"bigint"`
	RedeemedAt     int64          `json:"-" gorm:"bigint"`
	UsageExpiresAt int64          `json:"-" gorm:"bigint"`
	RedeemedBy     int            `json:"-" gorm:"index"`
	PlanID         int            `json:"-"`
	SubscriptionID int            `json:"-" gorm:"index"`
	TokenID        int            `json:"-"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index"`
}

type RelayForPrepaidGenerateInput struct {
	Name           string
	UserGroup      string
	Count          int
	Quota          int
	ValidityDays   int
	ClaimExpiresAt int64
	CreatedBy      int
}

func GenerateRelayForPrepaidCards(input RelayForPrepaidGenerateInput) ([]RelayForPrepaidCard, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.UserGroup = strings.TrimSpace(input.UserGroup)
	if input.Name == "" || len([]rune(input.Name)) > 80 || input.UserGroup == "" || len(input.UserGroup) > 64 ||
		input.Count < 1 || input.Count > 100 || input.Quota <= 0 || input.ValidityDays < 1 || input.ValidityDays > 3650 ||
		(input.ClaimExpiresAt != 0 && input.ClaimExpiresAt <= common.GetTimestamp()) {
		return nil, errors.New("额度卡参数无效：数量 1–100、有效期 1–3650 天，额度必须为正数")
	}
	if err := common.ValidateWalletQuota(input.Quota); err != nil {
		return nil, err
	}
	batch, err := common.GenerateRandomCharsKey(24)
	if err != nil {
		return nil, err
	}
	cards := make([]RelayForPrepaidCard, input.Count)
	for index := range cards {
		code, err := common.GenerateRandomCharsKey(48)
		if err != nil {
			return nil, err
		}
		fullCode := "rfp_" + code
		codeHash := sha256.Sum256([]byte(fullCode))
		cards[index] = RelayForPrepaidCard{BatchID: batch, Code: fullCode, CodeHash: hex.EncodeToString(codeHash[:]), Name: input.Name,
			UserGroup: input.UserGroup, Quota: input.Quota, ValidityDays: input.ValidityDays,
			Status: 1, CreatedBy: input.CreatedBy, CreatedAt: common.GetTimestamp(), ClaimExpiresAt: input.ClaimExpiresAt}
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		allowBalance, allowOverflow := false, true
		plan := SubscriptionPlan{Title: input.Name, Currency: "USD", DurationUnit: SubscriptionDurationDay,
			DurationValue: input.ValidityDays, TotalAmount: int64(input.Quota), QuotaResetPeriod: SubscriptionResetNever,
			AllowBalancePay: &allowBalance, AllowWalletOverflow: &allowOverflow}
		if err := tx.Create(&plan).Error; err != nil {
			return err
		}
		// SubscriptionPlan's legacy default:true needs an explicit update to make
		// this fulfillment-only plan unavailable in the purchase catalog.
		if err := tx.Model(&plan).Update("enabled", false).Error; err != nil {
			return err
		}
		for index := range cards {
			cards[index].PlanID = plan.Id
		}
		return tx.Create(&cards).Error
	})
	if err == nil {
		InvalidateSubscriptionPlanCache(cards[0].PlanID)
	}
	return cards, err
}

func prepaidTime(timestamp int64) any {
	if timestamp <= 0 {
		return nil
	}
	return time.Unix(timestamp, 0).UTC().Format(time.RFC3339)
}

func prepaidCardView(card RelayForPrepaidCard) map[string]any {
	status := card.Status
	if status == 1 && card.ClaimExpiresAt > 0 && card.ClaimExpiresAt <= common.GetTimestamp() {
		status = 3
	}
	identity := ""
	if card.RedeemedBy > 0 {
		identity = fmt.Sprintf("账户 #%d", card.RedeemedBy)
	}
	return map[string]any{"id": card.ID, "batch_id": card.BatchID, "code": card.Code, "name": card.Name,
		"user_group": card.UserGroup, "quota": card.Quota, "validity_days": card.ValidityDays, "status": status,
		"created_at": prepaidTime(card.CreatedAt), "claim_expires_at": prepaidTime(card.ClaimExpiresAt),
		"usage_expires_at": prepaidTime(card.UsageExpiresAt), "redeemed_at": prepaidTime(card.RedeemedAt),
		"redeemed_by_identity": identity}
}

func ListRelayForPrepaidCards(page, size int, keyword, status string) (map[string]any, error) {
	if page < 1 || size < 1 || size > 100 {
		return nil, errors.New("无效的分页参数")
	}
	query := DB.Model(&RelayForPrepaidCard{})
	if keyword != "" {
		if len(keyword) == 52 && strings.HasPrefix(keyword, "rfp_") {
			digest := sha256.Sum256([]byte(keyword))
			query = query.Where("code_hash = ?", hex.EncodeToString(digest[:]))
		} else {
			query = query.Where("name LIKE ? OR batch_id = ?", "%"+keyword+"%", keyword)
		}
	}
	now := common.GetTimestamp()
	switch status {
	case "1":
		query = query.Where("status = 1 AND (claim_expires_at = 0 OR claim_expires_at > ?)", now)
	case "2":
		query = query.Where("status = 2")
	case "3", "expired":
		query = query.Where("status = 1 AND claim_expires_at > 0 AND claim_expires_at <= ?", now)
	case "":
	default:
		return nil, errors.New("无效的额度卡状态")
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Distinct("batch_id").Count(&total).Error; err != nil {
		return nil, err
	}
	var selected []struct {
		BatchID  string
		LatestID int
	}
	if err := query.Session(&gorm.Session{}).Select("batch_id, MAX(id) AS latest_id").Group("batch_id").Order("latest_id desc").Limit(size).Offset((page - 1) * size).Scan(&selected).Error; err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(selected))
	for _, selectedBatch := range selected {
		var cards []RelayForPrepaidCard
		if err := query.Session(&gorm.Session{}).Where("batch_id = ?", selectedBatch.BatchID).Order("id asc").Find(&cards).Error; err != nil {
			return nil, err
		}
		if len(cards) == 0 {
			continue
		}
		batch := prepaidCardView(cards[0])
		delete(batch, "code")
		delete(batch, "id")
		counts := map[int]int{}
		views := make([]map[string]any, 0, len(cards))
		for _, card := range cards {
			view := prepaidCardView(card)
			counts[view["status"].(int)]++
			views = append(views, view)
		}
		batch["items"], batch["count"] = views, len(cards)
		batch["unused_count"], batch["redeemed_count"], batch["expired_count"] = counts[1], counts[2], counts[3]
		items = append(items, batch)
	}
	return map[string]any{"items": items, "total": total, "page": page, "page_size": size, "total_pages": (total + int64(size) - 1) / int64(size)}, nil
}

func DeleteRelayForPrepaidCards(id int, batch string, used bool) (int64, error) {
	query := DB.Model(&RelayForPrepaidCard{})
	if used {
		query = query.Where("status = 2 OR (status = 1 AND claim_expires_at > 0 AND claim_expires_at <= ?)", common.GetTimestamp())
	} else if id > 0 {
		query = query.Where("id = ? AND status = 1", id)
	} else if batch != "" {
		query = query.Where("batch_id = ? AND status = 1", batch)
	} else {
		return 0, errors.New("必须指定要删除的未使用额度卡或批次")
	}
	result := query.Delete(&RelayForPrepaidCard{})
	if result.Error == nil && result.RowsAffected == 0 && id > 0 {
		return 0, errors.New("额度卡不存在或已兑换，不能删除")
	}
	return result.RowsAffected, result.Error
}

func RedeemRelayForPrepaidCard(userID int, code string) (map[string]any, error) {
	code = strings.TrimSpace(code)
	if userID <= 0 || len(code) != 52 || !strings.HasPrefix(code, "rfp_") {
		return nil, errors.New("无效的额度卡")
	}
	var card RelayForPrepaidCard
	var token Token
	createdKey := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		codeHash := sha256.Sum256([]byte(code))
		if err := lockForUpdate(tx).Where("code_hash = ?", hex.EncodeToString(codeHash[:])).First(&card).Error; err != nil {
			return errors.New("额度卡无效或已删除")
		}
		now := common.GetTimestamp()
		if card.Status != 1 || (card.ClaimExpiresAt > 0 && card.ClaimExpiresAt <= now) {
			return errors.New("额度卡已使用或已过兑换期限")
		}
		var user User
		if err := lockForUpdate(tx).First(&user, userID).Error; err != nil {
			return err
		}
		if user.Status != common.UserStatusEnabled || user.LinuxDOId == "" {
			return errors.New("请通过当前账户的安全设置绑定 LinuxDO 后再兑换")
		}
		if user.Group != card.UserGroup {
			return errors.New("额度卡与当前账户的用户组不一致")
		}
		// The conditional update is also the write serialization point on SQLite,
		// whose lockForUpdate intentionally emits no FOR UPDATE clause.
		claimed := tx.Model(&RelayForPrepaidCard{}).Where("id = ? AND status = 1", card.ID).
			Updates(map[string]any{"status": 2, "redeemed_by": userID, "redeemed_at": now})
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected != 1 {
			return errors.New("额度卡已被兑换")
		}
		plan, err := getSubscriptionPlanByIdTx(tx, card.PlanID)
		if err != nil {
			return err
		}
		if plan.TotalAmount != int64(card.Quota) || NormalizeResetPeriod(plan.QuotaResetPeriod) != SubscriptionResetNever {
			return errors.New("额度卡资金计划与发行快照不一致，请联系管理员")
		}
		// Use the existing subscription accounting table with the immutable card
		// snapshot. Do not query global DB time from inside this transaction (it
		// deadlocks single-connection SQLite databases).
		subscription := &UserSubscription{UserId: userID, PlanId: card.PlanID, AmountTotal: int64(card.Quota),
			StartTime: now, EndTime: now + int64(card.ValidityDays)*86400, Status: "active", Source: "relayfor_prepaid", AllowWalletOverflow: true}
		if err := tx.Create(subscription).Error; err != nil {
			return err
		}
		var prior RelayForPrepaidCard
		priorResult := tx.Unscoped().Where("redeemed_by = ? AND user_group = ? AND status = 2 AND token_id > 0", userID, card.UserGroup).Order("id desc").First(&prior)
		if priorResult.Error != nil && !errors.Is(priorResult.Error, gorm.ErrRecordNotFound) {
			return priorResult.Error
		}
		foundToken := false
		if prior.TokenID > 0 {
			tokenResult := lockForUpdate(tx).Where("id = ? AND user_id = ?", prior.TokenID, userID).First(&token)
			if tokenResult.Error != nil && !errors.Is(tokenResult.Error, gorm.ErrRecordNotFound) {
				return tokenResult.Error
			}
			foundToken = tokenResult.Error == nil && token.Group == card.UserGroup && !token.UnlimitedQuota
		}
		if !foundToken {
			var tokenCount int64
			if err := tx.Model(&Token{}).Where("user_id = ?", userID).Count(&tokenCount).Error; err != nil {
				return err
			}
			if tokenCount >= int64(operation_setting.GetMaxUserTokens()) {
				return errors.New("已达到最大令牌数量限制，请先清理无用 API Key")
			}
			key, err := common.GenerateKey()
			if err != nil {
				return err
			}
			token = Token{UserId: userID, Name: "额度卡 · " + card.UserGroup, Key: key, Status: common.TokenStatusEnabled,
				CreatedTime: now, AccessedTime: now, ExpiredTime: subscription.EndTime, RemainQuota: card.Quota, Group: card.UserGroup}
			if err := tx.Create(&token).Error; err != nil {
				return err
			}
			createdKey = true
		} else {
			if token.RemainQuota > common.MaxWalletQuota-card.Quota {
				return errors.New("额度卡 Key 额度超过安全上限")
			}
			if err := invalidateTokenCacheForMutation(token.Key); err != nil {
				return err
			}
			updates := map[string]any{"remain_quota": gorm.Expr("remain_quota + ?", card.Quota), "expired_time": max(token.ExpiredTime, subscription.EndTime), "unlimited_quota": false}
			if token.Status == common.TokenStatusExpired || token.Status == common.TokenStatusExhausted {
				updates["status"] = common.TokenStatusEnabled
			}
			if err := tx.Model(&Token{}).Where("id = ? AND user_id = ?", token.Id, userID).Updates(updates).Error; err != nil {
				return err
			}
		}
		card.UsageExpiresAt, card.SubscriptionID, card.TokenID = subscription.EndTime, subscription.Id, token.Id
		return tx.Model(&RelayForPrepaidCard{}).Where("id = ?", card.ID).Updates(map[string]any{
			"usage_expires_at": card.UsageExpiresAt, "subscription_id": card.SubscriptionID, "token_id": card.TokenID}).Error
	})
	if err != nil {
		return nil, err
	}
	if err := invalidateTokenCacheForMutation(token.Key); err != nil {
		common.SysError("prepaid token cache invalidation failed: " + err.Error())
	}
	RecordLog(userID, LogTypeTopup, fmt.Sprintf("兑换额度卡 #%d，限时额度 %d，独立 Key #%d", card.ID, card.Quota, card.TokenID))
	result := map[string]any{"redeem_type": "prepaid", "added_quota": card.Quota, "expires_at": prepaidTime(card.UsageExpiresAt),
		"token_id": card.TokenID, "user_group": card.UserGroup, "registered_user": true, "merged": !createdKey,
		"funding_source": "subscription", "subscription_id": card.SubscriptionID}
	if createdKey {
		result["api_key"] = token.GetFullKey()
	}
	return result, nil
}

func GetRelayForPrepaidBalance(userID int) (map[string]any, error) {
	var user User
	if userID <= 0 {
		return nil, errors.New("请先登录")
	}
	if err := DB.First(&user, userID).Error; err != nil {
		return nil, err
	}
	if user.Status != common.UserStatusEnabled || user.LinuxDOId == "" {
		return nil, errors.New("请先绑定 LinuxDO")
	}
	var cards []RelayForPrepaidCard
	if err := DB.Unscoped().Where("redeemed_by = ? AND user_group = ? AND status = 2", userID, user.Group).Order("id desc").Find(&cards).Error; err != nil {
		return nil, err
	}
	buckets := make([]map[string]any, 0, len(cards))
	total, expires := int64(0), int64(0)
	var latestToken Token
	for _, card := range cards {
		var sub UserSubscription
		if err := DB.Where("id = ? AND user_id = ?", card.SubscriptionID, userID).First(&sub).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return nil, err
		}
		remaining := int64(0)
		if sub.Status == "active" && sub.EndTime > common.GetTimestamp() {
			remaining = max(int64(0), sub.AmountTotal-sub.AmountUsed)
		}
		if total > int64(common.MaxWalletQuota)-remaining {
			return nil, errors.New("额度卡合计额度超过安全显示上限")
		}
		total += remaining
		expires = max(expires, sub.EndTime)
		buckets = append(buckets, map[string]any{"name": card.Name, "quota_total": sub.AmountTotal, "quota_remaining": remaining, "expires_at": prepaidTime(sub.EndTime)})
		if latestToken.Id == 0 {
			query := DB.Where("id = ? AND user_id = ?", card.TokenID, userID).First(&latestToken)
			if query.Error != nil && !errors.Is(query.Error, gorm.ErrRecordNotFound) {
				return nil, query.Error
			}
		}
	}
	keyStatus := latestToken.Status
	if latestToken.ExpiredTime > 0 && latestToken.ExpiredTime <= common.GetTimestamp() {
		keyStatus = common.TokenStatusExpired
	} else if latestToken.Id > 0 && latestToken.RemainQuota <= 0 {
		keyStatus = common.TokenStatusExhausted
	}
	return map[string]any{"total_remaining": total, "expires_at": prepaidTime(expires), "key_hint": latestToken.GetMaskedKey(),
		"key_status": keyStatus, "buckets": buckets, "billing_preference": common.NormalizeBillingPreference(user.GetSetting().BillingPreference)}, nil
}
