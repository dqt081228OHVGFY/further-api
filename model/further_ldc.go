package model

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const PaymentProviderLDC = "ldc"

// Credentials are versioned so key rotation never strands a pending order.
// They are never returned by the settings, order, or history APIs.
type FurtherLDCCredential struct {
	ID           int    `json:"-" gorm:"primaryKey"`
	ClientID     string `json:"-" gorm:"size:128;not null"`
	ClientSecret string `json:"-" gorm:"type:text;not null"`
	CreatedAt    int64  `json:"-"`
}

type FurtherLDCSettings struct {
	ID              int    `json:"-" gorm:"primaryKey"`
	Enabled         bool   `json:"enabled"`
	ClientID        string `json:"client_id" gorm:"size:128"`
	ClientSecret    string `json:"-" gorm:"-"`
	CredentialID    int    `json:"-"`
	PrivateKey      string `json:"-" gorm:"type:text"`
	CreditsPerUnit  string `json:"credits_per_unit" gorm:"size:64"`
	MinTopUp        string `json:"min_topup" gorm:"size:64"`
	CallbackBaseURL string `json:"callback_base_url" gorm:"size:240"`
	UpdatedAt       int64  `json:"updated_at"`
}

type FurtherLDCOrder struct {
	ID             int    `json:"-" gorm:"primaryKey"`
	TradeNo        string `json:"trade_no" gorm:"size:64;not null;uniqueIndex"`
	UserID         int    `json:"-" gorm:"not null;index"`
	Kind           string `json:"kind" gorm:"size:16;not null"`
	PlanID         int    `json:"plan_id"`
	PlanTitle      string `json:"plan_title" gorm:"size:240"`
	PlanSnapshot   string `json:"-" gorm:"type:text"`
	WalletUnits    string `json:"wallet_units" gorm:"size:64"`
	Quota          int    `json:"quota"`
	Credits        string `json:"credits" gorm:"size:64;not null"`
	CreditsPerUnit string `json:"credits_per_unit" gorm:"size:64"`
	CredentialID   int    `json:"-"`
	Status         string `json:"status" gorm:"size:32;index"`
	GatewayTradeNo string `json:"-" gorm:"size:128"`
	CreatedAt      int64  `json:"created_at"`
	CompletedAt    int64  `json:"completed_at"`
}

