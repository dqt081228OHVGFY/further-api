package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupPayloadCapacity(t *testing.T) {
	t.Helper()
	oldMemory, oldFile, oldTotal := constant.ExternalPayloadMemoryBudgetMB, constant.MaxFileDownloadMB, constant.FileFetchRequestMaxMB
	oldFileConcurrency, oldPollConcurrency, oldPollSize := constant.FileFetchMaxConcurrency, constant.TaskPollMaxConcurrency, constant.TaskPollMaxResponseMB
	oldDisk := common.GetDiskCacheConfig()
	baseline := common.PayloadMemoryReservedBytes()
	constant.ExternalPayloadMemoryBudgetMB, constant.MaxFileDownloadMB, constant.FileFetchRequestMaxMB = 12, 1, 1
	constant.FileFetchMaxConcurrency, constant.TaskPollMaxConcurrency, constant.TaskPollMaxResponseMB = 1, 1, 1
	common.SetDiskCacheConfig(common.DiskCacheConfig{Enabled: false, Path: t.TempDir(), ThresholdMB: 1, MaxSizeMB: 4})
	t.Cleanup(func() {
		constant.ExternalPayloadMemoryBudgetMB, constant.MaxFileDownloadMB, constant.FileFetchRequestMaxMB = oldMemory, oldFile, oldTotal
		constant.FileFetchMaxConcurrency, constant.TaskPollMaxConcurrency, constant.TaskPollMaxResponseMB = oldFileConcurrency, oldPollConcurrency, oldPollSize
		common.SetDiskCacheConfig(oldDisk)
		require.Equal(t, baseline, common.PayloadMemoryReservedBytes())
	})
}

func TestInlineCumulativeCapacityDeduplicatesAndReleasesOnCacheClose(t *testing.T) {
	setupPayloadCapacity(t)
	baseline := common.PayloadMemoryReservedBytes()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	first := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'a'}, 600<<10))
	second := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'b'}, 600<<10))
	cache, err := LoadFileSource(c, types.NewBase64FileSource(first, "application/octet-stream"))
	require.NoError(t, err)
	duplicate, err := LoadFileSource(c, types.NewBase64FileSource(first, "application/octet-stream"))
	require.NoError(t, err)
	require.Same(t, cache, duplicate)
	_, err = LoadFileSource(c, types.NewBase64FileSource(second, "application/octet-stream"))
	require.ErrorIs(t, err, ErrRequestFileCapacity)
	require.Positive(t, common.PayloadMemoryReservedBytes())
	CleanupFileSources(c)
	require.Equal(t, baseline, common.PayloadMemoryReservedBytes())
	require.NoError(t, cache.Close())
}

func TestUnknownLengthDownloadStreamsToPrivateCacheAndReleases(t *testing.T) {
	setupPayloadCapacity(t)
	if httpClient == nil {
		InitHttpClient()
	}
	configured := common.GetDiskCacheConfig()
	configured.Enabled = true
	common.SetDiskCacheConfig(configured)
	fetch := system_setting.GetFetchSetting()
	oldFetch := *fetch
	fetch.EnableSSRFProtection, fetch.AllowPrivateIp = true, true
	fetch.AllowedPorts = []string{"1-65535"}
	t.Cleanup(func() { *fetch = oldFetch })
	payload := bytes.Repeat([]byte("streamed-local-payload"), 12<<10)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.(http.Flusher).Flush()
		_, _ = writer.Write(payload)
	}))
	defer server.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	cache, err := LoadFileSource(c, types.NewURLFileSource(server.URL), "local-capacity-test")
	require.NoError(t, err)
	require.True(t, cache.IsDisk())
	reader, err := cache.OpenBase64Reader()
	require.NoError(t, err)
	actual, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, reader))
	require.NoError(t, err)
	require.Equal(t, payload, actual)
	require.NoError(t, reader.Close())
	require.Positive(t, common.PayloadMemoryReservedBytes())
	CleanupFileSources(c)
	require.Zero(t, common.GetDiskCacheStats().CurrentDiskUsageBytes)
}

func TestPollingCapacityAndOversizedResponseAreBounded(t *testing.T) {
	setupPayloadCapacity(t)
	release, err := AcquireTaskPollResources(context.Background())
	require.NoError(t, err)
	_, err = AcquireTaskPollResources(context.Background())
	require.ErrorIs(t, err, ErrExternalPayloadCapacity)
	response := &http.Response{Body: io.NopCloser(bytes.NewReader(bytes.Repeat([]byte{'x'}, 2<<20)))}
	body, err := ReadTaskPollBody(response)
	require.Error(t, err)
	require.Len(t, body, (1<<20)+1)
	release()
	release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = AcquireTaskPollResources(ctx)
	require.ErrorIs(t, err, context.Canceled)
}
