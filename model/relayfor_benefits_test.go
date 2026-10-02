package model

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupRelayForBenefitsDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLog := DB, LOG_DB
	previousRedis, previousOptions := common.RedisEnabled, common.OptionMap
	previousRegister, previousPassword, previousLinux := common.RegisterEnabled, common.PasswordRegisterEnabled, common.LinuxDOOAuthEnabled
	previousQuota := common.QuotaForNewUser
	previousType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "benefits.db")+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_txlock=immediate"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	common.RedisEnabled = false
	common.OptionMap = make(map[string]string)
	common.RegisterEnabled, common.PasswordRegisterEnabled, common.LinuxDOOAuthEnabled = true, true, true
	common.QuotaForNewUser = 0
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(&User{}, &Option{}, &Log{}, &RelayForLoanSettings{}, &RelayForLoanAccount{}, &RelayForTokenLoan{}, &RelayForLoanCheckin{}, &RelayForLoanCut{}, &RelayForRegistrationSettings{}, &RelayForRegistrationCode{}))
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLog
		common.RedisEnabled, common.OptionMap = previousRedis, previousOptions
		common.RegisterEnabled, common.PasswordRegisterEnabled, common.LinuxDOOAuthEnabled = previousRegister, previousPassword, previousLinux
		common.QuotaForNewUser = previousQuota
		common.SetMainDatabaseType(previousType)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	return db
}

func relayForTestUser(t *testing.T, name string, quota int) User {
	t.Helper()
	user := User{Username: name, Password: "unused", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Quota: quota}
	require.NoError(t, DB.Create(&user).Error)
	return user
}

