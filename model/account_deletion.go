package model

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
)

// The proof has already been consumed by the request boundary. Account state,
// version and database session revocation commit together; an action failure
// rolls those changes back without restoring that one-use proof.
func deleteAccountForIdentityContext(ctx context.Context, account *User, identity *AuthSessionIdentity) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if account == nil || account.Id <= 0 {
		return errors.New("账户编号无效。")
	}
	var version int64
	var revoked []UserSession
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := reserveAccountMutationWriter(tx, account.Id); err != nil {
			return err
		}
		if identity != nil {
			if identity.UserID != account.Id {
				return ErrUserSessionInactive
			}
			if err := ValidateAuthSessionWithTx(tx, *identity); err != nil {
				return err
			}
			var role int
			if err := tx.Model(&User{}).Where("id = ?", account.Id).Select("role").Scan(&role).Error; err != nil {
				return err
			}
			if role == common.RoleRootUser {
				return ErrCannotDeleteRootUser
			}
		}
		var err error
		version, err = IncrementUserAuthVersionWithTx(tx, account.Id)
		if err != nil {
			return err
		}
		revoked, err = revokeDeletingAccountSessionsWithTx(tx, account.Id)
		if err != nil {
			return err
		}
		return tx.Delete(account).Error
	})
	if err != nil {
		return errors.Join(err, ctx.Err())
	}
	var failures []error
	if err := publishCommittedUserAuthVersion(account.Id, version); err != nil {
		failures = append(failures, fmt.Errorf("发布账户删除认证版本：%w", err))
	}
	for index := range revoked {
		if err := writeUserSessionCache(revoked[index].cacheEntry(), time.Time{}); err != nil {
			failures = append(failures, fmt.Errorf("发布删除账户的会话撤销：%w", err))
		}
	}
	if err := invalidateUserCache(account.Id); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

func revokeDeletingAccountSessionsWithTx(tx *gorm.DB, userID int) ([]UserSession, error) {
	now := time.Now().Unix()
	query := tx.Model(&UserSession{}).Where("user_id = ? AND status = ? AND expires_at > ?", userID, UserSessionStatusActive, now)
	var revoked []UserSession
	if common.RedisEnabled {
		if err := lockForUpdate(query).Order("sid ASC").Find(&revoked).Error; err != nil {
			return nil, err
		}
		for index := range revoked {
			if err := writeUserSessionDenyFence(&revoked[index], UserSessionStatusRevoking, now, "user_deleted"); err != nil {
				return nil, err
			}
		}
	}
	if err := query.Updates(map[string]any{
		"status": UserSessionStatusRevoked, "revoked_at": now, "revoked_reason": "user_deleted",
	}).Error; err != nil {
		return nil, err
	}
	for index := range revoked {
		revoked[index].Status = UserSessionStatusRevoked
		revoked[index].RevokedAt, revoked[index].RevokedReason = now, "user_deleted"
	}
	return revoked, nil
}
