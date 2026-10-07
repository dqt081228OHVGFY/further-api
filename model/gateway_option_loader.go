package model

import (
	"context"
	"maps"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/pkg/jsplugin"
	"github.com/dqt081228OHVGFY/further-api/setting/config"
)

func InitOptionMap() {
	defaults := map[string]string{"Notice": "", "About": "", "HomePageContent": ""}
	for key, binding := range gatewayOptionBindings() {
		defaults[key] = binding.read()
	}
	maps.Copy(defaults, config.GlobalConfig.ExportAllConfigs())
	for _, key := range []string{"SystemName", "Logo", "passkey.rp_display_name"} {
		defaults[key], _ = common.CanonicalBrandingValue(key)
	}
	common.OptionMapRWMutex.Lock()
	common.SystemName, common.Logo = common.ProductName, common.ProductLogo
	common.OptionMap = defaults
	common.OptionMapRWMutex.Unlock()
	jsplugin.DefaultRegistry.SetEnabled(constant.TaskPluginEnabled)
	jsplugin.DefaultRegistry.SetDisabledFactoryKeys(nil)
	loadOptionsFromDatabase()
}

func ReloadGatewayOptionsContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	gatewayOptionMutationMu.Lock()
	defer gatewayOptionMutationMu.Unlock()
	requestPolicyOptionMutex.Lock()
	defer requestPolicyOptionMutex.Unlock()
	passkeyOptionMutex.Lock()
	defer passkeyOptionMutex.Unlock()
	modelPricingMutationMu.Lock()
	defer modelPricingMutationMu.Unlock()
	// Read only after outstanding writes have committed and published. A
	// snapshot taken before these locks can overwrite a newer runtime value.
	rows, err := AllOptionsContext(ctx)
	if err != nil {
		return err
	}
	values := map[string]string{}
	for _, row := range rows {
		values[row.Key] = row.Value
	}
	plan, err := prepareGatewayOptionPlan(values, true)
	if err != nil {
		return err
	}
	options := maps.Clone(CurrentRequestPolicy().Options)
	for key, value := range plan.values {
		if IsRequestPolicyOption(key) {
			options[key] = value
		}
	}
	policy, err := BuildRequestPolicy(options)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := publishGatewayOptionPlan(plan); err != nil {
		return err
	}
	requestPolicySnapshot.Store(policy)
	return nil
}

func loadOptionsFromDatabase() { loadOptionsFromDatabaseContext(context.Background()) }
func loadOptionsFromDatabaseContext(ctx context.Context) {
	if err := ReloadGatewayOptionsContext(ctx); err != nil && ctx.Err() == nil {
		common.SysError("读取网关配置失败: " + err.Error())
	}
}
func SyncOptions(frequency int) {
	if frequency <= 0 {
		return
	}
	ctx := common.BackgroundContext()
	ticker := time.NewTicker(time.Duration(frequency) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			loadOptionsFromDatabaseContext(ctx)
		}
	}
}
