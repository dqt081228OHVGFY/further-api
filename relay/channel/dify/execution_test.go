package dify

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

func TestDifyRemoteImageUsesTheImageFileContract(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://dify.example"}}
	request := dto.GeneralOpenAIRequest{Messages: []dto.Message{{Role: "user", Content: []dto.MediaContent{{Type: dto.ContentTypeText, Text: "describe"}, {Type: dto.ContentTypeImageURL, ImageUrl: &dto.MessageImageUrl{Url: "https://media.example/image.png"}}}}}}
	converted, err := requestOpenAI2Dify(c, info, request)
	require.NoError(t, err)
	require.Len(t, converted.Files, 1)
	require.Equal(t, "image", converted.Files[0].Type)
	require.Equal(t, "remote_url", converted.Files[0].TransferMode)
	require.Contains(t, converted.Query, "describe")
}

func TestDifyErrorFrameDoesNotEmitSuccessCompletion(t *testing.T) {
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "dify-model"}, RelayFormat: types.RelayFormatOpenAI}
	response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("data: {\"event\":\"error\",\"message\":\"provider failed\"}\n\n"))}
	_, err := difyStreamHandler(c, info, response)
	require.NotNil(t, err)
	require.NotContains(t, writer.Body.String(), "[DONE]")
}