func TestRelayForLoanConcurrentBorrowAndCheckin(t *testing.T) {
	setupRelayForBenefitsDB(t)
	user := relayForTestUser(t, "loan-concurrent", 5)
	settings := defaultRelayForLoanSettings()
	settings.Enabled = true
	settings.MaxBorrowQuota = 500
	settings.MinCheckinQuota = 100
	require.NoError(t, SaveRelayForLoanSettings(settings))
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	var successes atomic.Int32
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := RelayForBorrow(user.Id, 450, now); err == nil {
				successes.Add(1)
			}
		}()
	}
	wait.Wait()
	require.EqualValues(t, 1, successes.Load())
	stored, err := GetUserById(user.Id, false)
	require.NoError(t, err)
	require.Equal(t, 455, stored.Quota)
	for range 8 {
		wait.Add(1)
		go func() { defer wait.Done(); _, _ = RelayForLoanCheckIn(user.Id, now) }()
	}
	wait.Wait()
	var count int64
	require.NoError(t, DB.Model(&RelayForLoanCheckin{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	var loan RelayForTokenLoan
	require.NoError(t, DB.First(&loan).Error)
	require.Equal(t, 350, loan.RemainingQuota)
	require.Equal(t, 1, loan.CheckinCount)
	for day := 1; day < 4; day++ {
		record, err := RelayForLoanCheckIn(user.Id, now.AddDate(0, 0, day))
		require.NoError(t, err)
		if day == 3 {
			require.Equal(t, 150, record.AwardedQuota)
			require.Zero(t, record.RemainingAfter)
		}
	}
	require.NoError(t, DB.First(&loan).Error)
	require.Equal(t, 2, loan.Status)
	require.Equal(t, 4, loan.CheckinCount)
	stored, err = GetUserById(user.Id, false)
	require.NoError(t, err)
	require.Equal(t, 455, stored.Quota, "check-in forgives debt without minting or debiting wallet quota")
}

func TestRelayForLoanOverflowIsOneTimeAndDailyResetUsesUTC8(t *testing.T) {
	setupRelayForBenefitsDB(t)
	user := relayForTestUser(t, "loan-bonus", 0)
	settings := defaultRelayForLoanSettings()
	settings.Enabled = true
	settings.MaxBorrowQuota = 100
	settings.MinCheckinQuota = 100
	require.NoError(t, SaveRelayForLoanSettings(settings))
	now := time.Date(2026, 9, 24, 15, 30, 0, 0, time.UTC)
	_, err := RelayForBorrow(user.Id, 80, now)
	require.NoError(t, err)
	first, err := RelayForLoanCheckIn(user.Id, now)
	require.NoError(t, err)
	require.Equal(t, 20, first.OverflowQuota)
	_, err = RelayForBorrow(user.Id, 121, now)
	require.Error(t, err)
	second, err := RelayForBorrow(user.Id, 120, now)
	require.NoError(t, err)
	duplicate, err := RelayForLoanCheckIn(user.Id, now)
	require.NoError(t, err)
	require.Equal(t, first.ID, duplicate.ID)
	var loan RelayForTokenLoan
	require.NoError(t, DB.First(&loan, second.ID).Error)
	require.Equal(t, 120, loan.RemainingQuota)
	status, err := RelayForLoanStatus(user.Id, now)
	require.NoError(t, err)
	require.Equal(t, 0, status["temporary_bonus_quota"])
	require.False(t, status["can_checkin"].(bool))
	newDay, err := RelayForLoanCheckIn(user.Id, now.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, "2026-09-25", newDay.CheckinDate)
	require.Equal(t, 20, newDay.RemainingAfter)
	_, err = RelayForBorrow(user.Id, -1, now)
	require.Error(t, err)
}

func TestRelayForLoanPoolCannotGoNegativeOrExceedDailyLimit(t *testing.T) {
	setupRelayForBenefitsDB(t)
	user := relayForTestUser(t, "loan-pool", 0)
	settings := defaultRelayForLoanSettings()
	settings.Enabled = true
	settings.LossPoolTotalQuota = 15
	settings.LossPoolRemainingQuota = 15
	settings.LossPoolCutMinQuota = 10
	settings.LossPoolCutMaxQuota = 10
	require.NoError(t, SaveRelayForLoanSettings(settings))
	now := time.Now().UTC()
	_, err := RelayForBorrow(user.Id, 10, now)
	require.Error(t, err)
	first, err := RelayForCutLossPool(user.Id, now)
	require.NoError(t, err)
	require.Equal(t, 10, first.Quota)
	second, err := RelayForCutLossPool(user.Id, now)
	require.NoError(t, err)
	require.Equal(t, 5, second.Quota)
	_, err = RelayForCutLossPool(user.Id, now)
	require.Error(t, err)
	settings, err = GetRelayForLoanSettings()
	require.NoError(t, err)
	require.Zero(t, settings.LossPoolRemainingQuota)
	_, err = RelayForBorrow(user.Id, 10, now)
	require.NoError(t, err)
	settings.LossPoolRemainingQuota = -1
	require.Error(t, SaveRelayForLoanSettings(settings))
}

func TestRelayForRegistrationSingleUseIsAtomicAndFailureRollsBack(t *testing.T) {
	setupRelayForBenefitsDB(t)
	settings := RelayForRegistrationSettings{LocalRegistrationEnabled: true, RegistrationCodeRequired: true, PublicInitialQuota: 100}
	require.NoError(t, SaveRelayForRegistrationSettings(settings, ""))
	codes, err := GenerateRelayForRegistrationCodes("batch", "pay", 2, time.Now().UTC())
	require.NoError(t, err)
	require.Len(t, codes[0].Code, 64)
	require.NotEqual(t, codes[0].Code, codes[1].Code)
	user := User{Username: "registered", Password: "real-password-123", Role: common.RoleCommonUser}
	require.NoError(t, InsertRelayForRegisteredUser(&user, 0, codes[0].Code))
	stored, err := GetUserById(user.Id, true)
	require.NoError(t, err)
	require.Equal(t, "pay", stored.Group)
	require.Equal(t, 100, stored.Quota)
	require.NotEqual(t, "real-password-123", stored.Password)
	require.True(t, common.ValidatePasswordAndHash("real-password-123", stored.Password))
	duplicate := User{Username: "registered", Password: "real-password-123", Role: common.RoleCommonUser}
	require.Error(t, InsertRelayForRegisteredUser(&duplicate, 0, codes[1].Code))
	var unused RelayForRegistrationCode
	require.NoError(t, DB.First(&unused, codes[1].ID).Error)
	require.Equal(t, 1, unused.Status)
	var successes atomic.Int32
	var wait sync.WaitGroup
	for index := range 6 {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			candidate := User{Username: fmt.Sprintf("redeem-%d", index), Password: "real-password-123", Role: common.RoleCommonUser}
			if InsertRelayForRegisteredUser(&candidate, 0, codes[1].Code) == nil {
				successes.Add(1)
			}
		}(index)
	}
	wait.Wait()
	require.EqualValues(t, 1, successes.Load())
	_, err = DeleteRelayForRegistrationCodes(codes[0].ID, "", false)
	require.NoError(t, err)
	var used RelayForRegistrationCode
	require.NoError(t, DB.First(&used, codes[0].ID).Error)
	require.Equal(t, 2, used.Status)
	page, err := ListRelayForRegistrationCodes(1, 25, "batch", "2", "pay")
	require.NoError(t, err)
	require.EqualValues(t, 1, page["total"])
	items := page["items"].([]map[string]any)
	require.Len(t, items, 1)
	require.Equal(t, 2, items[0]["used_count"])
}

func TestRelayForRegistrationSharedCodeIsHashedAndPublicViewIsSafe(t *testing.T) {
	setupRelayForBenefitsDB(t)
	settings := RelayForRegistrationSettings{LocalRegistrationEnabled: true, RegistrationCodeRequired: true, PublicInitialQuota: 100}
	require.NoError(t, SaveRelayForRegistrationSettings(settings, "shared-secret-123"))
	settings, err := GetRelayForRegistrationSettings()
	require.NoError(t, err)
	require.NotEqual(t, "shared-secret-123", settings.RegistrationCodeHash)
	encoded, err := json.Marshal(settings)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), settings.RegistrationCodeHash)
	require.NotContains(t, string(encoded), "shared-secret-123")
	view, err := RelayForRegistrationConfig(settings, false)
	require.NoError(t, err)
	require.NotContains(t, view, "public_initial_quota")
	require.NotContains(t, view, "registration_code_hash")
	require.NotContains(t, view, "pay_initial_quota")
	require.NotContains(t, view, "pay_local_registration_enabled")
	teamCodes, err := GenerateRelayForRegistrationCodes("ordinary group", "team-alpha", 1, time.Now().UTC())
	require.NoError(t, err)
	teamUser := User{Username: "team-member", Password: "safe-password-123", Role: common.RoleCommonUser}
	require.NoError(t, InsertRelayForRegisteredUser(&teamUser, 0, teamCodes[0].Code))
	require.Equal(t, "team-alpha", teamUser.Group)
	require.Equal(t, settings.PublicInitialQuota, teamUser.Quota)
	for index := range 2 {
		user := User{Username: fmt.Sprintf("shared-%d", index), Password: "safe-password-123", Role: common.RoleCommonUser}
		require.NoError(t, InsertRelayForRegisteredUser(&user, 0, "shared-secret-123"))
	}
	user := User{Username: "missing-code", Password: "safe-password-123", Role: common.RoleCommonUser}
	require.Error(t, InsertRelayForRegisteredUser(&user, 0, ""))
	settings.PublicInitialQuota = -1
	require.Error(t, SaveRelayForRegistrationSettings(settings, ""))
}
