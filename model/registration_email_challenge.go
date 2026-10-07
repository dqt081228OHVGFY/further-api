package model

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
)

const AuthFlowPurposeRegistrationEmail = "registration-email"

type RegistrationEmailChallenge struct {
	ID        int64
	Email     string
	Code      string
	ExpiresAt time.Time
}

type registrationEmailEvidence struct {
	Email  string `json:"email"`
	Digest string `json:"digest"`
}

func registrationEmailIdentity(email string) string {
	return common.GenerateHMACWithKey([]byte("further-api:registration-email:"+common.SessionSecret), NormalizeEmail(email))
}
func registrationEmailCodeDigest(email, code string) string {
	return common.GenerateHMACWithKey([]byte("further-api:registration-code:"+common.SessionSecret), NormalizeEmail(email)+":"+code)
}

func CreateRegistrationEmailChallengeContext(ctx context.Context, email string) (*RegistrationEmailChallenge, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	email = NormalizeEmail(email)
	if email == "" {
		return nil, ErrEmailNotFound
	}
	integer, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return nil, err
	}
	challenge := &RegistrationEmailChallenge{Email: email, Code: fmt.Sprintf("%06d", integer.Int64()), ExpiresAt: time.Now().Add(time.Duration(common.VerificationValidMinutes) * time.Minute)}
	payload, err := common.Marshal(registrationEmailEvidence{Email: email, Digest: registrationEmailCodeDigest(email, challenge.Code)})
	if err != nil {
		return nil, err
	}
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return withNormalizedEmailLock(tx, email, func(tx *gorm.DB) error {
			if err := ensureEmailAvailableWithTx(tx, email, 0); err != nil {
				return err
			}
			identity := registrationEmailIdentity(email)
			now := time.Now()
			if err := tx.Model(&AuthFlow{}).Where("purpose = ? AND session_id = ? AND consumed_at IS NULL", AuthFlowPurposeRegistrationEmail, identity).Update("consumed_at", now).Error; err != nil {
				return err
			}
			_, flow, err := createAuthFlowWithTx(tx, AuthFlowCreate{Purpose: AuthFlowPurposeRegistrationEmail, Provider: "email", SessionId: identity, Payload: string(payload), ExpiresAt: challenge.ExpiresAt})
			if err == nil {
				challenge.ID = flow.Id
			}
			return err
		})
	})
	if err != nil {
		return nil, err
	}
	return challenge, nil
}

// Consume and account creation share the caller's transaction and normalized
// email lock. A failed account insert leaves the valid challenge retryable.
func consumeRegistrationEmailChallengeTx(tx *gorm.DB, email, code string) (bool, error) {
	email, code = NormalizeEmail(email), strings.TrimSpace(code)
	if email == "" || code == "" {
		return false, ErrAuthFlowInvalid
	}
	var flow AuthFlow
	err := lockForUpdate(tx).Where("purpose = ? AND session_id = ?", AuthFlowPurposeRegistrationEmail, registrationEmailIdentity(email)).Order("id DESC").First(&flow).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if common.VerifyCodeWithKey(email, code, common.EmailVerificationPurpose) {
			return true, nil
		}
		return false, ErrAuthFlowInvalid
	}
	if err != nil {
		return false, err
	}
	if flow.ConsumedAt != nil {
		return false, ErrAuthFlowConsumed
	}
	if !flow.ExpiresAt.After(time.Now()) {
		return false, ErrAuthFlowExpired
	}
	var evidence registrationEmailEvidence
	if err := common.UnmarshalJsonStr(flow.Payload, &evidence); err != nil || evidence.Email != email || !hmac.Equal([]byte(evidence.Digest), []byte(registrationEmailCodeDigest(email, code))) {
		return false, ErrAuthFlowInvalid
	}
	now := time.Now()
	result := tx.Model(&AuthFlow{}).Where("id = ? AND consumed_at IS NULL AND expires_at > ?", flow.Id, now).Update("consumed_at", now)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected != 1 {
		return false, ErrAuthFlowConsumed
	}
	return false, nil
}

func RevokeRegistrationEmailChallengeContext(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if id <= 0 {
		return ErrAuthFlowInvalid
	}
	return DB.WithContext(ctx).Model(&AuthFlow{}).Where("id = ? AND purpose = ? AND consumed_at IS NULL", id, AuthFlowPurposeRegistrationEmail).Update("consumed_at", time.Now()).Error
}
