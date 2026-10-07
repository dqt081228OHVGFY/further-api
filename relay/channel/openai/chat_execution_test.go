package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func chatExecutionContext(body string) (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo, *http.Response) {
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: "gpt-4o"}, RelayMode: relayconstant.RelayModeChatCompletions, RelayFormat: types.RelayFormatOpenAI, ShouldIncludeUsage: true}
	response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
	return c, writer, info, response
}

func TestChatStreamRetainsUsageBeforeLaterEmptyFrames(t *testing.T) {
	c, writer, info, response := chatExecutionContext("data: {\"id\":\"chat-id\",\"model\":\"gpt-4o\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: {\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":15,\"total_tokens\":115},\"choices\":[]}\n\ndata: {\"choices\":[]}\n\ndata: {\"choices\":[]}\n\ndata: [DONE]\n\n")
	usage, err := OaiStreamHandler(c, info, response)
	require.Nil(t, err)
	require.Equal(t, 100, usage.PromptTokens)
	require.Equal(t, 15, usage.CompletionTokens)
	require.Contains(t, writer.Body.String(), "[DONE]")
}

func TestChatStreamReturnsUpstreamFailureWithoutCompletionFrame(t *testing.T) {
	c, writer, info, response := chatExecutionContext("data: {\"id\":\"chat-id\",\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\ndata: {\"error\":{\"type\":\"server_error\",\"message\":\"upstream failed\"}}\n\n")
	_, err := OaiStreamHandler(c, info, response)
	require.NotNil(t, err)
	require.NotContains(t, writer.Body.String(), "[DONE]")
}

func TestChatCancelledStreamDoesNotEmitCompletion(t *testing.T) {
	c, writer, info, response := chatExecutionContext("data: {\"choices\":[]}\n\ndata: [DONE]\n\n")
	ctx, cancel := context.WithCancel(c.Request.Context())
	cancel()
	c.Request = c.Request.WithContext(ctx)
	_, _ = OaiStreamHandler(c, info, response)
	require.NotContains(t, writer.Body.String(), "[DONE]")
}

func TestThinkingProjectionKeepsChoicesIndependent(t *testing.T) {
	c, writer, info, _ := chatExecutionContext("")
	InitializeProtocol(info)
	frame := `{"choices":[{"index":0,"delta":{"reasoning_content":"first"}},{"index":1,"delta":{"reasoning_content":"second"}}]}`
	require.NoError(t, sendStreamData(c, info, frame, false, true))
	require.NotContains(t, writer.Body.String(), "firstsecond")
	var decoded dto.ChatCompletionsStreamResponse
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(writer.Body.String(), "data: "))), &decoded))
	require.Equal(t, "<think>\nfirst", decoded.Choices[0].Delta.GetContentString())
	require.Equal(t, "<think>\nsecond", decoded.Choices[1].Delta.GetContentString())
}

func TestNullChatResponseIsAProtocolError(t *testing.T) {
	c, _, info, response := chatExecutionContext("null")
	_, err := OpenaiHandler(c, info, response)
	require.NotNil(t, err)
}
