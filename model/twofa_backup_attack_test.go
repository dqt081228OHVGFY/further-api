package model

import (
	"errors"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func backupAttackFixture(t *testing.T) (TwoFA, TwoFABackupCode) {
	t.Helper()
	factor := totpAttackFixture(t)
	require.NoError(t, DB.AutoMigrate(&TwoFABackupCode{}))
	hash, err := common.HashBackupCode("ABCD-1234")
	require.NoError(t, err)
	code := TwoFABackupCode{UserId: factor.UserId, CodeHash: hash}
	require.NoError(t, DB.Create(&code).Error)
	return factor, code
}

func TestBackupCodeOldSnapshotCannotClearConcurrentLockOrDisable(t *testing.T) {
	for _, change := range []string{"lock", "disable"} {
		t.Run(change, func(t *testing.T) {
			factor, code := backupAttackFixture(t)
			if change == "lock" {
				require.NoError(t, DB.Model(&TwoFA{}).Where("id = ?", factor.Id).Update("locked_until", time.Now().Add(time.Minute)).Error)
			} else {
				require.NoError(t, DB.Model(&TwoFA{}).Where("id = ?", factor.Id).Update("is_enabled", false).Error)
			}
			valid, err := factor.ValidateBackupCodeAndUpdateUsage("ABCD-1234")
			require.False(t, valid)
			if change == "lock" {
				require.ErrorIs(t, err, ErrTwoFAVerificationLocked)
			} else {
				require.ErrorIs(t, err, ErrTwoFANotEnabled)
			}
			require.NoError(t, DB.First(&code, code.Id).Error)
			require.False(t, code.IsUsed, "a locked or disabled factor must not consume the recovery code")
			if change == "lock" {
				current, err := GetTwoFAByUserId(factor.UserId)
				require.NoError(t, err)
				require.True(t, current.IsLocked())
			}
		})
	}
}

func TestBackupCodeAndUsageWriteRollbackTogether(t *testing.T) {
	factor, code := backupAttackFixture(t)
	failure := errors.New("fixture factor usage failure")
	callback := "test:backup-usage-rollback"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "two_fas" {
			tx.AddError(failure)
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Update().Remove(callback) })
	valid, err := factor.ValidateBackupCodeAndUpdateUsage("ABCD-1234")
	require.False(t, valid)
	require.ErrorIs(t, err, failure)
	require.NoError(t, DB.First(&code, code.Id).Error)
	require.False(t, code.IsUsed)
	require.NoError(t, DB.Callback().Update().Remove(callback))
	valid, err = factor.ValidateBackupCodeAndUpdateUsage("ABCD-1234")
	require.NoError(t, err)
	require.True(t, valid)
	valid, err = factor.ValidateBackupCodeAndUpdateUsage("ABCD-1234")
	require.NoError(t, err)
	require.False(t, valid)
}

func TestConcurrentBackupAssertionsAcceptOneSubmission(t *testing.T) {
	factor, _ := backupAttackFixture(t)
	type result struct {
		accepted bool
		err      error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for range 2 {
		snapshot := factor
		go func() {
			<-start
			valid, err := snapshot.ValidateBackupCodeAndUpdateUsage("ABCD-1234")
			results <- result{valid, err}
		}()
	}
	close(start)
	accepted := 0
	for range 2 {
		result := <-results
		require.NoError(t, result.err)
		if result.accepted {
			accepted++
		}
	}
	require.Equal(t, 1, accepted)
}
