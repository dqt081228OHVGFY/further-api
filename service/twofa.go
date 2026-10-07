package service

import (
	"context"
	"errors"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
)

type TwoFASetup struct {
	Secret      string   `json:"secret"`
	QRCodeData  string   `json:"qr_code_data"`
	BackupCodes []string `json:"backup_codes"`
	FlowToken   string   `json:"flow_token"`
	ExpiresAt   int64    `json:"expires_at"`
}

func StartTwoFASetup(identity AuthIdentity, authorization *model.AuthFlowAuthorization) (*TwoFASetup, error) {
	return StartTwoFASetupContext(context.Background(), identity, authorization)
}

func StartTwoFASetupContext(ctx context.Context, identity AuthIdentity, authorization *model.AuthFlowAuthorization) (*TwoFASetup, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ValidateFlowAuthorization(identity, VerificationOperation{Scope: VerificationScopeTwoFASetup}, authorization); err != nil {
		return nil, err
	}
	user, err := model.GetUserByIdContext(ctx, identity.UserID, false)
	if err != nil {
		return nil, err
	}
	key, err := common.GenerateTOTPSecret(user.Username)
	if err != nil {
		return nil, err
	}
	codes, err := common.GenerateBackupCodes()
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(5 * time.Minute)
	flowToken, err := model.CreateTwoFAEnrollmentContext(ctx, identity, authorization, key.Secret(), codes, expiresAt)
	if err != nil {
		return nil, err
	}
	return &TwoFASetup{Secret: key.Secret(), QRCodeData: common.GenerateQRCodeData(key.Secret(), user.Username), BackupCodes: codes, FlowToken: flowToken, ExpiresAt: expiresAt.Unix()}, nil
}

func FinishTwoFASetup(identity AuthIdentity, flowToken, code string) error {
	return FinishTwoFASetupContext(context.Background(), identity, flowToken, code)
}

func FinishTwoFASetupContext(ctx context.Context, identity AuthIdentity, flowToken, code string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	flow, err := model.GetAuthFlowContext(ctx, flowToken, model.AuthFlowMatch{
		Purpose: model.AuthFlowPurposeTwoFASetup, UserId: identity.UserID, SessionId: identity.SessionID,
	})
	if errors.Is(err, model.ErrAuthFlowInvalid) || errors.Is(err, model.ErrAuthFlowExpired) || errors.Is(err, model.ErrAuthFlowConsumed) {
		return model.ErrTwoFASetupInvalid
	}
	if err != nil {
		return err
	}
	var payload struct {
		Authorization *model.AuthFlowAuthorization `json:"authorization"`
	}
	if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil {
		return err
	}
	if err := ValidateFlowAuthorization(identity, VerificationOperation{Scope: VerificationScopeTwoFASetup}, payload.Authorization); err != nil {
		if errors.Is(err, model.ErrAuthFlowInvalid) {
			return model.ErrTwoFASetupInvalid
		}
		return err
	}
	return model.EnableTwoFAEnrollmentContext(ctx, identity, flowToken, code)
}
