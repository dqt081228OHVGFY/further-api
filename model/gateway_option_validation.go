package model

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/setting"
	"github.com/dqt081228OHVGFY/further-api/setting/billing_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/console_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/model_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
)

func validateGatewayOptionValue(key, value string) error {
	if key == "" || strings.TrimSpace(key) != key || len(key) > 255 {
		return fmt.Errorf("设置键名无效")
	}
	if err := common.ValidateBrandingOption(key, value); err != nil {
		return err
	}
	if err := operation_setting.ValidateQuotaOption(key, value); err != nil {
		return err
	}
	switch key {
	case operation_setting.ToolPriceOptionKey:
		return operation_setting.ValidateToolPricesJSON(value)
	case operation_setting.ChannelTestConcurrencyOptionKey:
		return operation_setting.ValidateChannelTestConcurrency(value)
	case "MaxTokenAutoGroups":
		return setting.ValidateMaxTokenAutoGroups(value)
	case "gemini.safety_settings":
		return model_setting.ValidateGeminiSafetySettings(value)
	case "claude.default_max_tokens":
		return model_setting.ValidateClaudeDefaultMaxTokens(value)
	case "LinuxDOMinimumTrustLevel":
		level, err := strconv.Atoi(value)
		if err != nil || level < 0 || level > 4 {
			return fmt.Errorf("Linux DO 最低信任等级须为 0 到 4")
		}
	case "QuotaPerUnit":
		unit, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(unit) || math.IsInf(unit, 0) || unit <= 0 {
			return fmt.Errorf("额度单位须为有效正数")
		}
	case "SMTPPort":
		port, err := strconv.Atoi(value)
		if err != nil || port < 0 || port > 65535 {
			return fmt.Errorf("邮件端口须在 0 到 65535 之间")
		}
	case "ModelRequestRateLimitGroup":
		return setting.CheckModelRequestRateLimitGroup(value)
	case "ModelRequestRateLimitCount", "ModelRequestRateLimitSuccessCount":
		count, err := strconv.ParseInt(value, 10, 64)
		if err != nil || count < 0 || count > math.MaxInt64/(24*60*60) {
			return fmt.Errorf("请求限流计数超出有效范围")
		}
	case "ServerAddress":
		if value == "" {
			return nil
		}
		address, err := url.Parse(strings.TrimSpace(value))
		if err != nil || address.Host == "" || (address.Scheme != "http" && address.Scheme != "https") || address.User != nil || (address.Path != "" && address.Path != "/") || address.RawQuery != "" || address.Fragment != "" {
			return fmt.Errorf("站点地址须为完整的 HTTP 或 HTTPS Origin")
		}
	case "CreemProducts":
		if strings.TrimSpace(value) == "" {
			return nil
		}
		var products []struct {
			ProductID string  `json:"productId"`
			Price     float64 `json:"price"`
			Quota     float64 `json:"quota"`
			Currency  string  `json:"currency"`
		}
		if err := json.Unmarshal([]byte(value), &products); err != nil {
			return fmt.Errorf("支付产品须为有效 JSON 数组")
		}
		seen := map[string]bool{}
		for _, product := range products {
			if strings.TrimSpace(product.ProductID) == "" || seen[product.ProductID] || product.Price <= 0 || product.Quota <= 0 || math.IsNaN(product.Price) || math.IsInf(product.Price, 0) || math.IsNaN(product.Quota) || math.IsInf(product.Quota, 0) {
				return fmt.Errorf("支付产品的编号、价格或额度无效")
			}
			seen[product.ProductID] = true
		}
	}
	if settingType, ok := map[string]string{"console_setting.api_info": "ApiInfo", "console_setting.announcements": "Announcements", "console_setting.faq": "FAQ", "console_setting.uptime_kuma_groups": "UptimeKumaGroups"}[key]; ok {
		return console_setting.ValidateConsoleSettings(value, settingType)
	}
	return nil
}

func validateGatewayPricingExpressions(key, value string) error {
	if key != "billing_setting.billing_expr" && key != billing_setting.PluginBillingExprOption {
		return nil
	}
	var expressions map[string]string
	if err := json.Unmarshal([]byte(value), &expressions); err != nil || expressions == nil {
		return fmt.Errorf("计费表达式须为有效的 JSON 对象")
	}
	if key == billing_setting.PluginBillingExprOption {
		for _, entry := range sortedGatewayOptionKeys(expressions) {
			plugin, name, valid := billing_setting.SplitPluginBillingExprKey(entry)
			if !valid {
				return fmt.Errorf("计费插件表达式键名无效: %s", entry)
			}
			if err := ValidateModelPricing(name, PricingValues{key: map[string]any{plugin: expressions[entry]}}); err != nil {
				return err
			}
		}
		return nil
	}
	stored := billing_setting.GetPluginBillingExprCopy()
	for _, name := range sortedGatewayOptionKeys(expressions) {
		variants := map[string]any{}
		for entry, expression := range stored {
			if plugin, modelName, valid := billing_setting.SplitPluginBillingExprKey(entry); valid && name == modelName {
				variants[plugin] = expression
			}
		}
		if err := ValidateModelPricing(name, PricingValues{key: expressions[name], billing_setting.PluginBillingExprOption: variants}); err != nil {
			return fmt.Errorf("模型 %s 的计费表达式无效: %w", name, err)
		}
	}
	return nil
}
