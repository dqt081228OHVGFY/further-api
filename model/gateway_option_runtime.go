package model

import (
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/setting/config"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/performance_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/ratio_setting"
)

type gatewayOptionPlan struct {
	values      map[string]string
	projections []func() error
	modules     map[string]map[string]string
}

func sortedGatewayOptionKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func prepareGatewayOptionPlan(values map[string]string, stored bool) (*gatewayOptionPlan, error) {
	plan := &gatewayOptionPlan{values: maps.Clone(values), modules: map[string]map[string]string{}}
	bindings := gatewayOptionBindings()
	for _, key := range sortedGatewayOptionKeys(values) {
		raw := values[key]
		if stored {
			if canonical, fixed := common.CanonicalBrandingValue(key); fixed {
				raw = canonical
				plan.values[key] = raw
			}
		}
		if key == retiredThemeOptionKey || key == retiredChatOptionKey {
			if !stored && key == retiredChatOptionKey {
				return nil, ErrRetiredChatOption
			}
			if !stored && key == retiredThemeOptionKey && raw != "default" {
				return nil, fmt.Errorf("站点主题请通过外观设置管理")
			}
			continue
		}
		if err := validateGatewayOptionValue(key, raw); err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		if !stored {
			if err := validateGatewayPricingExpressions(key, raw); err != nil {
				return nil, err
			}
		}
		if binding, ok := bindings[key]; ok {
			normalized, apply, err := binding.stage(raw)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			plan.values[key] = normalized
			plan.projections = append(plan.projections, apply)
			if binding.after != nil {
				plan.projections = append(plan.projections, binding.after)
			}
			continue
		}
		if key == operation_setting.ToolPriceOptionKey {
			plan.projections = append(plan.projections, func() error { operation_setting.LoadToolPricesFromJSONString(raw); return nil })
			continue
		}
		module, field, hasModule := strings.Cut(key, ".")
		if !hasModule || config.GlobalConfig.Get(module) == nil {
			continue
		}
		current, err := config.ConfigToMap(config.GlobalConfig.Get(module))
		if err != nil {
			return nil, err
		}
		if _, exists := current[field]; !exists {
			return nil, fmt.Errorf("设置字段不存在: %s", key)
		}
		if plan.modules[module] == nil {
			plan.modules[module] = map[string]string{}
		}
		plan.modules[module][field] = raw
	}
	for module, fields := range plan.modules {
		if err := config.GlobalConfig.ValidateUpdate(module, fields); err != nil {
			return nil, fmt.Errorf("%s: %w", module, err)
		}
	}
	return plan, nil
}

// All parsing happens before persistence. Publication keeps the option read
// model and its runtime projections under the same write lock.
func publishGatewayOptionPlan(plan *gatewayOptionPlan) error {
	if plan == nil {
		return nil
	}
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	flat := map[string]string{}
	for module, fields := range plan.modules {
		for field, value := range fields {
			flat[module+"."+field] = value
		}
	}
	if len(flat) > 0 {
		if err := config.GlobalConfig.LoadFromDB(flat); err != nil {
			return err
		}
	}
	for _, apply := range plan.projections {
		if err := apply(); err != nil {
			return err
		}
	}
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	for key, value := range plan.values {
		if key == retiredThemeOptionKey || key == retiredChatOptionKey {
			delete(common.OptionMap, key)
		} else {
			common.OptionMap[key] = value
		}
	}
	if _, changed := plan.modules["performance_setting"]; changed {
		performance_setting.UpdateAndSync()
	}
	if _, changed := plan.modules["billing_setting"]; changed {
		InvalidatePricingCache()
		ratio_setting.InvalidateExposedDataCache()
	}
	return nil
}

func updateOptionMap(key, value string) error {
	plan, err := prepareGatewayOptionPlan(map[string]string{key: value}, true)
	if err != nil {
		return err
	}
	return publishGatewayOptionPlan(plan)
}

// Called only with OptionMapRWMutex held by Passkey domain publication.
func handleConfigUpdate(key, value string) bool {
	if key == operation_setting.ToolPriceOptionKey {
		operation_setting.LoadToolPricesFromJSONString(value)
		return true
	}
	module, field, ok := strings.Cut(key, ".")
	if !ok || config.GlobalConfig.Get(module) == nil {
		return false
	}
	if err := config.GlobalConfig.Update(module, map[string]string{field: value}); err != nil {
		common.SysError("publish gateway setting " + key + ": " + err.Error())
	}
	return true
}

func validateOptionValue(key, value string) error {
	_, err := prepareGatewayOptionPlan(map[string]string{key: value}, false)
	return err
}

func ValidateGatewayOptionValues(values map[string]string) error {
	_, err := prepareGatewayOptionPlan(values, false)
	return err
}
