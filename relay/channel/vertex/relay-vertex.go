package vertex

import (
	"encoding/json"
	"strings"
)

// Region lookup accepts only strings. A malformed per-model configuration
// selects the documented global fallback rather than panicking in a request.
func GetModelRegion(configuration, model string) string {
	trimmed := strings.TrimSpace(configuration)
	if !strings.HasPrefix(trimmed, "{") {
		return configuration
	}
	var regions map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &regions); err != nil {
		return "global"
	}
	for _, key := range []string{model, "default"} {
		if raw, ok := regions[key]; ok {
			var region string
			if json.Unmarshal(raw, &region) == nil && strings.TrimSpace(region) != "" {
				return region
			}
		}
	}
	return "global"
}
