package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var relayForBenefitLocation = time.FixedZone("UTC+8", 8*3600)

const relayForMaxBenefitQuota = 500000000

type RelayForLoanSettings struct {
	ID                     int    `json:"-" gorm:"primaryKey"`
	Enabled                bool   `json:"enabled"`
	MaxBorrowQuota         int    `json:"max_borrow_quota"`
	MinCheckinQuota        int    `json:"min_checkin_quota"`
	DailyCutLimit          int    `json:"daily_cut_limit"`
	LossPoolVisible        bool   `json:"loss_pool_visible"`
	LossPoolTitle          string `json:"loss_pool_title" gorm:"size:320"`
	LossPoolDescription    string `json:"loss_pool_description" gorm:"size:1200"`
	LossPoolTotalQuota     int64  `json:"loss_pool_total_quota"`
	LossPoolRemainingQuota int64  `json:"loss_pool_remaining_quota"`
	LossPoolCutMinQuota    int    `json:"loss_pool_cut_min_quota"`
	LossPoolCutMaxQuota    int    `json:"loss_pool_cut_max_quota"`
}

type RelayForLoanAccount struct {
	UserID              int `gorm:"primaryKey"`
	ActiveLoanID        int64
	TemporaryBonusQuota int
}

type RelayForTokenLoan struct {
	ID             int64      `json:"id" gorm:"primaryKey"`
	UserID         int        `json:"-" gorm:"index"`
	Status         int        `json:"status"`
	Quota          int        `json:"quota"`
	RemainingQuota int        `json:"remaining_quota"`
	CheckinCount   int        `json:"checkin_count"`
	CreatedAt      time.Time  `json:"created_at"`
	CompletedAt    *time.Time `json:"completed_at"`
}

type RelayForLoanCheckin struct {
	ID             int64     `json:"id" gorm:"primaryKey"`
	UserID         int       `json:"-" gorm:"uniqueIndex:idx_relayfor_loan_user_day"`
	LoanID         int64     `json:"loan_id" gorm:"index"`
	CheckinDate    string    `json:"checkin_date" gorm:"size:10;uniqueIndex:idx_relayfor_loan_user_day"`
	AwardedQuota   int       `json:"awarded_quota"`
	RepaymentQuota int       `json:"repayment_quota"`
	OverflowQuota  int       `json:"overflow_quota"`
	RemainingAfter int       `json:"remaining_after"`
	CreatedAt      time.Time `json:"created_at"`
}

type RelayForLoanCut struct {
	ID        int64     `json:"id" gorm:"primaryKey"`
	UserID    int       `json:"-" gorm:"uniqueIndex:idx_relayfor_cut_day_sequence"`
	CutDate   string    `json:"cut_date" gorm:"size:10;uniqueIndex:idx_relayfor_cut_day_sequence"`
	Sequence  int       `json:"sequence" gorm:"uniqueIndex:idx_relayfor_cut_day_sequence"`
	Quota     int       `json:"quota"`
	CreatedAt time.Time `json:"created_at"`
}

func defaultRelayForLoanSettings() RelayForLoanSettings {
	return RelayForLoanSettings{ID: 1, MaxBorrowQuota: 500000, MinCheckinQuota: 1440000, DailyCutLimit: 2,
		LossPoolVisible: true, LossPoolTitle: "帮助站长砍一刀", LossPoolCutMinQuota: 500000, LossPoolCutMaxQuota: 1500000}
}

func relayForLoanSettings(tx *gorm.DB) (RelayForLoanSettings, error) {
	var settings RelayForLoanSettings
	err := tx.First(&settings, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultRelayForLoanSettings(), nil
	}
	return settings, err
}

func GetRelayForLoanSettings() (RelayForLoanSettings, error) { return relayForLoanSettings(DB) }

