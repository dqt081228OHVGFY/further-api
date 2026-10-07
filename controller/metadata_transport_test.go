package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMetadataCatalogRequiresExplicitConfiguredSource(t *testing.T) {
	for _, base := range []string{"", "file:///tmp/models", "https://user:secret@example.com", "https://example.com?key=secret"} {
		t.Setenv("FURTHER_METADATA_BASE", base)
		models, vendors := getUpstreamURLs("zh")
		require.Empty(t, models)
		require.Empty(t, vendors)
	}
	t.Setenv("FURTHER_METADATA_BASE", "https://catalog.example.com/further/")
	models, vendors := getUpstreamURLs("zh-CN")
	require.Equal(t, "https://catalog.example.com/further/zh/models.json", models)
	require.Equal(t, "https://catalog.example.com/further/zh/vendors.json", vendors)
}

func TestMetadataInvalidResponseCannotPoisonConditionalCache(t *testing.T) {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if count.Add(1) == 1 {
			w.Header().Set("ETag", "invalid")
			_, _ = w.Write([]byte(`{"success":false}`))
			return
		}
		require.Empty(t, r.Header.Get("If-None-Match"))
		_, _ = w.Write([]byte(`[{"name":"valid"}]`))
	}))
	t.Cleanup(server.Close)
	var result upstreamEnvelope[upstreamVendor]
	require.Error(t, fetchJSON(context.Background(), server.URL, &result))
	require.NoError(t, fetchJSON(context.Background(), server.URL, &result))
	require.Equal(t, "valid", result.Data[0].Name)
	require.EqualValues(t, 2, count.Load())
}

func TestMetadataRetryStopsWhenRequestIsCancelled(t *testing.T) {
	var count atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		cancel()
	}))
	t.Cleanup(server.Close)
	started := time.Now()
	var result upstreamEnvelope[upstreamVendor]
	err := fetchJSON(ctx, server.URL, &result)
	require.True(t, errors.Is(err, context.Canceled))
	require.Less(t, time.Since(started), 500*time.Millisecond)
	require.EqualValues(t, 1, count.Load())
}
