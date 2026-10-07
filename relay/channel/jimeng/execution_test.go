package jimeng

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestJimengSigningCanonicalizesQueryAndKeepsBodyReplayable(t *testing.T) {
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	first, _ := http.NewRequest(http.MethodPost, "https://vision.example/?q=a+b&Version=2022-08-31&Action=CVProcess", bytes.NewBufferString(`{"prompt":"test"}`))
	second, _ := http.NewRequest(http.MethodPost, "https://vision.example/?Action=CVProcess&q=a%20b&Version=2022-08-31", bytes.NewBufferString(`{"prompt":"test"}`))
	require.NoError(t, signAt(first, "access|secret", now))
	require.NoError(t, signAt(second, "access|secret", now))
	require.Equal(t, first.Header.Get("Authorization"), second.Header.Get("Authorization"))
	require.Equal(t, "20250102T030405Z", first.Header.Get("X-Date"))
	body, err := io.ReadAll(first.Body)
	require.NoError(t, err)
	require.Equal(t, `{"prompt":"test"}`, string(body))
	replay, err := first.GetBody()
	require.NoError(t, err)
	defer replay.Close()
	replayed, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.Equal(t, body, replayed)
	require.Error(t, signAt(first, "access|", now))
}

func TestJimengPairsURLAndBase64WithoutDoubleCountingImages(t *testing.T) {
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	info := &relaycommon.RelayInfo{}
	response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":10000,"data":{"binary_data_base64":["data-one"],"image_urls":["https://image.example/one.png"]}}`))}
	_, err := jimengImageHandler(c, response, info)
	require.Nil(t, err)
	require.Equal(t, 1, strings.Count(writer.Body.String(), `"b64_json"`))
	require.Equal(t, 1, strings.Count(writer.Body.String(), `"url"`))
}
