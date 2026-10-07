package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func namespaceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:namespace_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sql.Close() })
	return db
}

func TestBusinessNamespacePreservesRecordsAndIsRestartable(t *testing.T) {
	db := namespaceTestDB(t)
	require.NoError(t, db.Exec("CREATE TABLE relay_for_loan_accounts (id INTEGER PRIMARY KEY, quota INTEGER)").Error)
	require.NoError(t, db.Exec("INSERT INTO relay_for_loan_accounts VALUES (7, 12345)").Error)
	require.NoError(t, migrateBusinessNamespace(db))
	require.NoError(t, migrateBusinessNamespace(db))
	require.False(t, db.Migrator().HasTable("relay_for_loan_accounts"))
	var quota int
	require.NoError(t, db.Table("further_loan_accounts").Where("id = ?", 7).Pluck("quota", &quota).Error)
	require.Equal(t, 12345, quota)
}

func TestBusinessNamespaceConflictDoesNotOverwriteEitherTable(t *testing.T) {
	db := namespaceTestDB(t)
	for _, name := range []string{"relay_for_loan_accounts", "further_loan_accounts", "relay_for_benefit_grants"} {
		require.NoError(t, db.Exec("CREATE TABLE "+name+" (id INTEGER PRIMARY KEY)").Error)
	}
	require.ErrorContains(t, migrateBusinessNamespace(db), "business namespace conflict")
	require.True(t, db.Migrator().HasTable("relay_for_benefit_grants"))
	require.True(t, db.Migrator().HasTable("relay_for_loan_accounts"))
	require.True(t, db.Migrator().HasTable("further_loan_accounts"))
}

func TestBusinessNamespaceKeepsCurrentModelBalancesAndPreferences(t *testing.T) {
	db := namespaceTestDB(t)
	require.NoError(t, db.Table("relay_for_loan_accounts").AutoMigrate(&FurtherLoanAccount{}))
	require.NoError(t, db.Table("relay_for_ui_preferences").AutoMigrate(&FurtherUIPreferences{}))
	account := FurtherLoanAccount{UserID: 91, ActiveLoanID: 123, TemporaryBonusQuota: 89000}
	preferences := FurtherUIPreferences{UserID: 91, Theme: "dark", Timezone: "Asia/Shanghai", PageSize: 50}
	require.NoError(t, db.Table("relay_for_loan_accounts").Create(&account).Error)
	require.NoError(t, db.Table("relay_for_ui_preferences").Create(&preferences).Error)
	require.NoError(t, migrateBusinessNamespace(db))
	require.NoError(t, db.AutoMigrate(&FurtherLoanAccount{}, &FurtherUIPreferences{}))
	var migratedAccount FurtherLoanAccount
	var migratedPreferences FurtherUIPreferences
	require.NoError(t, db.First(&migratedAccount, "user_id = ?", 91).Error)
	require.NoError(t, db.First(&migratedPreferences, "user_id = ?", 91).Error)
	require.Equal(t, account, migratedAccount)
	require.Equal(t, preferences, migratedPreferences)
	require.NoError(t, migrateBusinessNamespace(db))
}

func TestBusinessValuesPreserveDestinationAndCustomContent(t *testing.T) {
	db := namespaceTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}, &UserSubscription{}))
	require.NoError(t, db.Create(&[]Option{{Key: "RelayForNotice", Value: "old"}, {Key: "FurtherNotice", Value: "current"}, {Key: "relayfor.payment", Value: "saved"}, {Key: "CustomNotice", Value: "RelayFor user content"}}).Error)
	require.NoError(t, db.Create(&UserSubscription{UserId: 1, Source: "relayfor_prepaid"}).Error)
	require.NoError(t, migrateBusinessValues(db))
	require.NoError(t, migrateBusinessValues(db))
	var options []Option
	require.NoError(t, db.Order("key").Find(&options).Error)
	require.Equal(t, []Option{{Key: "CustomNotice", Value: "RelayFor user content"}, {Key: "FurtherNotice", Value: "current"}, {Key: "further.payment", Value: "saved"}}, options)
	var subscription UserSubscription
	require.NoError(t, db.First(&subscription).Error)
	require.Equal(t, "further_prepaid", subscription.Source)
}
