package model

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"gorm.io/gorm"
)

var ErrPaymentConfirmationRootRequired = errors.New("仅超级管理员可确认付款渠道使用条件")

type PaymentTermsConfirmation struct {
	Confirmed    bool   `json:"confirmed"`
	TermsVersion string `json:"terms_version"`
	ConfirmedAt  int64  `json:"confirmed_at"`
	ConfirmedBy  int    `json:"confirmed_by"`
}

func ConfirmGatewayPaymentTermsContext(ctx context.Context, identity AuthSessionIdentity, ip string) (*PaymentTermsConfirmation, error) {
	if identity.UserID <= 0 || identity.SessionID == "" {
		return nil, ErrUserSessionInactive
	}
	confirmation := &PaymentTermsConfirmation{Confirmed: true, TermsVersion: operation_setting.CurrentComplianceTermsVersion, ConfirmedAt: time.Now().Unix(), ConfirmedBy: identity.UserID}
	values := map[string]string{
		"payment_setting.compliance_confirmed":     "true",
		"payment_setting.compliance_terms_version": confirmation.TermsVersion,
		"payment_setting.compliance_confirmed_at":  strconv.FormatInt(confirmation.ConfirmedAt, 10),
		"payment_setting.compliance_confirmed_by":  strconv.Itoa(identity.UserID),
		"payment_setting.compliance_confirmed_ip":  ip,
	}
	err := mutateGatewayOptionsContext(ctx, values, func(tx *gorm.DB) error {
		if err := ValidateAuthSessionWithTx(tx, identity); err != nil {
			return err
		}
		var account User
		if err := tx.Select("id", "role").First(&account, identity.UserID).Error; err != nil {
			return err
		}
		if account.Role != common.RoleRootUser {
			return ErrPaymentConfirmationRootRequired
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return confirmation, nil
}
