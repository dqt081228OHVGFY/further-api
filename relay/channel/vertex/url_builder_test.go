package vertex

import (
	"net/url"
	"testing"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestVertexModelEndpointKeepsResourceSegmentsEncoded(t *testing.T) {
	endpoint := BuildGoogleModelURL("https://proxy.example/api/v1?existing=kept", "v1", "team/a", "us-central1", "model/a b", "streamGenerateContent?alt=sse")
	require.Equal(t, "https://proxy.example/api/v1/projects/team%2Fa/locations/us-central1/publishers/google/models/model%2Fa%20b:streamGenerateContent?alt=sse&existing=kept", endpoint)
	require.Equal(t, "https://aiplatform.googleapis.com/v1/publishers/google/models/gemini:generateContent", BuildGoogleModelURL("", "v1", "", "", "gemini", "generateContent"))
	require.Equal(t, "https://us-central1-aiplatform.googleapis.com/v1/projects/p/locations/us-central1/publishers/anthropic/models/claude@date:rawPredict", BuildAnthropicModelURL("", "v1", "p", "us-central1", "claude@date", "rawPredict"))
	require.Empty(t, BuildGoogleModelURL("://broken", "v1", "p", "global", "gemini", "generateContent"))
}

func TestVertexAPIKeyAndStreamOptionsAreIndependentQueryValues(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://proxy.example/v1", UpstreamModelName: "gemini-test", ApiKey: "key&other=value", ChannelOtherSettings: dto.ChannelOtherSettings{VertexKeyType: dto.VertexKeyTypeAPIKey}}, IsStream: true}
	provider := &Adaptor{}
	provider.Init(info)
	endpoint, err := provider.GetRequestURL(info)
	require.NoError(t, err)
	parsed, err := url.Parse(endpoint)
	require.NoError(t, err)
	require.Equal(t, "key&other=value", parsed.Query().Get("key"))
	require.Equal(t, "sse", parsed.Query().Get("alt"))
	require.Empty(t, parsed.Query().Get("other"))
	require.Equal(t, "/v1/publishers/google/models/gemini-test:streamGenerateContent", parsed.Path)
}

func TestVertexRegionConfigurationDoesNotPanicOnNonStrings(t *testing.T) {
	require.Equal(t, "us-central1", GetModelRegion(`{"gemini":123,"default":"us-central1"}`, "gemini"))
	require.Equal(t, "global", GetModelRegion(`{"gemini":true}`, "gemini"))
	require.Equal(t, "global", GetModelRegion(`{broken`, "gemini"))
}
