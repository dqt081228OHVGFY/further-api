package vertex

import (
	"net/url"
	"strings"
)

const (
	DefaultAPIVersion    = "v1"
	OpenSourceAPIVersion = "v1beta1"
	PublisherGoogle      = "google"
	PublisherAnthropic   = "anthropic"
)

func normalizeVertexBaseURL(base string) string {
	return strings.TrimRight(strings.TrimSpace(base), "/")
}
func normalizeVertexRegion(region string) string {
	region = strings.TrimSpace(region)
	if region == "" {
		return "global"
	}
	return region
}
func appendVertexAPIVersion(base, version string) string {
	version = strings.Trim(strings.TrimSpace(version), "/")
	if version == "" || strings.HasSuffix(base, "/"+version) {
		return base
	}
	return base + "/" + version
}

func appendResource(endpoint *url.URL, segments ...string) {
	plain, encoded := strings.TrimRight(endpoint.Path, "/"), strings.TrimRight(endpoint.EscapedPath(), "/")
	for _, segment := range segments {
		plain += "/" + segment
		encoded += "/" + url.PathEscape(segment)
	}
	endpoint.Path, endpoint.RawPath = plain, encoded
	if plain == encoded {
		endpoint.RawPath = ""
	}
}

func BuildAPIBaseURL(base, version, project, region string) string {
	region = normalizeVertexRegion(region)
	base = normalizeVertexBaseURL(base)
	if base == "" {
		host := "aiplatform.googleapis.com"
		if region != "global" {
			host = region + "-" + host
		}
		base = "https://" + host
	}
	endpoint, err := url.Parse(base)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return ""
	}
	version = strings.Trim(strings.TrimSpace(version), "/")
	if version != "" && !strings.HasSuffix(strings.TrimRight(endpoint.Path, "/"), "/"+version) {
		appendResource(endpoint, version)
	}
	if strings.TrimSpace(project) != "" {
		appendResource(endpoint, "projects", project, "locations", region)
	}
	return endpoint.String()
}

func BuildPublisherModelURL(base, version, project, region, publisher, model, action string) string {
	endpoint, err := url.Parse(BuildAPIBaseURL(base, version, project, region))
	if err != nil || endpoint.Host == "" {
		return ""
	}
	actionURL, err := url.Parse(action)
	if err != nil {
		return ""
	}
	appendResource(endpoint, "publishers", publisher, "models", model)
	escapedPath := endpoint.EscapedPath()
	endpoint.Path += ":" + actionURL.Path
	endpoint.RawPath = escapedPath + ":" + actionURL.EscapedPath()
	query := endpoint.Query()
	for key, values := range actionURL.Query() {
		query[key] = values
	}
	endpoint.RawQuery = query.Encode()
	return endpoint.String()
}
func BuildGoogleModelURL(base, version, project, region, model, action string) string {
	return BuildPublisherModelURL(base, version, project, region, PublisherGoogle, model, action)
}
func BuildAnthropicModelURL(base, version, project, region, model, action string) string {
	return BuildPublisherModelURL(base, version, project, region, PublisherAnthropic, model, action)
}
func BuildOpenSourceChatCompletionsURL(base, project, region string) string {
	endpoint, err := url.Parse(BuildAPIBaseURL(base, OpenSourceAPIVersion, project, region))
	if err != nil || endpoint.Host == "" {
		return ""
	}
	appendResource(endpoint, "endpoints", "openapi", "chat", "completions")
	return endpoint.String()
}
