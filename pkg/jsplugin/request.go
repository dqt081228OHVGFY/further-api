package jsplugin

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// ValidateRequestURL restricts a request that may carry channel credentials to
// the configured endpoint or an explicitly approved host.
func ValidateRequestURL(requestURL, baseURL string, allowedHosts []string) error {
	request, err := parsePluginHTTPURL(requestURL)
	if err != nil {
		return err
	}
	base, err := parsePluginHTTPURL(baseURL)
	if err != nil || base.RawQuery != "" || base.ForceQuery {
		return fmt.Errorf("channel base URL is invalid")
	}
	if base.Scheme == "https" && request.Scheme != "https" {
		return fmt.Errorf("plugin request URL must use HTTPS when the channel base URL uses HTTPS")
	}
	requestHost := canonicalHost(request)
	if requestHost == canonicalHost(base) {
		return nil
	}
	for _, allowed := range allowedHosts {
		// Manifests contain host[:port] entries without a scheme. Revalidate
		// here because callers can also construct Meta values directly.
		normalized, hostErr := normalizeAllowedHost(allowed)
		if hostErr != nil {
			continue
		}
		allowedURL, parseErr := parsePluginHTTPURL(request.Scheme + "://" + normalized)
		if parseErr == nil && requestHost == canonicalHost(allowedURL) {
			return nil
		}
	}
	return fmt.Errorf("plugin request host %q is not allowed", request.Host)
}

// ValidateCredentiallessURL checks an artifact URL without imposing the
// channel's host allowlist. Signed CDN URLs commonly use a separate host.
func ValidateCredentiallessURL(requestURL string) error {
	_, err := parsePluginHTTPURL(requestURL)
	return err
}

func parsePluginHTTPURL(raw string) (*url.URL, error) {
	for _, character := range raw {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return nil, fmt.Errorf("plugin request URL must not contain whitespace or control characters")
		}
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || parsed.Scheme == "" || parsed.Host == "" || parsed.Hostname() == "" {
		return nil, fmt.Errorf("plugin request URL must be absolute")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("plugin request URL must use the http or https scheme")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("plugin request URL must not contain credentials")
	}
	if strings.Contains(raw, "#") {
		return nil, fmt.Errorf("plugin request URL must not contain a fragment")
	}
	host := parsed.Hostname()
	for _, character := range host {
		if character > unicode.MaxASCII || unicode.IsSpace(character) || unicode.IsControl(character) {
			return nil, fmt.Errorf("plugin request URL host must be ASCII")
		}
	}
	if strings.HasSuffix(parsed.Host, ":") {
		return nil, fmt.Errorf("plugin request URL has an invalid port")
	}
	if port := parsed.Port(); port != "" {
		number, parseErr := strconv.Atoi(port)
		if parseErr != nil || number < 1 || number > 65535 {
			return nil, fmt.Errorf("plugin request URL has an invalid port")
		}
	}
	return parsed, nil
}

func canonicalHost(value *url.URL) string {
	host := strings.ToLower(value.Hostname())
	port := value.Port()
	if port == "" {
		return host
	}
	number, err := strconv.Atoi(port)
	if err != nil {
		return ""
	}
	if number == 80 && value.Scheme == "http" || number == 443 && value.Scheme == "https" {
		return host
	}
	return net.JoinHostPort(host, strconv.Itoa(number))
}
