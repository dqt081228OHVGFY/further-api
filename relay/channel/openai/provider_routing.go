package openai

import (
	"errors"
	"net/url"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/ai360"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/lingyiwanwu"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openrouter"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/xinference"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
)

// ProviderDefinition describes protocol behavior rather than delegating to an
// adaptor of another channel. Attempt state stays on the calling Adaptor.
type ProviderDefinition struct {
	Name   string
	Models []string
	Azure  bool
	Custom bool
}

func ProviderFor(channelType int) ProviderDefinition {
	switch channelType {
	case constant.ChannelTypeAzure:
		return ProviderDefinition{Name: ChannelName, Models: ModelList, Azure: true}
	case constant.ChannelTypeCustom:
		return ProviderDefinition{Name: ChannelName, Models: ModelList, Custom: true}
	case constant.ChannelType360:
		return ProviderDefinition{Name: ai360.ChannelName, Models: ai360.ModelList}
	case constant.ChannelTypeLingYiWanWu:
		return ProviderDefinition{Name: lingyiwanwu.ChannelName, Models: lingyiwanwu.ModelList}
	case constant.ChannelTypeXinference:
		return ProviderDefinition{Name: xinference.ChannelName, Models: xinference.ModelList}
	case constant.ChannelTypeOpenRouter:
		return ProviderDefinition{Name: openrouter.ChannelName, Models: openrouter.ModelList}
	default:
		return ProviderDefinition{Name: ChannelName, Models: ModelList}
	}
}

// RequestEndpoint keeps the configured base immutable, including during a
// Realtime retry. Azure deployment names and versions are encoded as values,
// never interpreted as additional URL path or query parameters.
func RequestEndpoint(info *relaycommon.RelayInfo) (string, error) {
	if info == nil || !info.HasChannelMeta() {
		return "", errors.New("OpenAI provider configuration is missing")
	}
	profile := ProviderFor(info.ChannelType)
	if profile.Custom {
		return strings.ReplaceAll(info.ChannelBaseUrl, "{model}", info.UpstreamModelName), nil
	}
	base, err := url.Parse(info.ChannelBaseUrl)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https" && base.Scheme != "ws" && base.Scheme != "wss") {
		return "", errors.New("OpenAI provider requires an absolute HTTP or WebSocket endpoint")
	}
	if info.RelayMode == relayconstant.RelayModeRealtime {
		switch base.Scheme {
		case "https":
			base.Scheme = "wss"
		case "http":
			base.Scheme = "ws"
		}
	}
	request, err := url.Parse(info.RequestURLPath)
	if err != nil || request.IsAbs() || request.Host != "" {
		return "", errors.New("OpenAI request path must be relative to its provider")
	}
	if profile.Azure {
		request = azureEndpoint(info, request)
	} else if (info.RelayFormat == types.RelayFormatClaude || info.RelayFormat == types.RelayFormatGemini) &&
		info.RelayMode != relayconstant.RelayModeResponses && info.RelayMode != relayconstant.RelayModeResponsesCompact {
		request = &url.URL{Path: "/v1/chat/completions"}
	}
	if strings.EqualFold(base.Hostname(), "gateway.ai.cloudflare.com") {
		prefix := "/v1"
		if profile.Azure {
			prefix = "/openai/deployments"
		}
		request.Path = strings.TrimPrefix(request.Path, prefix)
		request.RawPath = strings.TrimPrefix(request.RawPath, prefix)
	}
	escapedPrefix := strings.TrimRight(base.EscapedPath(), "/")
	base.Path = strings.TrimRight(base.Path, "/") + "/" + strings.TrimLeft(request.Path, "/")
	base.RawPath = escapedPrefix + "/" + strings.TrimLeft(request.EscapedPath(), "/")
	if base.RawPath == base.Path {
		base.RawPath = ""
	}
	query := base.Query()
	for key, values := range request.Query() {
		query[key] = values
	}
	base.RawQuery = query.Encode()
	base.Fragment = ""
	return base.String(), nil
}

func azureEndpoint(info *relaycommon.RelayInfo, inbound *url.URL) *url.URL {
	version := info.ApiVersion
	if version == "" {
		version = constant.AzureDefaultAPIVersion
	}
	if info.RelayMode == relayconstant.RelayModeResponses || info.RelayMode == relayconstant.RelayModeResponsesCompact {
		path, responsesVersion := "/openai/v1/responses", "preview"
		if strings.Contains(info.ChannelBaseUrl, "cognitiveservices.azure.com") {
			path, responsesVersion = "/openai/responses", version
		}
		if configured := info.ChannelOtherSettings.AzureResponsesVersion; configured != "" {
			responsesVersion = configured
		}
		if info.RelayMode == relayconstant.RelayModeResponsesCompact {
			path += "/compact"
		}
		return &url.URL{Path: path, RawQuery: url.Values{"api-version": {responsesVersion}}.Encode()}
	}
	model := info.UpstreamModelName
	if info.ChannelCreateTime < constant.AzureNoRemoveDotTime {
		model = strings.ReplaceAll(model, ".", "")
	}
	if info.RelayMode == relayconstant.RelayModeRealtime {
		return &url.URL{Path: "/openai/realtime", RawQuery: url.Values{"api-version": {version}, "deployment": {model}}.Encode()}
	}
	task := strings.TrimPrefix(inbound.Path, "/v1/")
	if info.RelayFormat == types.RelayFormatGemini {
		task = "chat/completions"
	} else if info.RelayFormat == types.RelayFormatClaude {
		task = "chat/completions" + strings.TrimPrefix(task, "messages")
	}
	path := "/openai/deployments/" + model + "/" + task
	return &url.URL{Path: path, RawPath: "/openai/deployments/" + url.PathEscape(model) + "/" + task, RawQuery: url.Values{"api-version": {version}}.Encode()}
}
