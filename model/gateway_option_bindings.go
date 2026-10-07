package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/pkg/jsplugin"
	"github.com/dqt081228OHVGFY/further-api/setting"
	"github.com/dqt081228OHVGFY/further-api/setting/config"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/ratio_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
)

// The registry describes persisted keys and their runtime projection. Parsing
// produces a command without changing globals, collections or the option map.
type gatewayOptionBinding struct {
	read  func() string
	stage func(string) (string, func() error, error)
	after func() error
}

func gatewayScalarBinding(target any) gatewayOptionBinding {
	return gatewayOptionBinding{
		read: func() string {
			switch pointer := target.(type) {
			case *string:
				return *pointer
			case *bool:
				return strconv.FormatBool(*pointer)
			case *int:
				return strconv.Itoa(*pointer)
			case *float64:
				return strconv.FormatFloat(*pointer, 'f', -1, 64)
			}
			panic("unsupported gateway configuration binding")
		},
		stage: func(raw string) (string, func() error, error) {
			switch pointer := target.(type) {
			case *string:
				return raw, func() error { *pointer = raw; return nil }, nil
			case *bool:
				value, err := strconv.ParseBool(strings.TrimSpace(raw))
				if err != nil {
					return "", nil, errors.New("设置值须为 true 或 false")
				}
				return strconv.FormatBool(value), func() error { *pointer = value; return nil }, nil
			case *int:
				rational, valid := new(big.Rat).SetString(strings.TrimSpace(raw))
				if !valid || !rational.IsInt() || !rational.Num().IsInt64() {
					return "", nil, errors.New("设置值须为有效整数")
				}
				value := rational.Num().Int64()
				if value < 0 || value > int64(math.MaxInt) {
					return "", nil, errors.New("设置值须为范围内的非负整数")
				}
				return strconv.FormatInt(value, 10), func() error { *pointer = int(value); return nil }, nil
			case *float64:
				value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
				if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
					return "", nil, errors.New("设置值须为有效的非负数字")
				}
				return strconv.FormatFloat(value, 'f', -1, 64), func() error { *pointer = value; return nil }, nil
			}
			return "", nil, errors.New("设置字段类型无效")
		},
	}
}

func gatewayJSONBinding[T any](read func() string, apply func(string) error, validate func(T) error) gatewayOptionBinding {
	return gatewayOptionBinding{read: read, stage: func(raw string) (string, func() error, error) {
		normalized := strings.TrimSpace(raw)
		if normalized == "" || normalized == "null" {
			return "", nil, errors.New("设置值须为有效的 JSON 对象或数组")
		}
		var value T
		if err := json.Unmarshal([]byte(normalized), &value); err != nil {
			return "", nil, fmt.Errorf("设置 JSON 格式无效: %w", err)
		}
		if validate != nil {
			if err := validate(value); err != nil {
				return "", nil, err
			}
		}
		return raw, func() error {
			if apply != nil {
				return apply(raw)
			}
			return nil
		}, nil
	}}
}

func nonNegativeGatewayRatios(values map[string]float64) error {
	for name, value := range values {
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("%s 的倍率须为有效的非负数字", name)
		}
	}
	return nil
}

