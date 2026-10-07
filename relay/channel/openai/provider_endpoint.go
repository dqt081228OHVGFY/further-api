package openai

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
)

// ProviderEndpoint joins a provider base and a protocol route as URL values.
// Versioned proxy bases retain their prefix without adding the same version
// twice. Encoded base segments and configured query parameters remain intact.
func ProviderEndpoint(base, route string) (string, error) {
	endpoint, err := url.Parse(strings.TrimSpace(base))
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https" && endpoint.Scheme != "ws" && endpoint.Scheme != "wss") {
		return "", errors.New("provider endpoint must be an absolute HTTP or WebSocket URL")
	}
	operation, err := url.Parse(route)
	if err != nil || operation.IsAbs() || operation.Host != "" {
		return "", errors.New("provider route must be relative to its endpoint")
	}
	prefix, escaped := strings.TrimRight(endpoint.Path, "/"), strings.TrimRight(endpoint.EscapedPath(), "/")
	path, encoded := strings.TrimLeft(operation.Path, "/"), strings.TrimLeft(operation.EscapedPath(), "/")
	if version, rest, found := strings.Cut(path, "/"); found && (version == "v1" || version == "v2" || version == "v1beta" || version == "v1beta1") && strings.HasSuffix(prefix, "/"+version) {
		path = rest
		encoded = strings.TrimPrefix(encoded, version+"/")
	}
	endpoint.Path, endpoint.RawPath = prefix+"/"+path, escaped+"/"+encoded
	if endpoint.Path == endpoint.RawPath {
		endpoint.RawPath = ""
	}
	query := endpoint.Query()
	for key, values := range operation.Query() {
		query[key] = values
	}
	endpoint.RawQuery = query.Encode()
	endpoint.Fragment = ""
	return endpoint.String(), nil
}

func UnsupportedOperation(provider, operation string) error {
	return types.NewErrorWithStatusCode(errors.New(provider+" does not support "+operation), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}