func SaveRelayForLoanSettings(settings RelayForLoanSettings) error {
	if settings.MaxBorrowQuota < 1 || settings.MaxBorrowQuota > relayForMaxBenefitQuota ||
		settings.MinCheckinQuota < 1 || settings.MinCheckinQuota > relayForMaxBenefitQuota ||
		settings.DailyCutLimit < 1 || settings.DailyCutLimit > 20 ||
		settings.LossPoolTotalQuota < 0 || settings.LossPoolTotalQuota > 500000000000 ||
		settings.LossPoolRemainingQuota < 0 || settings.LossPoolRemainingQuota > settings.LossPoolTotalQuota ||
		settings.LossPoolCutMinQuota < 1 || settings.LossPoolCutMaxQuota < settings.LossPoolCutMinQuota || settings.LossPoolCutMaxQuota > relayForMaxBenefitQuota ||
		len([]rune(settings.LossPoolTitle)) > 80 || len([]rune(settings.LossPoolDescription)) > 300 {
		return errors.New("词元贷设置超出允许范围")
	}
	settings.ID = 1
	return DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&settings).Error
}

func (s RelayForLoanSettings) View() map[string]any {
	return map[string]any{"enabled": s.Enabled, "max_borrow_quota": s.MaxBorrowQuota,
		"min_checkin_quota": s.MinCheckinQuota, "daily_cut_limit": s.DailyCutLimit,
		"loss_pool": map[string]any{"visible": s.LossPoolVisible, "title": s.LossPoolTitle,
			"description": s.LossPoolDescription, "total_quota": s.LossPoolTotalQuota,
			"remaining_quota": s.LossPoolRemainingQuota, "cut_min_quota": s.LossPoolCutMinQuota,
			"cut_max_quota": s.LossPoolCutMaxQuota}}
}

func relayForBenefitUser(tx *gorm.DB, userID int) (*User, error) {
	var user User
	if err := lockForUpdate(tx).First(&user, userID).Error; err != nil {
		return nil, err
	}
	if userID <= 0 || user.Status != common.UserStatusEnabled || user.Group != "default" {
		return nil, errors.New("此公益福利仅限正常状态的 default 用户组账户")
	}
	return &user, nil
}

func relayForLoanAccount(tx *gorm.DB, userID int) (RelayForLoanAccount, error) {
	account := RelayForLoanAccount{UserID: userID}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&account).Error; err != nil {
		return account, err
	}
	err := lockForUpdate(tx).First(&account, userID).Error
	return account, err
}

func RelayForBorrow(userID int, quota int, now time.Time) (*RelayForTokenLoan, error) {
	if userID <= 0 || quota <= 0 || quota > common.MaxWalletQuota {
		return nil, errors.New("借款额度无效")
	}
	var loan RelayForTokenLoan
	err := DB.Transaction(func(tx *gorm.DB) error {
		settings, err := relayForLoanSettings(tx)
		if err != nil {
			return err
		}
		if !settings.Enabled || settings.LossPoolRemainingQuota > 0 {
			return errors.New("词元贷当前暂停新借款")
		}
		if _, err := relayForBenefitUser(tx, userID); err != nil {
			return err
		}
		account, err := relayForLoanAccount(tx, userID)
		if err != nil {
			return err
		}
		if account.ActiveLoanID != 0 {
			return errors.New("请先结清当前借款")
		}
		if quota > settings.MaxBorrowQuota+account.TemporaryBonusQuota {
			return errors.New("超过当前可借额度")
		}
		loan = RelayForTokenLoan{UserID: userID, Status: 1, Quota: quota, RemainingQuota: quota, CreatedAt: now}
		if err := tx.Create(&loan).Error; err != nil {
			return err
		}
		result := tx.Model(&RelayForLoanAccount{}).Where("user_id = ? AND active_loan_id = 0", userID).
			Updates(map[string]any{"active_loan_id": loan.ID, "temporary_bonus_quota": 0})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("当前已有未结清借款")
		}
		return creditTopUpQuota(tx, userID, quota, nil)
	})
	if err != nil {
		return nil, err
	}
	syncCreditUserQuotaCache(userID, quota, "RelayFor token loan")
	return &loan, nil
}

