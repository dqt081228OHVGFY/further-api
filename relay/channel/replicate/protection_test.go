package replicate

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
	"github.com/stretchr/testify/require"
)

func TestReplicateDownloadKeepsPrivateNetworkProtection(t *testing.T) {
	previous := *system_setting.GetFetchSetting()
	t.Cleanup(func() { *system_setting.GetFetchSetting() = previous })
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	fetch := system_setting.GetFetchSetting()
	fetch.EnableSSRFProtection = true
	fetch.AllowPrivateIp = false
	fetch.AllowedPorts = []string{target.Port()}
	c, writer, info := replicateTestContext(t, upstream.URL)
	info.Request = &dto.ImageRequest{ResponseFormat: "b64_json"}
	response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":"succeeded","output":"` + upstream.URL + `/image.png"}`))}
	_, failure := finishReplicateImage(c, response, info)
	require.NotNil(t, failure)
	require.Zero(t, calls.Load())
	require.Empty(t, writer.Body.String())
}

func TestReplicateRejectsOverlongResponseAndMalformedOutputWithoutWriting(t *testing.T) {
	for _, body := range []string{strings.Repeat(" ", int(replicateJSONLimit)+1), `{"status":"succeeded","output":["https://images.example/one.png",1]}`, `{"status":"failed","error":"provider rejected"}`} {
		c, writer, info := replicateTestContext(t, "https://api.replicate.example")
		response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
		_, failure := finishReplicateImage(c, response, info)
		require.NotNil(t, failure)
		require.Empty(t, writer.Body.String())
	}
}
