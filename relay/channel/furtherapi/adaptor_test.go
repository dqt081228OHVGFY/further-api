package furtherapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

func TestGatewayURL(t *testing.T) {
	tests := []struct {
		name string
		base string
		path string
		mode int
		want string
		err  bool
	}{
		{"chat", "https://gateway.example/prefix/", "/v1/chat/completions?trace=1", 0, "https://gateway.example/prefix/v1/chat/completions?trace=1", false},
		{"claude", "https://gateway.example", "/v1/messages", 0, "https://gateway.example/v1/messages", false},
		{"gemini", "https://gateway.example", "/v1beta/models/gemini-2:streamGenerateContent?alt=sse", 0, "https://gateway.example/v1beta/models/gemini-2:streamGenerateContent?alt=sse", false},
		{"alpha search", "https://gateway.example", "/ignored", relayconstant.RelayModeAlphaSearch, "https://gateway.example/v1/alpha/search", false},
		{"absolute target", "https://gateway.example", "https://other.example/v1/models", 0, "", true},
		{"network path", "https://gateway.example", "//other.example/v1/models", 0, "", true},
		{"base credentials", "https://user:pass@gateway.example", "/v1/models", 0, "", true},
		{"base query", "https://gateway.example/prefix?key=secret", "/v1/models", 0, "", true},
		{"base empty query", "https://gateway.example/prefix?", "/v1/models", 0, "", true},
		{"base fragment", "https://gateway.example/prefix#section", "/v1/models", 0, "", true},
		{"base empty fragment", "https://gateway.example/prefix#", "/v1/models", 0, "", true},
		{"base empty hostname", "https://:443/prefix", "/v1/models", 0, "", true},
		{"base empty port", "https://gateway.example:/prefix", "/v1/models", 0, "", true},
		{"base port zero", "https://gateway.example:0/prefix", "/v1/models", 0, "", true},
		{"base port overflow", "https://gateway.example:65536/prefix", "/v1/models", 0, "", true},
		{"base opaque", "https:gateway.example", "/v1/models", 0, "", true},
		{"base whitespace", "https://gateway.example/pre fix", "/v1/models", 0, "", true},
		{"base traversal", "https://gateway.example/prefix/%2e%2e", "/v1/models", 0, "", true},
		{"path fragment", "https://gateway.example", "/v1/models#section", 0, "", true},
		{"path empty fragment", "https://gateway.example", "/v1/models#", 0, "", true},
		{"query fragment", "https://gateway.example", "/v1/models?trace=1#section", 0, "", true},
		{"path whitespace", "https://gateway.example", "/v1/my model", 0, "", true},
		{"path control", "https://gateway.example", "/v1/models?trace=\n", 0, "", true},
		{"path traversal", "https://gateway.example/prefix", "/../v1/models", 0, "", true},
		{"encoded path traversal", "https://gateway.example/prefix", "/%2E%2E/v1/models", 0, "", true},
		{"encoded network path", "https://gateway.example/prefix", "/%2fother.example/v1/models", 0, "", true},
		{"encoded backslash", "https://gateway.example/prefix", "/v1/%5c..%5cmodels", 0, "", true},
		{"escaped path and signed query", "https://gateway.example/prefix%2Ftenant/", "/v1/files/id%2Fpart?sig=a%2Bb&x=1&x=2", 0, "https://gateway.example/prefix%2Ftenant/v1/files/id%2Fpart?sig=a%2Bb&x=1&x=2", false},
		{"escaped delimiters", "https://gateway.example", "/v1/files/a%3Fb%23c?sig=a%23b", 0, "https://gateway.example/v1/files/a%3Fb%23c?sig=a%23b", false},
		{"empty request query", "https://gateway.example", "/v1/models?", 0, "https://gateway.example/v1/models?", false},
		{"responses", "https://gateway.example/prefix/", "/v1/responses?trace=1", relayconstant.RelayModeResponses, "https://gateway.example/prefix/v1/responses?trace=1", false},
		{"responses compact", "https://gateway.example", "/v1/responses/compact", relayconstant.RelayModeResponsesCompact, "https://gateway.example/v1/responses/compact", false},
		{"image edits", "https://gateway.example", "/v1/images/edits", relayconstant.RelayModeImagesEdits, "https://gateway.example/v1/images/edits", false},
		{"audio", "https://gateway.example", "/v1/audio/transcriptions", relayconstant.RelayModeAudioTranscription, "https://gateway.example/v1/audio/transcriptions", false},
		{"embeddings", "https://gateway.example", "/v1/embeddings", relayconstant.RelayModeEmbeddings, "https://gateway.example/v1/embeddings", false},
		{"realtime TLS", "https://gateway.example/prefix", "/v1/realtime?model=gpt-realtime", relayconstant.RelayModeRealtime, "wss://gateway.example/prefix/v1/realtime?model=gpt-realtime", false},
		{"realtime local", "http://gateway.example", "/v1/realtime?model=gpt-realtime", relayconstant.RelayModeRealtime, "ws://gateway.example/v1/realtime?model=gpt-realtime", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := upstreamURL(&common.RelayInfo{ChannelMeta: &common.ChannelMeta{
				ChannelBaseUrl: tt.base,
			}, RequestURLPath: tt.path, RelayMode: tt.mode})
			if (err != nil) != tt.err {
				t.Fatalf("upstreamURL() error = %v, want error %v", err, tt.err)
			}
			if got != tt.want {
				t.Fatalf("upstreamURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGatewayURLRequiresChannelMeta(t *testing.T) {
	for _, info := range []*common.RelayInfo{nil, {}} {
		if _, err := upstreamURL(info); err == nil {
			t.Fatal("missing channel metadata must return an error")
		}
	}
}

func TestProtocolHeaders(t *testing.T) {
	tests := []struct {
		name   string
		format types.RelayFormat
		key    string
	}{
		{"openai", types.RelayFormatOpenAI, ""},
		{"responses", types.RelayFormatOpenAIResponses, ""},
		{"claude", types.RelayFormatClaude, "X-Api-Key"},
		{"gemini", types.RelayFormatGemini, "X-Goog-Api-Key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("{}"))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Request.Header.Set("anthropic-version", "2023-11-01")
			headers := make(http.Header)
			info := &common.RelayInfo{RelayFormat: tt.format, ChannelMeta: &common.ChannelMeta{ApiKey: "gateway-secret"}}
			if err := new(Adaptor).SetupRequestHeader(ctx, &headers, info); err != nil {
				t.Fatal(err)
			}
			if got := headers.Get("Authorization"); got != "Bearer gateway-secret" {
				t.Fatalf("authorization = %q", got)
			}
			if tt.key != "" && headers.Get(tt.key) != "gateway-secret" {
				t.Fatalf("%s = %q", tt.key, headers.Get(tt.key))
			}
			if tt.format == types.RelayFormatClaude && headers.Get("anthropic-version") != "2023-11-01" {
				t.Fatalf("anthropic-version = %q", headers.Get("anthropic-version"))
			}
			if got := headers.Get("Content-Type"); got != "application/json" {
				t.Fatalf("content type = %q", got)
			}
		})
	}
}

func TestGatewayName(t *testing.T) {
	if got := new(Adaptor).GetChannelName(); got != "further-api" {
		t.Fatalf("channel name = %q", got)
	}
}
