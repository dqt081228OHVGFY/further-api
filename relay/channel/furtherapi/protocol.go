package furtherapi

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/dqt081228OHVGFY/further-api/relay/channel/claude"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

type wireProtocol uint8

const (
	protocolOpenAI wireProtocol = iota
	protocolClaude
	protocolGemini
)

func protocolFor(info *relaycommon.RelayInfo) wireProtocol {
	if info == nil {
		return protocolOpenAI
	}
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		return protocolClaude
	case types.RelayFormatGemini:
		return protocolGemini
	default:
		return protocolOpenAI
	}
}

func upstreamURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil || !info.HasChannelMeta() {
		return "", errors.New("relay info is required")
	}
	base, err := url.Parse(info.ChannelBaseUrl)
	if err != nil || base == nil || base.Opaque != "" || base.Host == "" || base.Hostname() == "" || base.User != nil ||
		(base.Scheme != "http" && base.Scheme != "https") || base.RawQuery != "" || base.ForceQuery ||
		strings.Contains(info.ChannelBaseUrl, "#") || invalidURLCharacters(info.ChannelBaseUrl) ||
		strings.HasSuffix(base.Host, ":") || invalidGatewayPath(base.Path) {
		return "", errors.New("further-api base URL must be an absolute HTTP(S) URL without credentials, query or fragment")
	}
	if port := base.Port(); port != "" {
		number, portErr := strconv.Atoi(port)
		if portErr != nil || number < 1 || number > 65535 {
			return "", errors.New("further-api base URL has an invalid port")
		}
	}
	path := info.RequestURLPath
	if info.RelayMode == relayconstant.RelayModeAlphaSearch {
		path = "/v1/alpha/search"
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.Contains(path, "#") || invalidURLCharacters(path) {
		return "", errors.New("upstream request path must be relative to the configured gateway")
	}
	request, err := url.ParseRequestURI(path)
	if err != nil || request == nil || request.IsAbs() || request.Host != "" || request.Fragment != "" ||
		strings.HasPrefix(request.Path, "//") || invalidGatewayPath(request.Path) {
		return "", errors.New("invalid upstream request path")
	}
	// Join escaped paths without cleaning or re-escaping them: encoded slashes
	// can be part of a model/file ID and must not become route separators.
	base.RawPath = strings.TrimRight(base.EscapedPath(), "/") + request.EscapedPath()
	base.Path, err = url.PathUnescape(base.RawPath)
	if err != nil {
		return "", errors.New("invalid upstream request path")
	}
	base.RawQuery = request.RawQuery
	base.ForceQuery = request.ForceQuery
	base.Fragment = ""
	if info.RelayMode == relayconstant.RelayModeRealtime {
		if base.Scheme == "https" {
			base.Scheme = "wss"
		} else {
			base.Scheme = "ws"
		}
	}
	return base.String(), nil
}

func invalidURLCharacters(raw string) bool {
	return strings.Contains(raw, "\\") || strings.ContainsFunc(raw, func(character rune) bool {
		return unicode.IsSpace(character) || unicode.IsControl(character)
	})
}

func invalidGatewayPath(path string) bool {
	if strings.Contains(path, "\\") {
		return true
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "." || segment == ".." {
			return true
		}
	}
	return false
}

func setProtocolHeaders(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	// Every gateway route accepts its own bearer credential. Native protocol
	// routes also receive their standard key header for gateway compatibility.
	header.Set("Authorization", "Bearer "+info.ApiKey)
	switch protocolFor(info) {
	case protocolClaude:
		header.Set("x-api-key", info.ApiKey)
		version := c.Request.Header.Get("anthropic-version")
		if version == "" {
			version = "2023-06-01"
		}
		header.Set("anthropic-version", version)
		claude.CommonClaudeHeadersOperation(c, header, info)
	case protocolGemini:
		header.Set("x-goog-api-key", info.ApiKey)
	}
	return nil
}
