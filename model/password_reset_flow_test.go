package model

import (
	"context"
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPasswordResetFlowIsDurableSingleUseAndBoundToAccountVersion(t *testing.T) {
	db := businessCommandDatabase(t)
	require.NoError(t, db.AutoMigrate(&AuthFlow{}))
	user := User{Username: "reset-flow", Email: "reset-flow@example.com", AffCode: "reset-flow", AuthVersion: 1, Password: "before"}
	require.NoError(t, db.Create(&user).Error)
	token, err := CreatePasswordResetFlowContext(context.Background(), user.Email)
	require.NoError(t, err)
	require.ErrorIs(t, ResetPasswordWithFlowContext(context.Background(), "wrong@example.com", token, "new-password-123"), ErrAuthFlowInvalid)
	require.NoError(t, ResetPasswordWithFlowContext(context.Background(), user.Email, token, "new-password-123"))
	require.ErrorIs(t, ResetPasswordWithFlowContext(context.Background(), user.Email, token, "other-password-123"), ErrAuthFlowConsumed)
	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	require.True(t, common.ValidatePasswordAndHash("new-password-123", stored.Password))
	second, err := CreatePasswordResetFlowContext(context.Background(), user.Email)
	require.NoError(t, err)
	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("auth_version", stored.AuthVersion+1).Error)
	require.ErrorIs(t, ResetPasswordWithFlowContext(context.Background(), user.Email, second, "stale-password-123"), ErrAuthFlowInvalid)
}

func TestPasswordResetFlowFailureDoesNotConsumeProof(t *testing.T) {
	db := businessCommandDatabase(t)
	require.NoError(t, db.AutoMigrate(&AuthFlow{}))
	user := User{Username: "reset-rollback", Email: "reset-rollback@example.com", AffCode: "reset-rollback", AuthVersion: 1, Password: "before"}
	require.NoError(t, db.Create(&user).Error)
	token, err := CreatePasswordResetFlowContext(context.Background(), user.Email)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, ResetPasswordWithFlowContext(ctx, user.Email, token, "new-password-123"), context.Canceled)
	flow, err := GetAuthFlowContext(context.Background(), token, AuthFlowMatch{Purpose: authFlowPurposePasswordReset})
	require.NoError(t, err)
	require.Nil(t, flow.ConsumedAt)
}
