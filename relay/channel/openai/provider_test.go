package openai

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProviderEndpointRealtimeDoesNotChangeConfiguredBase(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, ChannelBaseUrl: "https://upstream.example/proxy", UpstreamModelName: "gpt-realtime"}, RequestURLPath: "/v1/realtime?model=gpt-realtime", RelayMode: relayconstant.RelayModeRealtime}
	first, err := RequestEndpoint(info)
	require.NoError(t, err)
	require.Equal(t, "wss://upstream.example/proxy/v1/realtime?model=gpt-realtime", first)
	require.Equal(t, "https://upstream.example/proxy", info.ChannelBaseUrl)
	second, err := RequestEndpoint(info)
	require.NoError(t, err)
	require.Equal(t, first, second)
}

func TestAzureEndpointTreatsDeploymentAndVersionAsValues(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeAzure, ChannelBaseUrl: "https://azure.example/proxy", UpstreamModelName: "deployment/name?extra", ApiVersion: "2026&injected=true", ChannelCreateTime: constant.AzureNoRemoveDotTime}, RequestURLPath: "/v1/chat/completions"}
	endpoint, err := RequestEndpoint(info)
	require.NoError(t, err)
	parsed, err := url.Parse(endpoint)
	require.NoError(t, err)
	require.Equal(t, "2026&injected=true", parsed.Query().Get("api-version"))
	require.Empty(t, parsed.Query().Get("injected"))
	require.Contains(t, parsed.EscapedPath(), "deployment%2Fname%3Fextra/chat/completions")
}

func TestCloudflareProxyRetainsItsConfiguredPrefix(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, ChannelBaseUrl: "https://gateway.ai.cloudflare.com/account/gateway/openai"}, RequestURLPath: "/v1/responses", RelayMode: relayconstant.RelayModeResponses}
	endpoint, err := RequestEndpoint(info)
	require.NoError(t, err)
	require.Equal(t, "https://gateway.ai.cloudflare.com/account/gateway/openai/responses", endpoint)
}

func TestRealtimeAuthenticationRetainsOnlyPreviewBeta(t *testing.T) {
	for _, model := range []string{"gpt-realtime", "gpt-4o-realtime-preview"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, ApiKey: "provider-key", UpstreamModelName: model}, RelayMode: relayconstant.RelayModeRealtime}
		headers := http.Header{}
		require.NoError(t, SetupProviderHeaders(c, &headers, info))
		require.Equal(t, "Bearer provider-key", headers.Get("Authorization"))
		if model == "gpt-realtime" {
			require.Empty(t, headers.Get("OpenAI-Beta"))
		} else {
			require.Equal(t, "realtime=v1", headers.Get("OpenAI-Beta"))
		}
	}
}

func TestProviderAuthenticationDoesNotReplaceExplicitAuthorization(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, ApiKey: "provider-key", HeadersOverride: map[string]any{"authorization": "custom"}}}
	headers := http.Header{}
	require.NoError(t, SetupProviderHeaders(c, &headers, info))
	require.Empty(t, headers.Get("Authorization"))
}
