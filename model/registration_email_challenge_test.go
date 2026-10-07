package model

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRegistrationEmailChallengeAndAccountCommitTogether(t *testing.T) {
	db := setupFurtherBenefitsDB(t)
	require.NoError(t, db.AutoMigrate(&AuthFlow{}, &Token{}))
	require.NoError(t, SaveFurtherRegistrationSettings(FurtherRegistrationSettings{LocalRegistrationEnabled: true}, ""))
	challenge, err := CreateRegistrationEmailChallengeContext(context.Background(), " Signup@Example.Invalid ")
	require.NoError(t, err)
	require.Equal(t, "signup@example.invalid", challenge.Email)
	input := PublicRegistrationInput{Username: "email-atomic", Password: "password-for-email-signup", Email: challenge.Email, RequireEmailVerification: true, EmailVerificationCode: challenge.Code, IssueDefaultToken: true}
	failure := errors.New("initial token unavailable")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("registration_email_token_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" {
			tx.AddError(failure)
		}
	}))
	_, err = CreateRegisteredAccountContext(context.Background(), input)
	require.ErrorIs(t, err, failure)
	var flow AuthFlow
	require.NoError(t, db.First(&flow, challenge.ID).Error)
	require.Nil(t, flow.ConsumedAt)
	var count int64
	require.NoError(t, db.Model(&User{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Callback().Create().Remove("registration_email_token_failure"))
	account, err := CreateRegisteredAccountContext(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, challenge.Email, account.Email)
	flow = AuthFlow{}
	require.NoError(t, db.First(&flow, challenge.ID).Error)
	require.NotNil(t, flow.ConsumedAt)
	input.Username = "email-replay"
	_, err = CreateRegisteredAccountContext(context.Background(), input)
	require.ErrorIs(t, err, ErrAuthFlowConsumed)
	require.NoError(t, db.Model(&User{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestRegistrationEmailChallengeResendExpiryAndCancellation(t *testing.T) {
	db := setupFurtherBenefitsDB(t)
	require.NoError(t, db.AutoMigrate(&AuthFlow{}, &Token{}))
	require.NoError(t, SaveFurtherRegistrationSettings(FurtherRegistrationSettings{LocalRegistrationEnabled: true}, ""))
	first, err := CreateRegistrationEmailChallengeContext(context.Background(), "resend@example.invalid")
	require.NoError(t, err)
	second, err := CreateRegistrationEmailChallengeContext(context.Background(), first.Email)
	require.NoError(t, err)
	var firstFlow AuthFlow
	require.NoError(t, db.First(&firstFlow, first.ID).Error)
	require.NotNil(t, firstFlow.ConsumedAt)
	input := PublicRegistrationInput{Username: "email-resend", Password: "password-for-email-signup", Email: first.Email, RequireEmailVerification: true, EmailVerificationCode: "invalid-code"}
	_, err = CreateRegisteredAccountContext(context.Background(), input)
	require.ErrorIs(t, err, ErrAuthFlowInvalid)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = CreateRegistrationEmailChallengeContext(ctx, first.Email)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, db.Model(&AuthFlow{}).Where("id = ?", second.ID).Update("expires_at", time.Now().Add(-time.Minute)).Error)
	input.EmailVerificationCode = second.Code
	_, err = CreateRegisteredAccountContext(context.Background(), input)
	require.ErrorIs(t, err, ErrAuthFlowExpired)
}
