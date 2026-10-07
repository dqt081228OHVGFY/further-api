package model

import (
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"
)

func totpAttackFixture(t *testing.T) TwoFA {
	t.Helper()
	db := businessCommandDatabase(t)
	require.NoError(t, db.AutoMigrate(&TwoFA{}))
	user := User{Username: "totp-attack", AffCode: "totp-attack"}
	require.NoError(t, db.Create(&user).Error)
	factor := TwoFA{UserId: user.Id, Secret: "JBSWY3DPEHPK3PXP", IsEnabled: true}
	require.NoError(t, db.Create(&factor).Error)
	return factor
}

func TestTOTPCodeCannotBeReplayedFromAnIndependentSnapshot(t *testing.T) {
	factor := totpAttackFixture(t)
	stale := factor
	code, err := totp.GenerateCode(factor.Secret, time.Now())
	require.NoError(t, err)
	valid, err := factor.ValidateTOTPAndUpdateUsage(code)
	require.NoError(t, err)
	require.True(t, valid)
	valid, err = stale.ValidateTOTPAndUpdateUsage(code)
	require.ErrorIs(t, err, ErrTwoFACodeReplayed)
	require.False(t, valid)
	stored, err := GetTwoFAByUserId(factor.UserId)
	require.NoError(t, err)
	require.Positive(t, stored.LastAcceptedTimeStep)
	require.Equal(t, 1, stored.FailedAttempts)
	// A fresh accepted counter remains usable, while an older skew-window code
	// cannot be used after a newer counter has already succeeded.
	nextCode, err := totp.GenerateCode(factor.Secret, time.Now().Add(30*time.Second))
	require.NoError(t, err)
	valid, err = stored.ValidateTOTPAndUpdateUsage(nextCode)
	require.NoError(t, err)
	require.True(t, valid)
	valid, err = stale.ValidateTOTPAndUpdateUsage(code)
	require.ErrorIs(t, err, ErrTwoFACodeReplayed)
	require.False(t, valid)
}

func TestConcurrentTOTPAssertionsAcceptOneSubmission(t *testing.T) {
	factor := totpAttackFixture(t)
	code, err := totp.GenerateCode(factor.Secret, time.Now())
	require.NoError(t, err)
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
			valid, err := snapshot.ValidateTOTPAndUpdateUsage(code)
			results <- result{valid, err}
		}()
	}
	close(start)
	accepted := 0
	for range 2 {
		result := <-results
		if result.accepted {
			require.NoError(t, result.err)
			accepted++
		} else {
			require.ErrorIs(t, result.err, ErrTwoFACodeReplayed)
		}
	}
	require.Equal(t, 1, accepted)
}

func TestTOTPAssertionRechecksConcurrentLockDisableAndSecretReplacement(t *testing.T) {
	for _, change := range []string{"lock", "disable", "replace_secret"} {
		t.Run(change, func(t *testing.T) {
			factor := totpAttackFixture(t)
			stale := factor
			code, err := totp.GenerateCode(factor.Secret, time.Now())
			require.NoError(t, err)
			switch change {
			case "lock":
				until := time.Now().Add(time.Minute)
				require.NoError(t, DB.Model(&factor).Update("locked_until", until).Error)
			case "disable":
				require.NoError(t, DB.Model(&factor).Update("is_enabled", false).Error)
			case "replace_secret":
				replacement := "KRUGS4ZANFZSAYJA"
				_, valid := common.MatchTOTPTimeStep(replacement, code, time.Now())
				require.False(t, valid, "fixture code must not collide with the replacement secret")
				require.NoError(t, DB.Model(&factor).Update("secret", replacement).Error)
			}
			valid, err := stale.ValidateTOTPAndUpdateUsage(code)
			require.False(t, valid)
			if change == "lock" {
				require.ErrorIs(t, err, ErrTwoFAVerificationLocked)
			}
			if change == "disable" {
				require.ErrorIs(t, err, ErrTwoFANotEnabled)
			}
			if change == "replace_secret" {
				require.NoError(t, err)
			}
			stored, readErr := GetTwoFAByUserId(factor.UserId)
			require.NoError(t, readErr)
			require.Zero(t, stored.LastAcceptedTimeStep)
			if change == "lock" {
				require.True(t, stored.IsLocked())
			}
		})
	}
}

func TestTOTPTimeStepWindowRetainsExistingAuthenticatorCompatibility(t *testing.T) {
	at := time.Unix(1700000020, 0)
	secret := "JBSWY3DPEHPK3PXP"
	for _, offset := range []int{-2, -1, 0, 1, 2} {
		code, err := totp.GenerateCode(secret, at.Add(time.Duration(offset)*30*time.Second))
		require.NoError(t, err)
		step, valid := common.MatchTOTPTimeStep(secret, code, at)
		if offset >= -1 && offset <= 1 {
			require.True(t, valid)
			require.Equal(t, at.Unix()/30+int64(offset), step)
		} else {
			require.False(t, valid)
		}
	}
	_, valid := common.MatchTOTPTimeStep(secret, "not-a-code", at)
	require.False(t, valid)
}
