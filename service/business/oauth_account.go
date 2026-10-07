package business

import (
	"context"
	"errors"
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/oauth"
	"github.com/dqt081228OHVGFY/further-api/service"
	"gorm.io/gorm"
	"strings"
	"unicode/utf8"
)

type OAuthUserDeletedError struct{}

func (*OAuthUserDeletedError) Error() string { return "user has been deleted" }

type OAuthRegistrationDisabledError struct{}

func (*OAuthRegistrationDisabledError) Error() string { return "registration is disabled" }

type OAuthEmailAlreadyTakenError struct{}

func (*OAuthEmailAlreadyTakenError) Error() string { return "email is already in use" }

type OAuthLegacyBindingNotConfirmedError struct{}

func (*OAuthLegacyBindingNotConfirmedError) Error() string { return "legacy binding was not confirmed" }

type OAuthAccountRequest struct {
	Provider       oauth.Provider
	Remote         *oauth.OAuthUser
	Token          *oauth.OAuthToken
	AffiliateCode  string
	Admissions     []*model.FurtherRegistrationAdmission
	MigrationAudit func(*model.User, bool, map[string]any)
}

var ErrOAuthAlreadyBound = errors.New("OAuth identity is already bound")

func resolveOAuthAccount(ctx context.Context, provider oauth.Provider, remoteID string) (*model.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if generic, ok := provider.(*oauth.GenericOAuthProvider); ok {
		var binding model.UserOAuthBinding
		if err := model.DB.WithContext(ctx).Where("provider_id = ? AND provider_user_id = ?", generic.GetProviderId(), remoteID).First(&binding).Error; err != nil {
			return nil, err
		}
		var account model.User
		if err := model.DB.WithContext(ctx).Unscoped().First(&account, binding.UserId).Error; err != nil {
			return nil, err
		}
		return &account, nil
	}
	// Provider implementations own their account mapping. Registered
	// extensions can advertise a binding column while resolving identities
	// through another trusted store; do not replace that mapping with a
	// guessed users-table lookup.
	// Registered extensions may own their account storage. Keep that provider
	// contract while built-in and configured SQL providers use Context reads.
	if !provider.IsUserIDTaken(remoteID) {
		return nil, gorm.ErrRecordNotFound
	}
	account := &model.User{}
	if err := provider.FillUserByProviderID(account, remoteID); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return account, nil
}