func GetFurtherLDCSettings() (FurtherLDCSettings, error) {
	cfg := FurtherLDCSettings{ID: 1, CreditsPerUnit: "1", MinTopUp: "1"}
	err := DB.First(&cfg, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if cfg.CredentialID > 0 {
		var credential FurtherLDCCredential
		if err := DB.First(&credential, cfg.CredentialID).Error; err != nil {
			return cfg, err
		}
		cfg.ClientSecret = credential.ClientSecret
	}
	return cfg, nil
}

func ParseFurtherLDCPrivateKey(raw string) (ed25519.PrivateKey, error) {
	raw = strings.TrimSpace(raw)
	if block, rest := pem.Decode([]byte(raw)); block != nil && len(strings.TrimSpace(string(rest))) == 0 {
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err == nil {
			if key, ok := parsed.(ed25519.PrivateKey); ok {
				return key, nil
			}
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err == nil {
		if len(decoded) == ed25519.SeedSize {
			return ed25519.NewKeyFromSeed(decoded), nil
		}
		if len(decoded) == ed25519.PrivateKeySize {
			key := ed25519.NewKeyFromSeed(decoded[:ed25519.SeedSize])
			if key.Equal(ed25519.PrivateKey(decoded)) {
				return key, nil
			}
		}
	}
	return nil, errors.New("Ed25519 私钥须为 PKCS8 PEM，或 Base64 编码的 32 字节种子/64 字节私钥")
}

func FurtherLDCPublicKeys(privateKey string) (raw, publicPEM string) {
	key, err := ParseFurtherLDCPrivateKey(privateKey)
	if err != nil {
		return "", ""
	}
	publicKey := key.Public().(ed25519.PublicKey)
	der, _ := x509.MarshalPKIXPublicKey(publicKey)
	return base64.StdEncoding.EncodeToString(publicKey), string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func furtherLDCDecimal(raw string, places int32) (decimal.Decimal, error) {
	if len(raw) == 0 || len(raw) > 32 || strings.ContainsAny(raw, "eE+- \t\n\r") {
		return decimal.Zero, errors.New("金额格式无效")
	}
	value, err := decimal.NewFromString(raw)
	if err != nil || !value.GreaterThan(decimal.Zero) || value.Exponent() < -places {
		return decimal.Zero, errors.New("金额必须为正数且小数位数在允许范围内")
	}
	return value, nil
}

func SaveFurtherLDCSettings(cfg FurtherLDCSettings) (FurtherLDCSettings, error) {
	old, err := GetFurtherLDCSettings()
	if err != nil {
		return cfg, err
	}
	cfg.ID, cfg.ClientID = 1, strings.TrimSpace(cfg.ClientID)
	cfg.ClientSecret, cfg.PrivateKey = strings.TrimSpace(cfg.ClientSecret), strings.TrimSpace(cfg.PrivateKey)
	if cfg.ClientID != old.ClientID && old.ClientID != "" && cfg.ClientSecret == "" {
		return cfg, errors.New("更换 LDC Client ID 时请同时填写对应的 Client Secret")
	}
	if cfg.ClientSecret == "" {
		cfg.ClientSecret = old.ClientSecret
	}
	if cfg.PrivateKey == "" {
		cfg.PrivateKey = old.PrivateKey
	}
	if cfg.PrivateKey == "" {
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return cfg, err
		}
		cfg.PrivateKey = base64.StdEncoding.EncodeToString(key)
	}
	if _, err := ParseFurtherLDCPrivateKey(cfg.PrivateKey); err != nil {
		return cfg, err
	}
	if len(cfg.ClientID) > 128 || strings.ContainsAny(cfg.ClientID, "&=\r\n\t ") || len(cfg.ClientSecret) > 512 {
		return cfg, errors.New("LDC 商户参数无效")
	}
	if cfg.Enabled && (cfg.ClientID == "" || cfg.ClientSecret == "") {
		return cfg, errors.New("启用 LDC 前请填写 Client ID 和 Client Secret")
	}
	rate, err := furtherLDCDecimal(cfg.CreditsPerUnit, 6)
	if err != nil || rate.GreaterThan(decimal.NewFromInt(1000000000)) {
		return cfg, errors.New("LDC 兑换率必须大于 0，最多 6 位小数且不超过 1000000000")
	}
	minimum, err := furtherLDCDecimal(cfg.MinTopUp, 2)
	if err != nil || minimum.GreaterThan(decimal.NewFromInt(1000000000)) {
		return cfg, errors.New("最低充值数量必须大于 0，最多 2 位小数")
	}
	cfg.CreditsPerUnit, cfg.MinTopUp = rate.String(), minimum.String()
	cfg.CallbackBaseURL = strings.TrimRight(strings.TrimSpace(cfg.CallbackBaseURL), "/")
	if cfg.CallbackBaseURL != "" {
		u, err := url.Parse(cfg.CallbackBaseURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return cfg, errors.New("LDC 回调基础地址必须是 HTTPS 网站地址")
		}
	}
	cfg.UpdatedAt = time.Now().Unix()
	err = DB.Transaction(func(tx *gorm.DB) error {
		if cfg.ClientID != old.ClientID || cfg.ClientSecret != old.ClientSecret || old.CredentialID == 0 {
			credential := FurtherLDCCredential{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, CreatedAt: cfg.UpdatedAt}
			if err := tx.Create(&credential).Error; err != nil {
				return err
			}
			cfg.CredentialID = credential.ID
		} else {
			cfg.CredentialID = old.CredentialID
		}
		return tx.Save(&cfg).Error
	})
	return cfg, err
}

type FurtherLDCQuote struct {
	Kind           string `json:"kind"`
	WalletUnits    string `json:"wallet_units"`
	Quota          int    `json:"quota"`
	Credits        string `json:"credits"`
	CreditsPerUnit string `json:"credits_per_unit"`
	Currency       string `json:"currency"`
	PlanID         int    `json:"plan_id"`
	PlanTitle      string `json:"plan_title"`
}

func quoteFurtherLDCTx(tx *gorm.DB, userID int, amount string, planID int, cfg FurtherLDCSettings) (FurtherLDCQuote, *SubscriptionPlan, error) {
	quote := FurtherLDCQuote{Kind: "topup", CreditsPerUnit: cfg.CreditsPerUnit, Currency: "LDC"}
	if !cfg.Enabled || cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.PrivateKey == "" {
		return quote, nil, errors.New("LDC 支付尚未启用或配置未完成")
	}
	rate, err := furtherLDCDecimal(cfg.CreditsPerUnit, 6)
	if err != nil {
		return quote, nil, errors.New("LDC 兑换率配置无效")
	}
	var plan *SubscriptionPlan
	var units decimal.Decimal
	if planID > 0 {
		if amount != "" {
			return quote, nil, errors.New("不能同时提交充值数量和订阅套餐")
		}
		plan = &SubscriptionPlan{}
		if err := lockForUpdate(tx).First(plan, planID).Error; err != nil {
			return quote, nil, errors.New("订阅套餐不存在")
		}
		protected, err := isInternalOrPrepaidSubscriptionPlanTx(tx, planID)
		if err != nil {
			return quote, nil, err
		}
		if !plan.Enabled || protected {
			return quote, nil, errors.New("该套餐当前不可购买")
		}
		if plan.Currency != "" && plan.Currency != "USD" {
			return quote, nil, errors.New("该套餐币种不支持 LDC 兑换")
		}
		units = decimal.NewFromFloat(plan.PriceAmount)
		quote.Kind, quote.PlanID, quote.PlanTitle = "subscription", plan.Id, plan.Title
	} else {
		units, err = furtherLDCDecimal(amount, 2)
		if err != nil {
			return quote, nil, err
		}
		minimum, err := furtherLDCDecimal(cfg.MinTopUp, 2)
		if err != nil || units.LessThan(minimum) {
			return quote, nil, fmt.Errorf("最低充值数量为 %s USD 钱包单位", cfg.MinTopUp)
		}
	}
	if !units.GreaterThan(decimal.Zero) {
		return quote, nil, errors.New("付款数量必须大于 0")
	}
	credits := units.Mul(rate).Round(2)
	if credits.LessThan(decimal.NewFromFloat(0.01)) || credits.GreaterThan(decimal.NewFromInt(1000000000)) {
		return quote, nil, errors.New("LDC 付款积分须在 0.01 至 1000000000 之间")
	}
	quote.WalletUnits, quote.Credits = units.String(), credits.StringFixed(2)
	var user User
	if err := lockForUpdate(tx).First(&user, userID).Error; err != nil || user.Status != common.UserStatusEnabled {
		return quote, nil, errors.New("付款账户不可用")
	}
	if plan == nil {
		quota, err := common.WalletQuotaFromDecimalStrict(units.Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
		if err != nil || quota <= 0 || user.Quota > common.MaxWalletQuota-quota {
			return quote, nil, errors.New("充值超出钱包额度上限")
		}
		quote.Quota = quota
	} else if plan.MaxPurchasePerUser > 0 {
		var count int64
		if err := tx.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ?", userID, planID).Count(&count).Error; err != nil {
			return quote, nil, err
		}
		if count >= int64(plan.MaxPurchasePerUser) {
			return quote, nil, errors.New("已达到该套餐购买上限")
		}
	}
	return quote, plan, nil
}

func QuoteFurtherLDC(userID int, amount string, planID int, cfg FurtherLDCSettings) (FurtherLDCQuote, error) {
	var quote FurtherLDCQuote
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		quote, _, err = quoteFurtherLDCTx(tx, userID, amount, planID, cfg)
		return err
	})
	return quote, err
}

func CreateFurtherLDCOrder(userID int, amount string, planID int, expectedCredits string, cfg FurtherLDCSettings) (*FurtherLDCOrder, error) {
	expected, err := furtherLDCDecimal(expectedCredits, 2)
	if err != nil {
		return nil, errors.New("请先获取并确认 LDC 付款报价")
	}
	random := make([]byte, 20)
	if _, err := rand.Read(random); err != nil {
		return nil, err
	}
	order := &FurtherLDCOrder{TradeNo: "LDC" + hex.EncodeToString(random), UserID: userID, Status: common.TopUpStatusPending, CreatedAt: time.Now().Unix(), CredentialID: cfg.CredentialID}
	err = DB.Transaction(func(tx *gorm.DB) error {
		var current FurtherLDCSettings
		if err := lockForUpdate(tx).First(&current, 1).Error; err != nil {
			return errors.New("LDC 支付配置不可用")
		}
		if !current.Enabled || current.CredentialID != cfg.CredentialID || current.CreditsPerUnit != cfg.CreditsPerUnit || current.MinTopUp != cfg.MinTopUp || current.PrivateKey != cfg.PrivateKey {
			return errors.New("LDC 支付配置已变更，请重新获取报价")
		}
		quote, plan, err := quoteFurtherLDCTx(tx, userID, amount, planID, cfg)
		if err != nil {
			return err
		}
		actual, _ := decimal.NewFromString(quote.Credits)
		if !actual.Equal(expected) {
			return errors.New("LDC 报价已变更，请重新确认")
		}
		order.Kind, order.PlanID, order.PlanTitle = quote.Kind, quote.PlanID, quote.PlanTitle
		order.WalletUnits, order.Quota, order.Credits, order.CreditsPerUnit = quote.WalletUnits, quote.Quota, quote.Credits, quote.CreditsPerUnit
		if plan != nil {
			// Keep exactly the entitlement the customer confirmed, even if the
			// administrator changes the plan after checkout creation.
			snapshot, err := json.Marshal(plan)
			if err != nil {
				return err
			}
			order.PlanSnapshot = string(snapshot)
		}
		return tx.Create(order).Error
	})
	return order, err
}

func GetFurtherLDCOrder(userID int, tradeNo string) (*FurtherLDCOrder, error) {
	var order FurtherLDCOrder
	query := DB.Where("trade_no = ?", tradeNo)
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.First(&order).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

// CompleteFurtherLDCOrder accepts only independently queried gateway data.
// The caller authenticates to LDC over HTTPS; callback form values alone must
// never reach this settlement function.
func CompleteFurtherLDCOrder(tradeNo, gatewayTradeNo, clientID, credits string) error {
	paid, err := furtherLDCDecimal(credits, 2)
	if err != nil || gatewayTradeNo == "" || len(gatewayTradeNo) > 128 {
		return errors.New("LDC 查单结果无效")
	}
	var order FurtherLDCOrder
	credited, groupChanged := false, false
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
			return err
		}
		var credential FurtherLDCCredential
		if err := tx.First(&credential, order.CredentialID).Error; err != nil {
			return err
		}
		amount, err := furtherLDCDecimal(order.Credits, 2)
		if err != nil || !paid.Equal(amount) || credential.ClientID != clientID || (order.GatewayTradeNo != "" && order.GatewayTradeNo != gatewayTradeNo) {
			return errors.New("LDC 订单商户、金额或平台单号不匹配")
		}
		if order.Status == common.TopUpStatusSuccess {
			return nil
		}
		if order.Status != common.TopUpStatusPending {
			return errors.New("LDC 订单状态不可结算")
		}
		var user User
		if err := lockForUpdate(tx).First(&user, order.UserID).Error; err != nil {
			return err
		}
		if order.Kind == "topup" {
			if err := creditTopUpQuota(tx, order.UserID, order.Quota, nil); err != nil {
				return err
			}
		} else if order.Kind == "subscription" {
			var plan SubscriptionPlan
			if err := json.Unmarshal([]byte(order.PlanSnapshot), &plan); err != nil || plan.Id != order.PlanID {
				return errors.New("LDC 套餐快照无效")
			}
			subscription, err := CreateUserSubscriptionFromPlanTx(tx, order.UserID, &plan, "order")
			if err != nil {
				return err
			}
			groupChanged = subscription.PrevUserGroup != ""
		} else {
			return errors.New("LDC 订单类型无效")
		}
		money, _ := paid.Float64()
		// Keep the shared history ledger without routing through any retired
		// provider. LDC order detail holds the exact decimal and quota snapshot.
		topup := TopUp{UserId: order.UserID, TradeNo: order.TradeNo, Money: money, WalletUnits: order.WalletUnits, CreditedQuota: order.Quota, PaymentMethod: PaymentProviderLDC, PaymentProvider: PaymentProviderLDC, CreateTime: order.CreatedAt, CompleteTime: time.Now().Unix(), Status: common.TopUpStatusSuccess}
		if order.Kind == "topup" {
			units, _ := decimal.NewFromString(order.WalletUnits)
			topup.Amount = units.IntPart()
		}
		if err := tx.Create(&topup).Error; err != nil {
			return err
		}
		order.Status, order.GatewayTradeNo, order.CompletedAt = common.TopUpStatusSuccess, gatewayTradeNo, topup.CompleteTime
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		credited = true
		return nil
	})
	if err != nil {
		return err
	}
	if credited {
		if order.Kind == "topup" {
			syncCreditUserQuotaCache(order.UserID, order.Quota, "LDC topup")
		}
		if groupChanged {
			refreshSubscriptionUserGroupCache(order.UserID, "LDC subscription")
		}
		RecordLog(order.UserID, LogTypeTopup, fmt.Sprintf("LDC 付款完成：%s，%s LDC", order.TradeNo, order.Credits))
	}
	return nil
}
