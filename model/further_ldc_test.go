package model

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"sync"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/stretchr/testify/require"
)

func setupLDCTest(t *testing.T) FurtherLDCSettings {
	t.Helper()
	setupFurtherBenefitsDB(t)
	require.NoError(t, DB.AutoMigrate(&FurtherLDCSettings{}, &FurtherLDCCredential{}, &FurtherLDCOrder{}, &TopUp{}, &SubscriptionPlan{}, &UserSubscription{}))
	sqlDB, err := DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	cfg, err := SaveFurtherLDCSettings(FurtherLDCSettings{Enabled: true, ClientID: "ldc-test-merchant", ClientSecret: "fake-test-secret", CreditsPerUnit: "10", MinTopUp: "0.01", CallbackBaseURL: "https://example.test"})
	require.NoError(t, err)
	return cfg
}

func TestFurtherLDCKeyFormatsAndConfigRotation(t *testing.T) {
	cfg := setupLDCTest(t)
	key, err := ParseFurtherLDCPrivateKey(cfg.PrivateKey)
	require.NoError(t, err)
	public, publicPEM := FurtherLDCPublicKeys(cfg.PrivateKey)
	decoded, err := base64.StdEncoding.DecodeString(public)
	require.NoError(t, err)
	require.Equal(t, []byte(key.Public().(ed25519.PublicKey)), decoded)
	block, _ := pem.Decode([]byte(publicPEM))
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	require.NoError(t, err)
	require.True(t, parsed.(ed25519.PublicKey).Equal(key.Public()))
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	for _, encoded := range []string{base64.StdEncoding.EncodeToString(key.Seed()), string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))} {
		actual, err := ParseFurtherLDCPrivateKey(encoded)
		require.NoError(t, err)
		require.True(t, actual.Equal(key))
	}
	_, err = ParseFurtherLDCPrivateKey("invalid")
	require.Error(t, err)
	oldID := cfg.CredentialID
	cfg.ClientSecret = "rotated-fake-test-secret"
	cfg, err = SaveFurtherLDCSettings(cfg)
	require.NoError(t, err)
	require.NotEqual(t, oldID, cfg.CredentialID)
	var old FurtherLDCCredential
	require.NoError(t, DB.First(&old, oldID).Error)
	require.Equal(t, "fake-test-secret", old.ClientSecret)
	cfg.ClientSecret, cfg.PrivateKey = "", ""
	cfg, err = SaveFurtherLDCSettings(cfg)
	require.NoError(t, err)
	require.Equal(t, "rotated-fake-test-secret", cfg.ClientSecret)
	require.NotEmpty(t, cfg.PrivateKey)
	cfg.CallbackBaseURL = "https://example.test/?secret=not-allowed"
	_, err = SaveFurtherLDCSettings(cfg)
	require.Error(t, err)
}

func TestFurtherLDCQuoteAndSnapshotAmount(t *testing.T) {
	cfg := setupLDCTest(t)
	user := activityTestUser(t, "ldc-fraction", 0)
	quote, err := QuoteFurtherLDC(user.Id, "1.25", 0, cfg)
	require.NoError(t, err)
	require.Equal(t, "12.50", quote.Credits)
	require.Equal(t, int(1.25*common.QuotaPerUnit), quote.Quota)
	for _, value := range []string{"", "0", "-1", "NaN", "1e9", "1.001", "10000000000000000000000000000000000"} {
		_, err := QuoteFurtherLDC(user.Id, value, 0, cfg)
		require.Error(t, err, value)
	}
	_, err = CreateFurtherLDCOrder(user.Id, "1.25", 0, "1.00", cfg)
	require.ErrorContains(t, err, "报价")
	order, err := CreateFurtherLDCOrder(user.Id, "1.25", 0, "12.50", cfg)
	require.NoError(t, err)
	previousQuotaUnit := common.QuotaPerUnit
	common.QuotaPerUnit *= 2
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaUnit })
	// The wallet credit is fixed at checkout, not recomputed after settings change.
	require.NoError(t, CompleteFurtherLDCOrder(order.TradeNo, "7001", cfg.ClientID, "12.50"))
	require.NoError(t, DB.First(&user, user.Id).Error)
	require.Equal(t, order.Quota, user.Quota)
	var ledger TopUp
	require.NoError(t, DB.First(&ledger, "trade_no = ?", order.TradeNo).Error)
	require.Equal(t, "1.25", ledger.WalletUnits)
	require.Equal(t, order.Quota, ledger.CreditedQuota)
}