func ResolveOAuthLogin(ctx context.Context, request OAuthAccountRequest) (*model.User, *service.LegacyGitHubMigration, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if request.Provider == nil || request.Remote == nil || request.Remote.ProviderUserID == "" {
		return nil, nil, ErrOAuthInput
	}
	account, err := resolveOAuthAccount(ctx, request.Provider, request.Remote.ProviderUserID)
	if err == nil {
		if account.Id <= 0 || account.DeletedAt.Valid {
			return nil, nil, &OAuthUserDeletedError{}
		}
		return account, nil, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	if request.Provider.ProviderUserIDColumn() == "telegram_id" {
		return nil, nil, oauth.ErrTelegramAccountNotBound
	}
	legacyID, _ := request.Remote.Extra["legacy_id"].(string)
	if strings.ContainsFunc(legacyID, func(r rune) bool { return r < '0' || r > '9' }) {
		candidate, lookupErr := resolveOAuthAccount(ctx, request.Provider, legacyID)
		if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return nil, nil, lookupErr
		}
		if lookupErr == nil && candidate.Id > 0 && !candidate.DeletedAt.Valid {
			verification, err := model.GetUserVerificationState(candidate.Id)
			if err != nil {
				return nil, nil, err
			}
			if verification.HasTwoFA || verification.HasPasskey {
				return candidate, &service.LegacyGitHubMigration{GitHubID: request.Remote.ProviderUserID, LegacyID: legacyID}, nil
			}
			matched, reason := false, "no_matching_evidence"
			if evidence, ok := request.Provider.(oauth.VerifiedEmailProvider); ok && candidate.Email != "" {
				emails, err := evidence.GetVerifiedEmails(ctx, request.Token)
				if err != nil {
					reason = "verified_emails_unavailable"
				}
				for _, email := range emails {
					if model.NormalizeEmail(email) == model.NormalizeEmail(candidate.Email) {
						matched = true
						break
					}
				}
			}
			if !matched {
				if request.MigrationAudit != nil {
					request.MigrationAudit(candidate, false, map[string]any{"legacy_id": legacyID, "provider_user_id": request.Remote.ProviderUserID, "reason": reason})
				}
				return nil, nil, &OAuthLegacyBindingNotConfirmedError{}
			}
			written := false
			err = model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				var err error
				written, err = model.MigrateLegacyGitHubBindingWithTx(tx, candidate.Id, legacyID, request.Remote.ProviderUserID)
				return err
			})
			if err != nil {
				return nil, nil, err
			}
			if written {
				candidate.GitHubId = request.Remote.ProviderUserID
				warn := service.NotifyAccountSecurityChange(candidate.Email, "Login account linked: "+request.Provider.GetName()) != nil
				if request.MigrationAudit != nil {
					request.MigrationAudit(candidate, true, map[string]any{"legacy_id": legacyID, "provider_user_id": request.Remote.ProviderUserID, "verified_email_matched": true, "notification_failed": warn})
				}
			}
			return candidate, nil, nil
		}
	}
	if !common.RegisterEnabled {
		return nil, nil, &OAuthRegistrationDisabledError{}
	}
	username := strings.TrimSpace(request.Remote.Username)
	if username == "" || utf8.RuneCountInString(username) > model.UserNameMaxLength {
		username = ""
	}
	if username != "" {
		exists, err := model.CheckUserExistOrDeleted(username, "")
		if err != nil {
			return nil, nil, err
		}
		if exists {
			username = ""
		}
	}
	if username == "" {
		prefix := []rune(request.Provider.GetProviderPrefix())
		if len(prefix) > 10 {
			prefix = prefix[:10]
		}
		username = string(prefix) + common.GetRandomString(10)
	}
	display := request.Remote.DisplayName
	if display == "" {
		display = request.Remote.Username
	}
	if display == "" {
		display = request.Provider.GetName() + " User"
	}
	if runes := []rune(display); len(runes) > 20 {
		display = string(runes[:20])
	}
	account = &model.User{Username: username, DisplayName: display, Email: model.NormalizeEmail(request.Remote.Email), Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	inviter := 0
	if request.AffiliateCode != "" {
		inviter, _ = model.GetUserIdByAffCode(request.AffiliateCode)
	}
	err = model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, generic := request.Provider.(*oauth.GenericOAuthProvider); !generic {
			request.Provider.SetProviderUserID(account, request.Remote.ProviderUserID)
		}
		account.Role, account.Status = common.RoleCommonUser, common.UserStatusEnabled
		if err := account.InsertWithTx(tx, inviter); err != nil {
			return err
		}
		if provider, ok := request.Provider.(*oauth.GenericOAuthProvider); ok {
			return model.CreateUserOAuthBindingWithTx(tx, &model.UserOAuthBinding{UserId: account.Id, ProviderId: provider.GetProviderId(), ProviderUserId: request.Remote.ProviderUserID})
		}
		if _, linuxdo := request.Provider.(*oauth.LinuxDOProvider); linuxdo {
			return model.ApplyFurtherLinuxDORegistration(tx, account, request.Admissions...)
		}
		return nil
	})
	if errors.Is(err, model.ErrEmailAlreadyTaken) {
		return nil, nil, &OAuthEmailAlreadyTakenError{}
	}
	if err != nil {
		return nil, nil, err
	}
	account.FinalizeOAuthUserCreation(inviter)
	return account, nil, nil
}

func BindOAuthAccount(ctx context.Context, identity service.AuthIdentity, providerName string, provider oauth.Provider, remoteID, token string, match model.AuthFlowMatch, payload OAuthFlowPayload) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	proofContext, err := common.Marshal(service.AccountBindingContext{Provider: providerName})
	if err != nil {
		return false, err
	}
	if err := service.ValidateFlowAuthorization(identity, service.VerificationOperation{Scope: service.VerificationScopeAccountBind, Context: proofContext}, payload.Authorization); err != nil {
		return false, err
	}
	if _, err := resolveOAuthAccount(ctx, provider, remoteID); err == nil {
		return false, ErrOAuthAlreadyBound
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	_, err = model.ConsumeAuthFlowWithActionContext(ctx, token, match, func(tx *gorm.DB, _ *model.AuthFlow) error {
		if providerName == "telegram" {
			return model.BindTelegramForSessionWithTx(tx, identity, remoteID)
		}
		if custom, ok := provider.(*oauth.GenericOAuthProvider); ok {
			return model.UpdateUserOAuthBindingForSessionWithTx(tx, identity, custom.GetProviderId(), remoteID)
		}
		return model.UpdateUserBindColumnForSessionWithTx(tx, identity, provider.ProviderUserIDColumn(), remoteID)
	})
	if err != nil {
		return false, err
	}
	return true, nil
}
