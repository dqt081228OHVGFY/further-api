package channel

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"

	hostcommon "github.com/dqt081228OHVGFY/further-api/common"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/http/httpguts"
)

const clientHeaderPlaceholderPrefix = "{client_header:"
const headerPassthroughAllKey = "*"
const headerPassthroughRegexPrefix = "re:"
const headerPassthroughRegexPrefixV2 = "regex:"

var passthroughRegexes = struct {
	sync.Mutex
	items map[string]*regexp.Regexp
}{items: make(map[string]*regexp.Regexp)}

func SetupApiRequestHeader(info *relaycommon.RelayInfo, c *gin.Context, header *http.Header) {
	if info == nil || c == nil || c.Request == nil || header == nil {
		return
	}
	if *header == nil {
		*header = make(http.Header)
	}
	switch info.RelayMode {
	case constant.RelayModeAudioTranscription, constant.RelayModeAudioTranslation, constant.RelayModeRealtime:
		return
	}
	if value := c.Request.Header.Get("Content-Type"); value != "" {
		header.Set("Content-Type", value)
	}
	accept := c.Request.Header.Get("Accept")
	if info.IsStream && accept == "" {
		accept = "text/event-stream"
	}
	if accept != "" {
		header.Set("Accept", accept)
	}
}

func getHeaderPassthroughRegex(pattern string) (*regexp.Regexp, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, errors.New("header passthrough pattern is empty")
	}
	passthroughRegexes.Lock()
	defer passthroughRegexes.Unlock()
	if compiled := passthroughRegexes.items[pattern]; compiled != nil {
		return compiled, nil
	}
	// HTTP field names are case-insensitive; the expression itself retains its
	// case so escapes and character classes are never rewritten accidentally.
	compiled, err := regexp.Compile("(?i:" + pattern + ")")
	if err != nil {
		return nil, err
	}
	if len(passthroughRegexes.items) >= 128 {
		clear(passthroughRegexes.items)
	}
	passthroughRegexes.items[pattern] = compiled
	return compiled, nil
}

func IsHeaderPassthroughRuleKey(key string) bool { return isHeaderPassthroughRuleKey(key) }

func isHeaderPassthroughRuleKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return key == headerPassthroughAllKey || strings.HasPrefix(key, headerPassthroughRegexPrefix) || strings.HasPrefix(key, headerPassthroughRegexPrefixV2)
}

func shouldSkipPassthroughHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade", "cookie", "host", "content-length", "accept-encoding", "authorization", "x-api-key", "x-goog-api-key", "sec-websocket-key", "sec-websocket-version", "sec-websocket-extensions":
		return true
	}
	return strings.EqualFold(name, hostcommon.RequestIdKey)
}

func applyHeaderOverridePlaceholders(template string, c *gin.Context, apiKey string) (string, bool, error) {
	trimmed := strings.TrimSpace(template)
	if !strings.HasPrefix(trimmed, clientHeaderPlaceholderPrefix) {
		value := strings.ReplaceAll(template, "{api_key}", apiKey)
		return value, strings.TrimSpace(value) != "", nil
	}
	if !strings.HasSuffix(trimmed, "}") {
		return "", false, errors.New("client header placeholder must fill the whole value")
	}
	name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, clientHeaderPlaceholderPrefix), "}"))
	if !httpguts.ValidHeaderFieldName(name) {
		return "", false, errors.New("client header placeholder contains an invalid field name")
	}
	if c == nil || c.Request == nil {
		return "", false, errors.New("client header placeholder requires an inbound request")
	}
	// Client values are data; an API key placeholder inside them is never expanded.
	value := c.Request.Header.Get(name)
	return value, strings.TrimSpace(value) != "", nil
}

func invalidHeaderPolicy(err error) error {
	return types.NewError(err, types.ErrorCodeChannelHeaderOverrideInvalid, types.ErrOptionWithSkipRetry())
}

// Compile routing rules before collecting values. Explicit field overrides
// are applied in a deterministic second phase and always win over passthrough.
func processHeaderOverride(info *relaycommon.RelayInfo, c *gin.Context) (map[string]string, error) {
	result := make(map[string]string)
	if info == nil {
		return result, nil
	}
	source := relaycommon.GetEffectiveHeaderOverride(info)
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var patterns []*regexp.Regexp
	all := false
	if !info.IsChannelTest {
		for _, raw := range keys {
			key := strings.TrimSpace(raw)
			lower := strings.ToLower(key)
			if key == "*" {
				all = true
				continue
			}
			var pattern string
			switch {
			case strings.HasPrefix(lower, headerPassthroughRegexPrefix):
				pattern = key[len(headerPassthroughRegexPrefix):]
			case strings.HasPrefix(lower, headerPassthroughRegexPrefixV2):
				pattern = key[len(headerPassthroughRegexPrefixV2):]
			default:
				continue
			}
			compiled, err := getHeaderPassthroughRegex(pattern)
			if err != nil {
				return nil, invalidHeaderPolicy(err)
			}
			patterns = append(patterns, compiled)
		}
	}
	if all || len(patterns) > 0 {
		if c == nil || c.Request == nil {
			return nil, invalidHeaderPolicy(errors.New("header passthrough requires an inbound request"))
		}
		connectionFields := make(map[string]bool)
		for _, entry := range c.Request.Header.Values("Connection") {
			for _, field := range strings.Split(entry, ",") {
				connectionFields[strings.ToLower(strings.TrimSpace(field))] = true
			}
		}
		for name := range c.Request.Header {
			key := strings.ToLower(name)
			if shouldSkipPassthroughHeader(name) || connectionFields[key] || !httpguts.ValidHeaderFieldName(name) {
				continue
			}
			matched := all
			for _, pattern := range patterns {
				matched = matched || pattern.MatchString(name)
			}
			value := c.Request.Header.Get(name)
			if matched && strings.TrimSpace(value) != "" && httpguts.ValidHeaderFieldValue(value) {
				result[key] = value
			}
		}
	}
	for _, raw := range keys {
		if isHeaderPassthroughRuleKey(raw) {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(raw))
		if key == "" {
			continue
		}
		if !httpguts.ValidHeaderFieldName(key) {
			return nil, invalidHeaderPolicy(fmt.Errorf("invalid upstream header name %q", raw))
		}
		template, ok := source[raw].(string)
		if !ok {
			return nil, invalidHeaderPolicy(fmt.Errorf("upstream header %q must contain a string", raw))
		}
		if info.IsChannelTest && strings.HasPrefix(strings.TrimSpace(template), clientHeaderPlaceholderPrefix) {
			continue
		}
		value, include, err := applyHeaderOverridePlaceholders(template, c, info.ApiKey)
		if err != nil {
			return nil, invalidHeaderPolicy(err)
		}
		if include {
			if !httpguts.ValidHeaderFieldValue(value) {
				return nil, invalidHeaderPolicy(fmt.Errorf("upstream header %q has an invalid value", raw))
			}
			result[key] = value
		}
	}
	return result, nil
}

func ResolveHeaderOverride(info *relaycommon.RelayInfo, c *gin.Context) (map[string]string, error) {
	return processHeaderOverride(info, c)
}

func applyHeaderOverrideToRequest(req *http.Request, overrides map[string]string) {
	if req == nil {
		return
	}
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	for name, value := range overrides {
		req.Header.Set(name, value)
		if strings.EqualFold(name, "Host") {
			req.Host = value
		}
	}
}