func RelayForLoanCheckIn(userID int, now time.Time) (*RelayForLoanCheckin, error) {
	if userID <= 0 {
		return nil, errors.New("无效用户")
	}
	var record RelayForLoanCheckin
	err := DB.Transaction(func(tx *gorm.DB) error {
		// A committed record makes retries idempotent, including after settlement.
		day := now.In(relayForBenefitLocation).Format("2006-01-02")
		if _, err := relayForBenefitUser(tx, userID); err != nil {
			return err
		}
		err := tx.Where("user_id = ? AND checkin_date = ?", userID, day).First(&record).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		settings, err := relayForLoanSettings(tx)
		if err != nil {
			return err
		}
		account, err := relayForLoanAccount(tx, userID)
		if err != nil {
			return err
		}
		var loan RelayForTokenLoan
		if account.ActiveLoanID == 0 {
			return errors.New("当前没有待还借款")
		}
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ? AND status = 1", account.ActiveLoanID, userID).First(&loan).Error; err != nil {
			return err
		}
		award := settings.MinCheckinQuota
		if loan.CheckinCount >= 3 {
			award = max(award, loan.RemainingQuota)
		}
		repaid := min(award, loan.RemainingQuota)
		remaining := loan.RemainingQuota - repaid
		record = RelayForLoanCheckin{UserID: userID, LoanID: loan.ID, CheckinDate: day, AwardedQuota: award,
			RepaymentQuota: repaid, OverflowQuota: award - repaid, RemainingAfter: remaining, CreatedAt: now}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		updates := map[string]any{"remaining_quota": remaining, "checkin_count": loan.CheckinCount + 1}
		if remaining == 0 {
			updates["status"] = 2
			updates["completed_at"] = now
		}
		result := tx.Model(&RelayForTokenLoan{}).Where("id = ? AND checkin_count = ? AND status = 1", loan.ID, loan.CheckinCount).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("签到状态已更新，请重试")
		}
		if remaining == 0 {
			return tx.Model(&RelayForLoanAccount{}).Where("user_id = ? AND active_loan_id = ?", userID, loan.ID).
				Updates(map[string]any{"active_loan_id": 0, "temporary_bonus_quota": record.OverflowQuota}).Error
		}
		return nil
	})
	return &record, err
}

func RelayForCutLossPool(userID int, now time.Time) (*RelayForLoanCut, error) {
	if userID <= 0 {
		return nil, errors.New("无效用户")
	}
	var result RelayForLoanCut
	err := DB.Transaction(func(tx *gorm.DB) error {
		if _, err := relayForBenefitUser(tx, userID); err != nil {
			return err
		}
		var settings RelayForLoanSettings
		if err := lockForUpdate(tx).First(&settings, 1).Error; err != nil {
			return err
		}
		if !settings.LossPoolVisible || settings.LossPoolRemainingQuota <= 0 {
			return errors.New("亏损池已经清零或未开放")
		}
		day := now.In(relayForBenefitLocation).Format("2006-01-02")
		var count int64
		if err := tx.Model(&RelayForLoanCut{}).Where("user_id = ? AND cut_date = ?", userID, day).Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(settings.DailyCutLimit) {
			return errors.New("今日次数已用完")
		}
		delta, err := rand.Int(rand.Reader, big.NewInt(int64(settings.LossPoolCutMaxQuota-settings.LossPoolCutMinQuota+1)))
		if err != nil {
			return err
		}
		quota := min(int64(settings.LossPoolCutMinQuota)+delta.Int64(), settings.LossPoolRemainingQuota)
		result = RelayForLoanCut{UserID: userID, CutDate: day, Sequence: int(count) + 1, Quota: int(quota), CreatedAt: now}
		if err := tx.Create(&result).Error; err != nil {
			return err
		}
		update := tx.Model(&RelayForLoanSettings{}).Where("id = 1 AND loss_pool_remaining_quota >= ?", quota).
			Update("loss_pool_remaining_quota", gorm.Expr("loss_pool_remaining_quota - ?", quota))
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return errors.New("亏损池已更新，请重试")
		}
		return nil
	})
	return &result, err
}

