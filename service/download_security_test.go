package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFileDownloadStopsWhenInboundRequestIsCanceled(t *testing.T) {
	notificationHTTPFixture(t)
	started, canceled := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	defer upstream.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
	done := make(chan error, 1)
	go func() { _, err := loadFromURL(c, upstream.URL); done <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("file fetch did not start")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("abandoned request retained outbound fetch")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("upstream connection was not canceled")
	}
}

func TestFileDownloadDeadlineAlsoBoundsStalledResponseBody(t *testing.T) {
	notificationHTTPFixture(t)
	previousTimeout := fileDownloadTimeout
	fileDownloadTimeout = 50 * time.Millisecond
	t.Cleanup(func() { fileDownloadTimeout = previousTimeout })
	var canceled atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		canceled.Store(true)
	}))
	defer upstream.Close()
	response, err := DoDownloadRequest(upstream.URL)
	require.NoError(t, err)
	_, err = io.ReadAll(response.Body)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, response.Body.Close())
	require.Eventually(t, canceled.Load, time.Second, 5*time.Millisecond)
}
