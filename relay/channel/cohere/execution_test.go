package cohere

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCohereHistoryKeepsEarlierUserMessages(t *testing.T) {
	converted := requestOpenAI2Cohere(dto.GeneralOpenAIRequest{Model: "command", Messages: []dto.Message{{Role: "user", Content: "first"}, {Role: "assistant", Content: "answer"}, {Role: "user", Content: "latest"}}})
	require.Equal(t, "latest", converted.Message)
	require.Len(t, converted.ChatHistory, 2)
	require.Equal(t, ChatHistory{Role: "USER", Message: "first"}, converted.ChatHistory[0])
}

func TestCohereNDJSONStreamKeepsFinalUsageAndCompletion(t *testing.T) {
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "command"}, RelayFormat: types.RelayFormatOpenAI, ShouldIncludeUsage: true}
	response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{\"event_type\":\"text-generation\",\"text\":\"hello\"}\n{\"is_finished\":true,\"finish_reason\":\"COMPLETE\",\"response\":{\"meta\":{\"billed_units\":{\"input_tokens\":8,\"output_tokens\":3}}}}\n"))}
	usage, err := cohereStreamHandler(c, info, response)
	require.Nil(t, err)
	require.Equal(t, 11, usage.TotalTokens)
	require.Contains(t, writer.Body.String(), "hello")
	require.Contains(t, writer.Body.String(), "[DONE]")
}
