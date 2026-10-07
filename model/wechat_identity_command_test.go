package model

import (
	"context"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestWeChatIdentityResolvesSingleOwnerAndPreservesExistingAccount(t *testing.T) {
	db := businessCommandDatabase(t)
	require.NoError(t, db.AutoMigrate(&ExternalIdentityClaim{}, &Log{}))
	account, err := ResolveWeChatAccountContext(context.Background(), "verified-wechat-subject", true)
	require.NoError(t, err)
	require.Contains(t, account.Username, "wechat_")
	repeat, err := ResolveWeChatAccountContext(context.Background(), "verified-wechat-subject", false)
	require.NoError(t, err)
	require.Equal(t, account.Id, repeat.Id)
	var count int64
	require.NoError(t, db.Model(&User{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Model(&User{}).Where("id = ?", account.Id).Update("status", common.UserStatusDisabled).Error)
	_, err = ResolveWeChatAccountContext(context.Background(), "verified-wechat-subject", true)
	require.ErrorIs(t, err, ErrUserSessionInactive)
}

func TestWeChatIdentityDoesNotAcceptConflictingClaimOrCanceledCreation(t *testing.T) {
	db := businessCommandDatabase(t)
	require.NoError(t, db.AutoMigrate(&ExternalIdentityClaim{}, &Log{}))
	owner := User{Username: "wechat-existing", AffCode: "wechat-existing", WeChatId: "subject"}
	other := User{Username: "wechat-conflicting", AffCode: "wechat-conflicting"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderWeChat, wechatClaimSubject("subject"), other.Id)
	}))
	_, err := ResolveWeChatAccountContext(context.Background(), "subject", true)
	require.ErrorIs(t, err, ErrExternalIdentityAlreadyClaimed)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ResolveWeChatAccountContext(ctx, "new-subject", true)
	require.ErrorIs(t, err, context.Canceled)
}
