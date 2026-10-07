package controller

import (
	"errors"
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/i18n"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"strings"
)

type LoginRequest struct {
	Username          string `json:"username"`
	Password          string `json:"password"`
	PasswordEncrypted string `json:"password_encrypted"`
	EncryptionKeyID   string `json:"encryption_key_id"`
}
type PublicRegistrationRequest struct {
	Username         string `json:"username"`
	Password         string `json:"password"`
	Email            string `json:"email"`
	VerificationCode string `json:"verification_code"`
	AffiliateCode    string `json:"aff_code"`
	RegistrationCode string `json:"registration_code"`
}
type topUpRequest struct {
	Key string `json:"key"`
}

func GetPasswordEncryptionKey(c *gin.Context) {
	if !common.PasswordLoginEncryptionEnabled {
		common.ApiSuccess(c, gin.H{"enabled": false})
		return
	}
	id, key := common.PasswordEncryptionPublicKey()
	if id == "" || key == "" {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	common.ApiSuccess(c, gin.H{"enabled": true, "kid": id, "public_key": key})
}

func Login(c *gin.Context) {
	if !common.PasswordLoginEnabled {
		common.ApiErrorI18n(c, i18n.MsgUserPasswordLoginDisabled)
		return
	}
	var input LoginRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	password := input.Password
	if common.PasswordLoginEncryptionEnabled {
		if input.PasswordEncrypted == "" || input.EncryptionKeyID == "" {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		var err error
		password, err = common.DecryptPassword(input.PasswordEncrypted, input.EncryptionKeyID)
		if err != nil {
			common.ApiErrorI18n(c, i18n.MsgUserUsernameOrPasswordError)
			return
		}
	}
	account, err := model.AuthenticatePasswordContext(c.Request.Context(), input.Username, password)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrUserEmptyCredentials):
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		case errors.Is(err, model.ErrDatabase):
			common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		case errors.Is(err, common.ErrPasswordKDFBusy):
			writeSecurityOperationError(c, err)
		default:
			common.ApiErrorI18n(c, i18n.MsgUserUsernameOrPasswordError)
		}
		return
	}
	setupLogin(account, nil, c)
}

func Register(c *gin.Context) {
	if !common.RegisterEnabled {
		common.ApiErrorI18n(c, i18n.MsgUserRegisterDisabled)
		return
	}
	if !common.PasswordRegisterEnabled {
		common.ApiErrorI18n(c, i18n.MsgUserPasswordRegisterDisabled)
		return
	}
	var input PublicRegistrationRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	email := ""
	if common.EmailVerificationEnabled {
		email = model.NormalizeEmail(input.Email)
		if email == "" || input.VerificationCode == "" {
			common.ApiErrorI18n(c, i18n.MsgUserEmailVerificationRequired)
			return
		}
	}
	_, err := model.CreateRegisteredAccountContext(c.Request.Context(), model.PublicRegistrationInput{Username: input.Username, Password: input.Password, Email: email, AffiliateCode: input.AffiliateCode, RegistrationCode: input.RegistrationCode, IssueDefaultToken: constant.GenerateDefaultToken, DefaultTokenAutoGroup: setting.DefaultUseAutoGroup, RequireEmailVerification: common.EmailVerificationEnabled, EmailVerificationCode: input.VerificationCode})
	if err != nil {
		switch {
		case errors.Is(err, common.ErrPasswordKDFBusy):
			writeSecurityOperationError(c, err)
		case errors.Is(err, model.ErrAuthFlowInvalid), errors.Is(err, model.ErrAuthFlowConsumed), errors.Is(err, model.ErrAuthFlowExpired):
			common.ApiErrorI18n(c, i18n.MsgUserVerificationCodeError)
		case errors.Is(err, model.ErrAccountNameUnavailable):
			common.ApiErrorI18n(c, i18n.MsgUserExists)
		case errors.Is(err, model.ErrEmailAlreadyTaken):
			common.ApiErrorI18n(c, i18n.MsgUserEmailAlreadyTaken)
		default:
			common.ApiError(c, err)
		}
		return
	}
	common.ApiSuccess(c, nil)
}

func TopUp(c *gin.Context) {
	if !operation_setting.IsPaymentComplianceConfirmed() {
		common.ApiErrorI18n(c, i18n.MsgPaymentComplianceRequired)
		return
	}
	var input topUpRequest
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.Key) == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	lock := getTopUpLock(c.GetInt("id"))
	if !lock.TryLock() {
		common.ApiErrorI18n(c, i18n.MsgUserTopUpProcessing)
		return
	}
	defer lock.Unlock()
	quota, err := model.RedeemContext(c.Request.Context(), input.Key, c.GetInt("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgRedeemFailed)
		return
	}
	common.ApiSuccess(c, quota)
}
