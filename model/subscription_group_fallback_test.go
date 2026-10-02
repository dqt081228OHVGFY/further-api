package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func createOverlappingGroupSubscriptions(t *testing.T, shortExpired bool) (User, UserSubscription, UserSubscription) {
	t.Helper()
	now := GetDBTimestamp()
	user := User{
		Username: "overlapping-group-user",
		Password: "unused-password-hash",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "svip",
	}
	require.NoError(t, DB.Create(&user).Error)
	vip := UserSubscription{
		UserId: user.Id, Status: "active", StartTime: now - 200,
		EndTime:      now + int64(30*24*time.Hour/time.Second),
		UpgradeGroup: "vip", PrevUserGroup: "default",
	}
	require.NoError(t, DB.Create(&vip).Error)
	svipEnd := now + int64(24*time.Hour/time.Second)
	if shortExpired {
		svipEnd = now - 1
	}
	svip := UserSubscription{
		UserId: user.Id, Status: "active", StartTime: now - 100,
		EndTime:      svipEnd,
		UpgradeGroup: "svip", PrevUserGroup: "vip",
	}
	require.NoError(t, DB.Create(&svip).Error)
	return user, vip, svip
}

func TestExpireDueSubscriptionsRestoresSurvivingPlanGroup(t *testing.T) {
	setupRelayForStatsTest(t)
	user, vip, svip := createOverlappingGroupSubscriptions(t, true)

	expired, err := ExpireDueSubscriptions(20)
	require.NoError(t, err)
	require.Equal(t, 1, expired)

	var updated User
	require.NoError(t, DB.First(&updated, user.Id).Error)
	require.Equal(t, "vip", updated.Group)
	var storedVIP, storedSVIP UserSubscription
	require.NoError(t, DB.First(&storedVIP, vip.Id).Error)
	require.NoError(t, DB.First(&storedSVIP, svip.Id).Error)
	require.Equal(t, "active", storedVIP.Status)
	require.Equal(t, "expired", storedSVIP.Status)
}

func TestAdminInvalidateSubscriptionRestoresSurvivingPlanGroup(t *testing.T) {
	setupRelayForStatsTest(t)
	user, vip, svip := createOverlappingGroupSubscriptions(t, false)

	message, err := AdminInvalidateUserSubscription(svip.Id)
	require.NoError(t, err)
	require.Contains(t, message, "vip")

	var updated User
	require.NoError(t, DB.First(&updated, user.Id).Error)
	require.Equal(t, "vip", updated.Group)
	var storedVIP, storedSVIP UserSubscription
	require.NoError(t, DB.First(&storedVIP, vip.Id).Error)
	require.NoError(t, DB.First(&storedSVIP, svip.Id).Error)
	require.Equal(t, "active", storedVIP.Status)
	require.Equal(t, "cancelled", storedSVIP.Status)
}

func TestAdminDeleteSubscriptionRestoresSurvivingPlanGroup(t *testing.T) {
	setupRelayForStatsTest(t)
	user, vip, svip := createOverlappingGroupSubscriptions(t, false)

	message, err := AdminDeleteUserSubscription(svip.Id)
	require.NoError(t, err)
	require.Contains(t, message, "vip")

	var updated User
	require.NoError(t, DB.First(&updated, user.Id).Error)
	require.Equal(t, "vip", updated.Group)
	var storedVIP UserSubscription
	require.NoError(t, DB.First(&storedVIP, vip.Id).Error)
	require.Equal(t, "active", storedVIP.Status)
	var deleted int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", svip.Id).Count(&deleted).Error)
	require.Zero(t, deleted)
}
