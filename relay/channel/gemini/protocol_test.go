package gemini

import (
	"net/url"
	"testing"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestEmbeddingPayloadSelectsTheBatchEndpointAndDimensions(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://gemini.example", UpstreamModelName: "gemini-embedding-001"}}
	dimensions := 768
	value, err := BatchEmbeddings(info, dto.EmbeddingRequest{Input: []string{"one", "two"}, Dimensions: &dimensions})
	require.NoError(t, err)
	batch := value.(map[string]any)["requests"].([]map[string]any)
	require.Len(t, batch, 2)
	require.Equal(t, 768, batch[0]["outputDimensionality"])
	endpoint, err := ContentEndpoint(info)
	require.NoError(t, err)
	require.Contains(t, endpoint, ":batchEmbedContents")
}

func TestGeminiStreamEndpointKeepsModelPunctuationOutOfItsQuery(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://gemini.example/proxy", UpstreamModelName: "gemini-test?extra=true"}, IsStream: true, RelayMode: relayconstant.RelayModeGemini}
	endpoint, err := ContentEndpoint(info)
	require.NoError(t, err)
	parsed, err := url.Parse(endpoint)
	require.NoError(t, err)
	require.Equal(t, "sse", parsed.Query().Get("alt"))
	require.Empty(t, parsed.Query().Get("extra"))
	require.Contains(t, parsed.EscapedPath(), "gemini-test%3Fextra=true:streamGenerateContent")
	require.True(t, info.DisablePing)
}

func TestNativeContentNormalizesMissingRoleAndYouTubeMedia(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-test"}}
	request := &dto.GeminiChatRequest{Contents: []dto.GeminiChatContent{{Parts: []dto.GeminiPart{{FileData: &dto.GeminiFileData{FileUri: "https://www.youtube.com/watch?v=test"}}}}}}
	result, err := NativeContent(info, request)
	require.NoError(t, err)
	require.Equal(t, "user", result.Contents[0].Role)
	require.Equal(t, "video/webm", result.Contents[0].Parts[0].FileData.MimeType)
}
