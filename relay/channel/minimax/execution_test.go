package minimax

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func miniMaxTestSpeech(t *testing.T, format, body string) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	c.Set("response_format", "wav")
	c.Set("minimax_stream_format", format)
	response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, IsStream: true}
	return c, writer, response, info
}
func TestNativeSpeechStreamsBothOutputFormatsWithoutRepeatingAggregate(t *testing.T) {
	const body = "data: {\"data\":{\"audio\":\"6869\",\"status\":1}}\n\ndata: {\"data\":{\"audio\":\"6869\",\"status\":2},\"extra_info\":{\"usage_characters\":7}}\n\ndata: [DONE]\n\n"
	for _, format := range []string{"sse", "audio"} {
		t.Run(format, func(t *testing.T) {
			c, writer, response, info := miniMaxTestSpeech(t, format, body)
			usage, failure := handleTTSResponse(c, response, info)
			require.Nil(t, failure)
			require.Equal(t, 7, usage.(*dto.Usage).PromptTokens)
			if format == "audio" {
				require.Equal(t, "hi", writer.Body.String())
				require.Equal(t, "audio/wav", writer.Header().Get("Content-Type"))
				require.True(t, info.DisablePing)
			} else {
				require.Equal(t, 1, strings.Count(writer.Body.String(), "speech.audio.delta"))
				require.Contains(t, writer.Body.String(), `"audio":"aGk="`)
				require.Contains(t, writer.Body.String(), "speech.audio.done")
			}
		})
	}
}
func TestNativeSpeechRejectsInvalidHexAndCanceledResponses(t *testing.T) {
	c, writer, response, info := miniMaxTestSpeech(t, "sse", "data: {\"data\":{\"audio\":\"invalid\",\"status\":1}}\n\n")
	_, failure := handleTTSResponse(c, response, info)
	require.NotNil(t, failure)
	require.Empty(t, writer.Body.String())
	emptyContext, _, emptyResponse, emptyInfo := miniMaxTestSpeech(t, "sse", "data: {}\n\ndata: [DONE]\n\n")
	_, emptyFailure := handleTTSResponse(emptyContext, emptyResponse, emptyInfo)
	require.NotNil(t, emptyFailure)
	ctx, cancel := context.WithCancel(c.Request.Context())
	cancel()
	c.Request = c.Request.WithContext(ctx)
	response.Body = io.NopCloser(strings.NewReader("{}"))
	response.Header.Set("Content-Type", "application/json")
	_, failure = handleTTSResponse(c, response, info)
	require.NotNil(t, failure)
}