func gatewayOptionBindings() map[string]gatewayOptionBinding {
	bindings := make(map[string]gatewayOptionBinding)
	scalars := map[string]any{
		"FileUploadPermission": &common.FileUploadPermission, "FileDownloadPermission": &common.FileDownloadPermission, "ImageUploadPermission": &common.ImageUploadPermission, "ImageDownloadPermission": &common.ImageDownloadPermission,
		"PasswordLoginEnabled": &common.PasswordLoginEnabled, "PasswordRegisterEnabled": &common.PasswordRegisterEnabled, "EmailVerificationEnabled": &common.EmailVerificationEnabled,
		"GitHubOAuthEnabled": &common.GitHubOAuthEnabled, "LinuxDOOAuthEnabled": &common.LinuxDOOAuthEnabled, "TelegramOAuthEnabled": &common.TelegramOAuthEnabled, "WeChatAuthEnabled": &common.WeChatAuthEnabled, "TurnstileCheckEnabled": &common.TurnstileCheckEnabled, "RegisterEnabled": &common.RegisterEnabled,
		"AutomaticDisableChannelEnabled": &common.AutomaticDisableChannelEnabled, "AutomaticEnableChannelEnabled": &common.AutomaticEnableChannelEnabled, "LogConsumeEnabled": &common.LogConsumeEnabled,
		"DisplayInCurrencyEnabled": &common.DisplayInCurrencyEnabled, "DisplayTokenStatEnabled": &common.DisplayTokenStatEnabled, "DrawingEnabled": &common.DrawingEnabled, "TaskEnabled": &common.TaskEnabled, "TaskPluginEnabled": &constant.TaskPluginEnabled, "DataExportEnabled": &common.DataExportEnabled,
		"EmailDomainRestrictionEnabled": &common.EmailDomainRestrictionEnabled, "EmailAliasRestrictionEnabled": &common.EmailAliasRestrictionEnabled,
		"SMTPServer": &common.SMTPServer, "SMTPFrom": &common.SMTPFrom, "SMTPPort": &common.SMTPPort, "SMTPAccount": &common.SMTPAccount, "SMTPToken": &common.SMTPToken, "SMTPSSLEnabled": &common.SMTPSSLEnabled, "SMTPStartTLSEnabled": &common.SMTPStartTLSEnabled, "SMTPInsecureSkipVerify": &common.SMTPInsecureSkipVerify, "SMTPForceAuthLogin": &common.SMTPForceAuthLogin,
		"Footer": &common.Footer, "SystemName": &common.SystemName, "Logo": &common.Logo,
		"ServerAddress": &system_setting.ServerAddress, "TaskPublicAddress": &system_setting.TaskPublicAddress, "WorkerUrl": &system_setting.WorkerUrl, "WorkerValidKey": &system_setting.WorkerValidKey, "WorkerAllowHttpImageRequestEnabled": &system_setting.WorkerAllowHttpImageRequestEnabled,
		"PayAddress": &operation_setting.PayAddress, "CustomCallbackAddress": &operation_setting.CustomCallbackAddress, "EpayId": &operation_setting.EpayId, "EpayKey": &operation_setting.EpayKey, "Price": &operation_setting.Price, "USDExchangeRate": &operation_setting.USDExchangeRate, "MinTopUp": &operation_setting.MinTopUp,
		"StripeMinTopUp": &setting.StripeMinTopUp, "StripeApiSecret": &setting.StripeApiSecret, "StripeWebhookSecret": &setting.StripeWebhookSecret, "StripePriceId": &setting.StripePriceId, "StripeUnitPrice": &setting.StripeUnitPrice, "StripePromotionCodesEnabled": &setting.StripePromotionCodesEnabled,
		"CreemApiKey": &setting.CreemApiKey, "CreemProducts": &setting.CreemProducts, "CreemTestMode": &setting.CreemTestMode, "CreemWebhookSecret": &setting.CreemWebhookSecret,
		"WaffoEnabled": &setting.WaffoEnabled, "WaffoApiKey": &setting.WaffoApiKey, "WaffoPrivateKey": &setting.WaffoPrivateKey, "WaffoPublicCert": &setting.WaffoPublicCert, "WaffoSandboxPublicCert": &setting.WaffoSandboxPublicCert, "WaffoSandboxApiKey": &setting.WaffoSandboxApiKey, "WaffoSandboxPrivateKey": &setting.WaffoSandboxPrivateKey, "WaffoSandbox": &setting.WaffoSandbox, "WaffoMerchantId": &setting.WaffoMerchantId, "WaffoNotifyUrl": &setting.WaffoNotifyUrl, "WaffoReturnUrl": &setting.WaffoReturnUrl, "WaffoSubscriptionReturnUrl": &setting.WaffoSubscriptionReturnUrl, "WaffoCurrency": &setting.WaffoCurrency, "WaffoUnitPrice": &setting.WaffoUnitPrice, "WaffoMinTopUp": &setting.WaffoMinTopUp,
		"WaffoPancakeMerchantID": &setting.WaffoPancakeMerchantID, "WaffoPancakePrivateKey": &setting.WaffoPancakePrivateKey, "WaffoPancakeReturnURL": &setting.WaffoPancakeReturnURL, "WaffoPancakeUnitPrice": &setting.WaffoPancakeUnitPrice, "WaffoPancakeMinTopUp": &setting.WaffoPancakeMinTopUp, "WaffoPancakeStoreID": &setting.WaffoPancakeStoreID, "WaffoPancakeProductID": &setting.WaffoPancakeProductID,
		"DefaultUseAutoGroup": &setting.DefaultUseAutoGroup, "GitHubClientId": &common.GitHubClientId, "GitHubClientSecret": &common.GitHubClientSecret, "LinuxDOClientId": &common.LinuxDOClientId, "LinuxDOClientSecret": &common.LinuxDOClientSecret, "LinuxDOMinimumTrustLevel": &common.LinuxDOMinimumTrustLevel,
		"TelegramBotToken": &common.TelegramBotToken, "TelegramBotName": &common.TelegramBotName, "WeChatServerAddress": &common.WeChatServerAddress, "WeChatServerToken": &common.WeChatServerToken, "WeChatAccountQRCodeImageURL": &common.WeChatAccountQRCodeImageURL, "TurnstileSiteKey": &common.TurnstileSiteKey, "TurnstileSecretKey": &common.TurnstileSecretKey,
		"QuotaForNewUser": &common.QuotaForNewUser, "QuotaForInviter": &common.QuotaForInviter, "QuotaForInvitee": &common.QuotaForInvitee, "QuotaRemindThreshold": &common.QuotaRemindThreshold, "PreConsumedQuota": &common.PreConsumedQuota, "QuotaPerUnit": &common.QuotaPerUnit,
		"ModelRequestRateLimitCount": &setting.ModelRequestRateLimitCount, "ModelRequestRateLimitDurationMinutes": &setting.ModelRequestRateLimitDurationMinutes, "ModelRequestRateLimitSuccessCount": &setting.ModelRequestRateLimitSuccessCount, "ModelRequestRateLimitEnabled": &setting.ModelRequestRateLimitEnabled,
		"RetryTimes": &common.RetryTimes, "ChannelDisableThreshold": &common.ChannelDisableThreshold, "DataExportInterval": &common.DataExportInterval, "DataExportDefaultTime": &common.DataExportDefaultTime, "DefaultCollapseSidebar": &common.DefaultCollapseSidebar, "TopUpLink": &common.TopUpLink,
		"MjNotifyEnabled": &setting.MjNotifyEnabled, "MjAccountFilterEnabled": &setting.MjAccountFilterEnabled, "MjModeClearEnabled": &setting.MjModeClearEnabled, "MjForwardUrlEnabled": &setting.MjForwardUrlEnabled, "MjActionCheckSuccessEnabled": &setting.MjActionCheckSuccessEnabled,
		"CheckSensitiveEnabled": &setting.CheckSensitiveEnabled, "DemoSiteEnabled": &operation_setting.DemoSiteEnabled, "SelfUseModeEnabled": &operation_setting.SelfUseModeEnabled, "CheckSensitiveOnPromptEnabled": &setting.CheckSensitiveOnPromptEnabled, "StopOnSensitiveEnabled": &setting.StopOnSensitiveEnabled, "StreamCacheQueueLength": &setting.StreamCacheQueueLength,
	}
	for key, target := range scalars {
		bindings[key] = gatewayScalarBinding(target)
	}
	taskBinding := bindings["TaskPluginEnabled"]
	taskBinding.after = func() error { jsplugin.DefaultRegistry.SetEnabled(constant.TaskPluginEnabled); return nil }
	bindings["TaskPluginEnabled"] = taskBinding
	displayBinding := bindings["DisplayInCurrencyEnabled"]
	displayBinding.after = func() error {
		mode := "TOKENS"
		if common.DisplayInCurrencyEnabled {
			mode = "USD"
		}
		return config.GlobalConfig.Update("general_setting", map[string]string{"quota_display_type": mode})
	}
	bindings["DisplayInCurrencyEnabled"] = displayBinding
	bindings["ExposeRatioEnabled"] = gatewayOptionBinding{read: func() string { return strconv.FormatBool(ratio_setting.IsExposeRatioEnabled()) }, stage: func(raw string) (string, func() error, error) {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return "", nil, errors.New("价格展示设置须为布尔值")
		}
		return strconv.FormatBool(value), func() error { ratio_setting.SetExposeRatioEnabled(value); return nil }, nil
	}}
	bindings["EmailDomainWhitelist"] = gatewayOptionBinding{read: func() string { return strings.Join(common.EmailDomainWhitelist, ",") }, stage: func(raw string) (string, func() error, error) {
		domains := []string{}
		for _, domain := range strings.Split(raw, ",") {
			if domain = strings.TrimSpace(domain); domain != "" {
				domains = append(domains, domain)
			}
		}
		normalized := strings.Join(domains, ",")
		return normalized, func() error { common.EmailDomainWhitelist = domains; return nil }, nil
	}}
	bindings["SensitiveWords"] = gatewayOptionBinding{read: setting.SensitiveWordsToString, stage: func(raw string) (string, func() error, error) {
		return raw, func() error { setting.SensitiveWordsFromString(raw); return nil }, nil
	}}
	bindings["AutomaticDisableKeywords"] = gatewayOptionBinding{read: operation_setting.AutomaticDisableKeywordsToString, stage: func(raw string) (string, func() error, error) {
		return raw, func() error { operation_setting.AutomaticDisableKeywordsFromString(raw); return nil }, nil
	}}
	for key, projection := range map[string]struct {
		read  func() string
		apply func(string) error
	}{
		"AutomaticDisableStatusCodes": {operation_setting.AutomaticDisableStatusCodesToString, operation_setting.AutomaticDisableStatusCodesFromString},
		"AutomaticRetryStatusCodes":   {operation_setting.AutomaticRetryStatusCodesToString, operation_setting.AutomaticRetryStatusCodesFromString},
	} {
		bindings[key] = gatewayOptionBinding{read: projection.read, stage: func(raw string) (string, func() error, error) {
			if _, err := operation_setting.ParseHTTPStatusCodeRanges(raw); err != nil {
				return "", nil, err
			}
			return raw, func() error { return projection.apply(raw) }, nil
		}}
	}
	bindings["MaxTokenAutoGroups"] = gatewayOptionBinding{read: func() string { return strconv.Itoa(setting.GetMaxTokenAutoGroups()) }, stage: func(raw string) (string, func() error, error) {
		if err := setting.ValidateMaxTokenAutoGroups(raw); err != nil {
			return "", nil, err
		}
		return raw, func() error { return setting.UpdateMaxTokenAutoGroups(raw) }, nil
	}}
	bindings["AutoGroups"] = gatewayJSONBinding[[]string](setting.AutoGroups2JsonString, setting.UpdateAutoGroupsByJsonString, nil)
	bindings["UserUsableGroups"] = gatewayJSONBinding[map[string]string](setting.UserUsableGroups2JSONString, setting.UpdateUserUsableGroupsByJSONString, nil)
	bindings["PayMethods"] = gatewayJSONBinding[[]map[string]string](operation_setting.PayMethods2JsonString, operation_setting.UpdatePayMethodsByJsonString, nil)
	bindings["WaffoPayMethods"] = gatewayJSONBinding[[]constant.WaffoPayMethod](setting.WaffoPayMethods2JsonString, nil, nil)
	bindings["ModelRequestRateLimitGroup"] = gatewayJSONBinding[map[string][2]int](setting.ModelRequestRateLimitGroup2JSONString, func(raw string) error {
		var value map[string][2]int
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return err
		}
		setting.ModelRequestRateLimitMutex.Lock()
		setting.ModelRequestRateLimitGroup = value
		setting.ModelRequestRateLimitMutex.Unlock()
		return nil
	}, func(value map[string][2]int) error {
		raw, _ := json.Marshal(value)
		return setting.CheckModelRequestRateLimitGroup(string(raw))
	})
	bindings["TopupGroupRatio"] = gatewayJSONBinding[map[string]float64](common.TopupGroupRatio2JSONString, common.UpdateTopupGroupRatioByJSONString, nonNegativeGatewayRatios)
	bindings["GroupRatio"] = gatewayJSONBinding[map[string]float64](ratio_setting.GroupRatio2JSONString, ratio_setting.UpdateGroupRatioByJSONString, nonNegativeGatewayRatios)
	bindings["GroupGroupRatio"] = gatewayJSONBinding[map[string]map[string]float64](ratio_setting.GroupGroupRatio2JSONString, ratio_setting.UpdateGroupGroupRatioByJSONString, func(values map[string]map[string]float64) error {
		for _, value := range values {
			if err := nonNegativeGatewayRatios(value); err != nil {
				return err
			}
		}
		return nil
	})
	for key, projection := range map[string]struct {
		read  func() string
		apply func(string) error
	}{
		"ModelRatio": {ratio_setting.ModelRatio2JSONString, ratio_setting.UpdateModelRatioByJSONString}, "ModelPrice": {ratio_setting.ModelPrice2JSONString, ratio_setting.UpdateModelPriceByJSONString}, "CompletionRatio": {ratio_setting.CompletionRatio2JSONString, ratio_setting.UpdateCompletionRatioByJSONString}, "CacheRatio": {ratio_setting.CacheRatio2JSONString, ratio_setting.UpdateCacheRatioByJSONString}, "CreateCacheRatio": {ratio_setting.CreateCacheRatio2JSONString, ratio_setting.UpdateCreateCacheRatioByJSONString}, "ImageRatio": {ratio_setting.ImageRatio2JSONString, ratio_setting.UpdateImageRatioByJSONString}, "AudioRatio": {ratio_setting.AudioRatio2JSONString, ratio_setting.UpdateAudioRatioByJSONString}, "AudioCompletionRatio": {ratio_setting.AudioCompletionRatio2JSONString, ratio_setting.UpdateAudioCompletionRatioByJSONString},
	} {
		bindings[key] = gatewayJSONBinding[map[string]float64](projection.read, projection.apply, nonNegativeGatewayRatios)
	}
	bindings[setting.TaskPluginMarketplaceSourcesKey] = gatewayJSONBinding[[]setting.TaskPluginMarketplaceSource](setting.TaskPluginMarketplaceSources2JsonString, nil, nil)
	bindings[setting.TaskPluginDisabledFactoryKeysKey] = gatewayJSONBinding[[]string](func() string { return "[]" }, func(raw string) error {
		jsplugin.DefaultRegistry.SetDisabledFactoryKeys(setting.ParseTaskPluginDisabledFactoryKeys(raw))
		return nil
	}, nil)
	return bindings
}
