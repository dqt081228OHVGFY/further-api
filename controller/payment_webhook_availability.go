package controller

import (
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"strings"
)

// Retired gateways can settle historical orders. Callback verification must
// remain independent of retired checkout products, enable switches and policy.
func isStripeWebhookConfigured() bool { return strings.TrimSpace(setting.StripeWebhookSecret) != "" }
func isStripeWebhookEnabled() bool    { return isStripeWebhookConfigured() }
func isCreemWebhookConfigured() bool  { return strings.TrimSpace(setting.CreemWebhookSecret) != "" }
func isCreemWebhookEnabled() bool     { return isCreemWebhookConfigured() }
func isWaffoWebhookConfigured() bool {
	if setting.WaffoSandbox {
		return strings.TrimSpace(setting.WaffoSandboxApiKey) != "" && strings.TrimSpace(setting.WaffoSandboxPrivateKey) != "" && strings.TrimSpace(setting.WaffoSandboxPublicCert) != ""
	}
	return strings.TrimSpace(setting.WaffoApiKey) != "" && strings.TrimSpace(setting.WaffoPrivateKey) != "" && strings.TrimSpace(setting.WaffoPublicCert) != ""
}
func isWaffoWebhookEnabled() bool { return isWaffoWebhookConfigured() }

// Pancake embeds verification public keys in the SDK. Verifying historical
// events needs no merchant checkout credentials; local order binding is checked.
func isWaffoPancakeWebhookConfigured() bool { return true }
func isWaffoPancakeWebhookEnabled() bool    { return isWaffoPancakeWebhookConfigured() }
func isEpayWebhookConfigured() bool {
	return strings.TrimSpace(operation_setting.PayAddress) != "" && strings.TrimSpace(operation_setting.EpayId) != "" && strings.TrimSpace(operation_setting.EpayKey) != ""
}
func isEpayWebhookEnabled() bool { return isEpayWebhookConfigured() }