func TestFurtherLDCSettlementRejectsForgeryAndDuplicate(t *testing.T) {
	cfg := setupLDCTest(t)
	user := activityTestUser(t, "ldc-concurrent", 0)
	order, err := CreateFurtherLDCOrder(user.Id, "1", 0, "10.00", cfg)
	require.NoError(t, err)
	require.Error(t, CompleteFurtherLDCOrder(order.TradeNo, "7002", "another-merchant", "10.00"))
	require.Error(t, CompleteFurtherLDCOrder(order.TradeNo, "7002", cfg.ClientID, "9.99"))
	require.Error(t, CompleteFurtherLDCOrder("unknown-order", "7002", cfg.ClientID, "10.00"))
	var wait sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results <- CompleteFurtherLDCOrder(order.TradeNo, "7002", cfg.ClientID, "10.00")
		}()
	}
	wait.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	require.NoError(t, DB.First(&user, user.Id).Error)
	require.Equal(t, order.Quota, user.Quota)
	var count int64
	require.NoError(t, DB.Model(&TopUp{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.Error(t, CompleteFurtherLDCOrder(order.TradeNo, "different-platform-order", cfg.ClientID, "10.00"))
	other := activityTestUser(t, "ldc-other", 0)
	_, err = GetFurtherLDCOrder(other.Id, order.TradeNo)
	require.Error(t, err)
}

func TestFurtherLDCSettlementRollbackAndDisabledPurchases(t *testing.T) {
	cfg := setupLDCTest(t)
	user := activityTestUser(t, "ldc-overflow", 0)
	order, err := CreateFurtherLDCOrder(user.Id, "1", 0, "10.00", cfg)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&user).Update("quota", common.MaxWalletQuota).Error)
	require.Error(t, CompleteFurtherLDCOrder(order.TradeNo, "7003", cfg.ClientID, "10.00"))
	stored, err := GetFurtherLDCOrder(user.Id, order.TradeNo)
	require.NoError(t, err)
	require.Equal(t, common.TopUpStatusPending, stored.Status)
	var count int64
	require.NoError(t, DB.Model(&TopUp{}).Count(&count).Error)
	require.Zero(t, count)
	cfg.Enabled = false
	_, err = CreateFurtherLDCOrder(user.Id, "1", 0, "10.00", cfg)
	require.Error(t, err)
	stale := cfg
	stale.Enabled = true
	_, err = SaveFurtherLDCSettings(cfg)
	require.NoError(t, err)
	_, err = CreateFurtherLDCOrder(user.Id, "1", 0, "10.00", stale)
	require.ErrorContains(t, err, "配置已变更")
	require.NoError(t, DB.Model(&user).Update("quota", 0).Error)
	// Existing paid orders still settle while new purchases are disabled.
	require.NoError(t, CompleteFurtherLDCOrder(order.TradeNo, "7003", cfg.ClientID, "10.00"))
}

func TestFurtherLDCSubscriptionSnapshotAndPlanGuards(t *testing.T) {
	cfg := setupLDCTest(t)
	user := activityTestUser(t, "ldc-subscriber", 0)
	plan := SubscriptionPlan{Title: "LDC monthly", PriceAmount: 2, Currency: "USD", TotalAmount: 12345, DurationUnit: "month", DurationValue: 1, MaxPurchasePerUser: 1}
	require.NoError(t, DB.Create(&plan).Error)
	order, err := CreateFurtherLDCOrder(user.Id, "", plan.Id, "20.00", cfg)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&plan).Updates(map[string]any{"price_amount": 5, "total_amount": 999, "enabled": false}).Error)
	require.NoError(t, CompleteFurtherLDCOrder(order.TradeNo, "7004", cfg.ClientID, "20.00"))
	require.NoError(t, CompleteFurtherLDCOrder(order.TradeNo, "7004", cfg.ClientID, "20.00"))
	var subscriptions []UserSubscription
	require.NoError(t, DB.Where("user_id = ?", user.Id).Find(&subscriptions).Error)
	require.Len(t, subscriptions, 1)
	require.EqualValues(t, 12345, subscriptions[0].AmountTotal)
	_, err = CreateFurtherLDCOrder(user.Id, "", plan.Id, "50.00", cfg)
	require.Error(t, err)
	require.NoError(t, DB.Model(&plan).Update("enabled", true).Error)
	_, err = CreateFurtherLDCOrder(user.Id, "", plan.Id, "50.00", cfg)
	require.ErrorContains(t, err, "上限")
	require.NoError(t, DB.AutoMigrate(&FurtherSubscriptionPolicy{}))
	internal := SubscriptionPlan{Title: "internal", PriceAmount: 1}
	require.NoError(t, DB.Create(&internal).Error)
	require.NoError(t, DB.Create(&FurtherSubscriptionPolicy{PlanID: internal.Id, Internal: true, Code: "ldc-internal"}).Error)
	_, err = CreateFurtherLDCOrder(user.Id, "", internal.Id, "10.00", cfg)
	require.ErrorContains(t, err, "不可购买")
}

func TestFurtherLDCInvalidPrivateKeyPublicSuffix(t *testing.T) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	key[len(key)-1] ^= 1
	_, err = ParseFurtherLDCPrivateKey(base64.StdEncoding.EncodeToString(key))
	require.Error(t, err)
}