func RelayForLoanStatus(userID int, now time.Time) (map[string]any, error) {
	user, err := GetUserById(userID, false)
	if err != nil {
		return nil, err
	}
	settings, err := GetRelayForLoanSettings()
	if err != nil {
		return nil, err
	}
	var account RelayForLoanAccount
	err = DB.First(&account, userID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var loan *RelayForTokenLoan
	var lastLoan RelayForTokenLoan
	err = DB.Where("user_id = ?", userID).Order("id DESC").First(&lastLoan).Error
	if err == nil {
		loan = &lastLoan
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	checkins := make([]RelayForLoanCheckin, 0)
	if loan != nil {
		if err := DB.Where("loan_id = ?", loan.ID).Order("id ASC").Limit(4).Find(&checkins).Error; err != nil {
			return nil, err
		}
	}
	day := now.In(relayForBenefitLocation).Format("2006-01-02")
	var today int64
	if err := DB.Model(&RelayForLoanCheckin{}).Where("user_id = ? AND checkin_date = ?", userID, day).Count(&today).Error; err != nil {
		return nil, err
	}
	var cuts struct {
		Count int64
		Quota int64
	}
	if err := DB.Model(&RelayForLoanCut{}).Select("COUNT(*) AS count, COALESCE(SUM(quota),0) AS quota").Where("user_id = ? AND cut_date = ?", userID, day).Scan(&cuts).Error; err != nil {
		return nil, err
	}
	eligible := user.Group == "default" && user.Status == common.UserStatusEnabled
	active := loan != nil && loan.Status == 1
	progress := 100.0
	if settings.LossPoolTotalQuota > 0 {
		progress = 100 * (1 - float64(settings.LossPoolRemainingQuota)/float64(settings.LossPoolTotalQuota))
	}
	return map[string]any{"visible": eligible && (settings.Enabled || active), "enabled": settings.Enabled,
		"loan": loan, "max_borrow_quota": settings.MaxBorrowQuota + account.TemporaryBonusQuota,
		"temporary_bonus_quota": account.TemporaryBonusQuota, "borrow_paused": settings.LossPoolRemainingQuota > 0,
		"can_borrow":  eligible && settings.Enabled && !active && settings.LossPoolRemainingQuota == 0,
		"can_checkin": eligible && active && today == 0, "checked_in_today": today > 0, "checkins": checkins,
		"loss_pool": map[string]any{"visible": settings.LossPoolVisible, "title": settings.LossPoolTitle, "description": settings.LossPoolDescription,
			"active": settings.LossPoolRemainingQuota > 0, "total_quota": settings.LossPoolTotalQuota, "remaining_quota": settings.LossPoolRemainingQuota,
			"progress_percent": progress, "daily_cut_limit": settings.DailyCutLimit, "user_cuts_today": cuts.Count, "user_cut_quota": cuts.Quota,
			"can_cut": eligible && settings.LossPoolVisible && settings.LossPoolRemainingQuota > 0 && cuts.Count < int64(settings.DailyCutLimit)}}, nil
}

type RelayForRegistrationSettings struct {
	ID                         int    `json:"-" gorm:"primaryKey"`
	LocalRegistrationEnabled   bool   `json:"local_registration_enabled"`
	LinuxdoLoginEnabled        bool   `json:"linuxdo_login_enabled"`
	LinuxdoRegistrationEnabled bool   `json:"linuxdo_registration_enabled"`
	RegistrationCodeRequired   bool   `json:"registration_code_required"`
	PublicInitialQuota         int    `json:"public_initial_quota"`
	LinuxdoExtraQuota          int    `json:"linuxdo_extra_quota"`
	RegistrationCodeHash       string `json:"-" gorm:"size:255"`
}

type RelayForRegistrationCode struct {
	ID            int64      `json:"id" gorm:"primaryKey"`
	BatchID       string     `json:"batch_id" gorm:"size:64;index"`
	Name          string     `json:"name" gorm:"size:256"`
	Code          string     `json:"code" gorm:"size:64;uniqueIndex"`
	CodeDigest    string     `json:"-" gorm:"size:64;uniqueIndex"`
	UserGroup     string     `json:"user_group" gorm:"size:32"`
	Status        int        `json:"status" gorm:"index"`
	UsedByUserID  int        `json:"used_by_user_id"`
	UsedByAccount string     `json:"used_by_account" gorm:"size:80"`
	UsedAt        *time.Time `json:"used_at"`
	CreatedAt     time.Time  `json:"created_at" gorm:"index"`
}

func RelayForRandomCode() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func relayForCodeDigest(code string) string {
	digest := sha256.Sum256([]byte(code))
	return hex.EncodeToString(digest[:])
}

func GetRelayForRegistrationSettings() (RelayForRegistrationSettings, error) {
	return relayForRegistrationSettings(DB)
}

func relayForRegistrationSettings(tx *gorm.DB) (RelayForRegistrationSettings, error) {
	var settings RelayForRegistrationSettings
	err := tx.First(&settings, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return RelayForRegistrationSettings{ID: 1, LocalRegistrationEnabled: common.RegisterEnabled && common.PasswordRegisterEnabled,
			LinuxdoLoginEnabled:        common.LinuxDOOAuthEnabled,
			LinuxdoRegistrationEnabled: common.RegisterEnabled, PublicInitialQuota: common.QuotaForNewUser}, nil
	}
	return settings, err
}

func SaveRelayForRegistrationSettings(settings RelayForRegistrationSettings, newCode string) error {
	for _, quota := range []int{settings.PublicInitialQuota, settings.LinuxdoExtraQuota} {
		if quota < 0 || quota > relayForMaxBenefitQuota {
			return errors.New("初始额度必须在0到500000000之间")
		}
	}
	newHash := ""
	if newCode != "" {
		if len(newCode) < 8 || len(newCode) > 64 {
			return errors.New("共享注册码长度必须在8到64字符之间")
		}
		hash, err := common.Password2Hash(newCode)
		if err != nil {
			return err
		}
		newHash = hash
	}
	settings.ID = 1
	options := map[string]string{
		"RegisterEnabled":         strconv.FormatBool(settings.LocalRegistrationEnabled || settings.LinuxdoRegistrationEnabled),
		"PasswordRegisterEnabled": strconv.FormatBool(settings.LocalRegistrationEnabled),
		"LinuxDOOAuthEnabled":     strconv.FormatBool(settings.LinuxdoLoginEnabled),
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		previous, err := relayForRegistrationSettings(tx)
		if err != nil {
			return err
		}
		settings.RegistrationCodeHash = previous.RegistrationCodeHash
		if newHash != "" {
			settings.RegistrationCodeHash = newHash
		}
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&settings).Error; err != nil {
			return err
		}
		for key, value := range options {
			if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&Option{Key: key, Value: value}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for key, value := range options {
		if err := updateOptionMap(key, value); err != nil {
			return err
		}
	}
	return nil
}

func GenerateRelayForRegistrationCodes(name, group string, count int, now time.Time) ([]RelayForRegistrationCode, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 64 || count < 1 || count > 100 || strings.TrimSpace(group) == "" || len(group) > 32 {
		return nil, errors.New("注册码批次参数无效")
	}
	batch, err := RelayForRandomCode()
	if err != nil {
		return nil, err
	}
	codes := make([]RelayForRegistrationCode, 0, count)
	for range count {
		code, err := RelayForRandomCode()
		if err != nil {
			return nil, err
		}
		codes = append(codes, RelayForRegistrationCode{BatchID: batch, Name: name, Code: code, CodeDigest: relayForCodeDigest(code), UserGroup: group, Status: 1, CreatedAt: now})
	}
	return codes, DB.Create(&codes).Error
}

func ListRelayForRegistrationCodes(page, pageSize int, keyword, status, group string) (map[string]any, error) {
	if page < 1 || page > 10000 || pageSize < 1 || pageSize > 100 || len(keyword) > 128 ||
		(status != "" && status != "1" && status != "2") || len(group) > 32 {
		return nil, errors.New("查询参数无效")
	}
	query := DB.Model(&RelayForRegistrationCode{})
	if keyword != "" {
		pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(keyword) + "%"
		query = query.Where("name LIKE ? ESCAPE '!' OR code LIKE ? ESCAPE '!' OR batch_id LIKE ? ESCAPE '!'", pattern, pattern, pattern)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if group != "" {
		query = query.Where("user_group = ?", group)
	}
	var total int64
	if err := query.Distinct("batch_id").Count(&total).Error; err != nil {
		return nil, err
	}
	var batches []struct {
		BatchID string
		LastID  int64
	}
	if err := query.Select("batch_id, MAX(id) AS last_id").Group("batch_id").Order("last_id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Scan(&batches).Error; err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(batches))
	for _, batch := range batches {
		codes := make([]RelayForRegistrationCode, 0)
		if err := DB.Where("batch_id = ?", batch.BatchID).Order("id ASC").Limit(100).Find(&codes).Error; err != nil {
			return nil, err
		}
		if len(codes) == 0 {
			continue
		}
		used := 0
		for _, code := range codes {
			if code.Status == 2 {
				used++
			}
		}
		first := codes[0]
		items = append(items, map[string]any{"batch_id": batch.BatchID, "name": first.Name, "user_group": first.UserGroup,
			"created_at": first.CreatedAt, "count": len(codes), "used_count": used, "unused_count": len(codes) - used, "items": codes})
	}
	return map[string]any{"items": items, "total": total, "total_pages": (total + int64(pageSize) - 1) / int64(pageSize), "page": page, "page_size": pageSize}, nil
}

func DeleteRelayForRegistrationCodes(id int64, batchID string, used bool) (int64, error) {
	query := DB.Model(&RelayForRegistrationCode{})
	if used {
		query = query.Where("status = 2")
	} else if id > 0 {
		query = query.Where("id = ? AND status = 1", id)
	} else if batchID != "" && len(batchID) <= 64 {
		query = query.Where("batch_id = ? AND status = 1", batchID)
	} else {
		return 0, errors.New("必须指定注册码或批次")
	}
	result := query.Delete(&RelayForRegistrationCode{})
	return result.RowsAffected, result.Error
}

func RelayForRegistrationConfig(settings RelayForRegistrationSettings, includeQuota bool) (map[string]any, error) {
	view := map[string]any{"local_registration_enabled": settings.LocalRegistrationEnabled && common.RegisterEnabled && common.PasswordRegisterEnabled,
		"enabled":                      common.RegisterEnabled,
		"linuxdo_login_enabled":        settings.LinuxdoLoginEnabled && common.LinuxDOOAuthEnabled,
		"linuxdo_registration_enabled": settings.LinuxdoRegistrationEnabled && common.RegisterEnabled,
		"registration_code_required":   settings.RegistrationCodeRequired}
	if includeQuota {
		var unused []struct {
			UserGroup string
			Count     int64
		}
		if err := DB.Model(&RelayForRegistrationCode{}).Select("user_group,COUNT(*) AS count").Where("status = 1").Group("user_group").Scan(&unused).Error; err != nil {
			return nil, err
		}
		var available int64
		for _, item := range unused {
			available += item.Count
		}
		view["public_initial_quota"], view["linuxdo_extra_quota"] = settings.PublicInitialQuota, settings.LinuxdoExtraQuota
		view["registration_code_configured"] = settings.RegistrationCodeHash != "" || available > 0
	}
	return view, nil
}

// InsertRelayForRegisteredUser keeps one-use redemption and account creation
// in the same transaction. New API still hashes passwords and validates email.
func InsertRelayForRegisteredUser(user *User, inviterID int, registrationCode string) error {
	registrationCode = strings.TrimSpace(registrationCode)
	if len(registrationCode) > 128 {
		return errors.New("注册码无效")
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		settings, err := relayForRegistrationSettings(tx)
		if err != nil {
			return err
		}
		group := "default"
		var code RelayForRegistrationCode
		oneUse := false
		shared := false
		if registrationCode != "" {
			err = lockForUpdate(tx).Where("code_digest = ?", relayForCodeDigest(registrationCode)).First(&code).Error
			if err == nil {
				if code.Status != 1 {
					return errors.New("注册码已使用")
				}
				group, oneUse = code.UserGroup, true
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			} else if settings.RegistrationCodeHash != "" && common.ValidatePasswordAndHash(registrationCode, settings.RegistrationCodeHash) {
				shared = true
			} else {
				return errors.New("注册码无效")
			}
		}
		enabled, required, initial := settings.LocalRegistrationEnabled, settings.RegistrationCodeRequired, settings.PublicInitialQuota
		if !enabled {
			return errors.New("此用户组未开放本地注册")
		}
		if required && !oneUse && !shared {
			return errors.New("请提供有效注册码")
		}
		user.Group = group
		if err := user.InsertWithTx(tx, inviterID); err != nil {
			return err
		}
		user.Quota = initial
		if err := tx.Model(user).Update("quota", initial).Error; err != nil {
			return err
		}
		if oneUse {
			now := time.Now().UTC()
			result := tx.Model(&RelayForRegistrationCode{}).Where("id = ? AND status = 1", code.ID).
				Updates(map[string]any{"status": 2, "used_by_user_id": user.Id, "used_by_account": user.Username, "used_at": now})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errors.New("注册码已被使用")
			}
		}
		return nil
	})
	if err == nil {
		user.FinishInsert(inviterID)
	}
	return err
}

// Only a credential reference is persisted in the existing one-use OAuth
// state. Shared registration codes and their password hashes never enter it.
type RelayForRegistrationAdmission struct {
	Group         string `json:"group"`
	CodeID        int64  `json:"code_id,omitempty"`
	SharedVersion string `json:"shared_version,omitempty"`
}

func relayForSharedVersion(hash string) string {
	if hash == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(hash))
	return hex.EncodeToString(digest[:])
}

func PrepareRelayForRegistrationAdmission(rawCode string) (*RelayForRegistrationAdmission, error) {
	rawCode = strings.TrimSpace(rawCode)
	if rawCode == "" {
		return nil, nil
	}
	if len(rawCode) > 128 {
		return nil, errors.New("注册码无效")
	}
	var code RelayForRegistrationCode
	err := DB.Where("code_digest = ? AND status = 1", relayForCodeDigest(rawCode)).First(&code).Error
	if err == nil {
		return &RelayForRegistrationAdmission{Group: code.UserGroup, CodeID: code.ID}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	settings, err := GetRelayForRegistrationSettings()
	if err != nil {
		return nil, err
	}
	for _, candidate := range []struct{ group, hash string }{{"default", settings.RegistrationCodeHash}} {
		if candidate.hash != "" && common.ValidatePasswordAndHash(rawCode, candidate.hash) {
			return &RelayForRegistrationAdmission{Group: candidate.group, SharedVersion: relayForSharedVersion(candidate.hash)}, nil
		}
	}
	return nil, errors.New("注册码无效或已使用")
}

// ApplyRelayForLinuxDORegistration runs inside New API's OAuth account creation
// transaction after InsertWithTx and leaves provider identity validation there.
func ApplyRelayForLinuxDORegistration(tx *gorm.DB, user *User, admissions ...*RelayForRegistrationAdmission) error {
	settings, err := relayForRegistrationSettings(tx)
	if err != nil {
		return err
	}
	if !settings.LinuxdoLoginEnabled || !settings.LinuxdoRegistrationEnabled {
		return errors.New("LinuxDO新用户注册已关闭")
	}
	var admission *RelayForRegistrationAdmission
	if len(admissions) > 0 {
		admission = admissions[0]
	}
	group := "default"
	valid := false
	if admission != nil {
		group = admission.Group
		if strings.TrimSpace(group) == "" || len(group) > 32 {
			return errors.New("注册用户组无效")
		}
		if admission.CodeID > 0 {
			var code RelayForRegistrationCode
			if err := lockForUpdate(tx).Where("id = ? AND status = 1 AND user_group = ?", admission.CodeID, group).First(&code).Error; err != nil {
				return errors.New("注册码无效或已使用")
			}
			valid = true
		} else {
			hash := settings.RegistrationCodeHash
			valid = admission.SharedVersion != "" && admission.SharedVersion == relayForSharedVersion(hash)
			if !valid {
				return errors.New("共享注册码已变更，请重新开始注册")
			}
		}
	}
	required, initial := settings.RegistrationCodeRequired, settings.PublicInitialQuota
	if required && !valid {
		return errors.New("请提供有效注册码以创建LinuxDO账户")
	}
	quota := initial + settings.LinuxdoExtraQuota
	if err := common.ValidateWalletQuota(quota); err != nil {
		return err
	}
	user.Quota = quota
	user.Group = group
	if err := tx.Model(user).Updates(map[string]any{"quota": quota, "group": group}).Error; err != nil {
		return err
	}
	if admission != nil && admission.CodeID > 0 {
		result := tx.Model(&RelayForRegistrationCode{}).Where("id = ? AND status = 1", admission.CodeID).
			Updates(map[string]any{"status": 2, "used_by_user_id": user.Id, "used_by_account": user.Username, "used_at": time.Now().UTC()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("注册码已被使用")
		}
	}
	return nil
}
