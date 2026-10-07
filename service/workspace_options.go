package service

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/setting/billing_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/ratio_setting"
)

// WorkspaceOption is a presentation DTO, independent of persisted option rows.
type WorkspaceOption struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

var completionRatioSources = []string{
	"ModelPrice", "ModelRatio", "CompletionRatio", "CacheRatio", "CreateCacheRatio",
	"ImageRatio", "AudioRatio", "AudioCompletionRatio",
}

// IsPrivateWorkspaceOption applies to credentials, never to the contents of an
// arbitrary option. A configured indicator is added explicitly by the reader.
func IsPrivateWorkspaceOption(key string) bool {
	canonical := strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "").Replace(key))
	for _, suffix := range []string{"token", "secret", "key", "password", "credential", "privatekeypem"} {
		if strings.HasSuffix(canonical, suffix) {
			return true
		}
	}
	return false
}

func workspaceOptionSnapshot() (map[string]string, bool) {
	common.OptionMapRWMutex.RLock()
	values := make(map[string]string, len(common.OptionMap))
	credentials := make(map[string]string)
	for key, value := range common.OptionMap {
		if key == "theme.frontend" || key == "Chats" {
			continue
		}
		if IsPrivateWorkspaceOption(key) {
			credentials[key+"Configured"] = strconv.FormatBool(value != "")
			continue
		}
		values[key] = value
	}
	for key, configured := range credentials {
		values[key] = configured
	}
	configured := common.LinuxDOClientSecret != ""
	common.OptionMapRWMutex.RUnlock()
	return values, configured
}

// ReadWorkspaceOptions holds a shared lock only while copying primitives.
// Derived billing configuration and JSON encoding happen after releasing it;
// callers receive their own sorted result and cannot mutate running settings.
func ReadWorkspaceOptions() ([]WorkspaceOption, error) {
	values, configured := workspaceOptionSnapshot()
	values["LinuxDOClientSecretConfigured"] = strconv.FormatBool(configured)
	for key, expressions := range map[string]map[string]string{
		"billing_setting.billing_mode": billing_setting.GetBillingModeCopy(),
		"billing_setting.billing_expr": billing_setting.GetBillingExprCopy(),
	} {
		encoded, err := json.Marshal(expressions)
		if err != nil {
			return nil, err
		}
		values[key] = string(encoded)
	}
	values["CompletionRatioMeta"] = completionRatioMetadata(values)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]WorkspaceOption, 0, len(keys))
	for _, key := range keys {
		result = append(result, WorkspaceOption{Key: key, Value: values[key]})
	}
	return result, nil
}

func completionRatioMetadata(values map[string]string) string {
	names := make(map[string]struct{})
	for _, key := range completionRatioSources {
		var entries map[string]json.RawMessage
		if json.Unmarshal([]byte(values[key]), &entries) != nil {
			continue
		}
		for name := range entries {
			names[name] = struct{}{}
		}
	}
	metadata := make(map[string]ratio_setting.CompletionRatioInfo, len(names))
	for name := range names {
		metadata[name] = ratio_setting.GetCompletionRatioInfo(name)
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
