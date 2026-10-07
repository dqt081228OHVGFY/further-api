package gemini

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSingleGeminiEmbeddingBecomesOneOpenAIVector(t *testing.T) {
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-embedding-001"}}
	response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"embedding":{"values":[0.1,0.2]}}`))}
	_, err := GeminiEmbeddingHandler(c, info, response)
	require.Nil(t, err)
	require.Contains(t, writer.Body.String(), `"embedding":[0.1,0.2]`)
	require.Contains(t, writer.Body.String(), `"index":0`)
}

func TestGeminiModelPaginationEncodesTokensAndDeduplicatesModels(t *testing.T) {
	const token = "part&injected=value +/next"
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			_, _ = io.WriteString(w, `{"models":[{"name":"models/first"}],"nextPageToken":"`+token+`"}`)
			return
		}
		require.Equal(t, token, r.URL.Query().Get("pageToken"))
		require.Empty(t, r.URL.Query().Get("injected"))
		_, _ = io.WriteString(w, `{"models":[{"name":"models/first"},{"name":"models/second"}]}`)
	}))
	defer server.Close()
	models, err := FetchGeminiModelsContext(context.Background(), server.URL, "test-key", "")
	require.NoError(t, err)
	require.Equal(t, []string{"first", "second"}, models)
	require.EqualValues(t, 2, calls.Load())
}

func TestGeminiModelPaginationStopsRepeatedTokensAndCancellation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"models":[],"nextPageToken":"repeated"}`)
	}))
	defer server.Close()
	_, err := FetchGeminiModelsContext(context.Background(), server.URL, "test-key", "")
	require.ErrorContains(t, err, "repeated")
	require.EqualValues(t, 2, calls.Load())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = FetchGeminiModelsContext(ctx, server.URL, "test-key", "")
	require.ErrorIs(t, err, context.Canceled)
	require.EqualValues(t, 2, calls.Load())
}

func TestImagenFilteredOnlyResultDoesNotReturnEmptySuccess(t *testing.T) {
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "imagen-4"}}
	response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"predictions":[{"raiFilteredReason":"filtered"}]}`))}
	_, err := GeminiImageHandler(c, info, response)
	require.NotNil(t, err)
	require.Empty(t, writer.Body.String())
}
