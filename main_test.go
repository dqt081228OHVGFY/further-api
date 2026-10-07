package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceInitializationRollsBackFailedAndEarlierResources(t *testing.T) {
	var closed []string
	initializationFailure, cleanupFailure := errors.New("migration failed"), errors.New("close failed")
	resources, err := initializeResourcePlan([]resourceStep{
		{name: "database", initialize: func() error { return nil }, close: func() error { closed = append(closed, "database"); return nil }},
		{name: "Redis", initialize: func() error { return initializationFailure }, close: func() error { closed = append(closed, "Redis"); return cleanupFailure }},
		{name: "never", initialize: func() error { t.Fatal("initialized after failure"); return nil }},
	})
	assert.Nil(t, resources)
	assert.ErrorIs(t, err, initializationFailure)
	assert.ErrorIs(t, err, cleanupFailure)
	assert.Equal(t, []string{"Redis", "database"}, closed)
}

func TestResourceCloseRunsAllStepsOnce(t *testing.T) {
	var closed []string
	failure := errors.New("close failed")
	resources, err := initializeResourcePlan([]resourceStep{
		{name: "first", initialize: func() error { return nil }, close: func() error { closed = append(closed, "first"); return nil }},
		{name: "second", initialize: func() error { return nil }, close: func() error { closed = append(closed, "second"); return failure }},
	})
	require.NoError(t, err)
	assert.ErrorIs(t, resources.close(), failure)
	assert.ErrorIs(t, resources.close(), failure)
	assert.Equal(t, []string{"second", "first"}, closed)
}

func TestHTTPShutdownDrainsActiveRequest(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	server := newHTTPServer(common.RuntimeConfig{ReadHeaderTimeout: time.Second}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte("completed"))
	}))
	assert.Zero(t, server.ReadTimeout)
	assert.Zero(t, server.WriteTimeout)
	served, requested := make(chan error, 1), make(chan error, 1)
	go func() { served <- serveHTTP(ctx, server, listener, 2*time.Second, nil) }()
	go func() {
		client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil}}
		response, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			response.Body.Close()
		}
		requested <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("HTTP request did not start")
	}
	cancel()
	select {
	case err := <-served:
		close(release)
		t.Fatalf("server exited before active request drained: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-requested)
	require.NoError(t, <-served)
}

func TestHTTPShutdownTimeoutClosesActiveRequest(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, stopped := make(chan struct{}), make(chan struct{})
	server := newHTTPServer(common.RuntimeConfig{}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(stopped)
	}))
	served, requested := make(chan error, 1), make(chan error, 1)
	go func() { served <- serveHTTP(ctx, server, listener, 20*time.Millisecond, nil) }()
	go func() {
		client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil}}
		response, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			response.Body.Close()
		}
		requested <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP request did not start")
	}
	cancel()
	assert.ErrorIs(t, <-served, context.DeadlineExceeded)
	assert.Error(t, <-requested)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("forced close did not cancel active request")
	}
}

func TestAnalyticsInjectionEscapesValuesAndPreservesMarkers(t *testing.T) {
	values := map[string]string{"UMAMI_WEBSITE_ID": `id" onload="bad`, "UMAMI_SCRIPT_URL": "https://analytics.example/script.js"}
	script := umamiScript(func(name string) string { return values[name] })
	assert.Contains(t, script, "&#34;")
	page := injectAnalytics([]byte("<head><!--umami--></head>"), "<!--umami-->", script)
	assert.NotContains(t, string(page), "<!--umami-->")
	assert.Contains(t, string(page), script)
	assert.Equal(t, 1, strings.Count(string(page), "<script"))
	values["UMAMI_SCRIPT_URL"] = "javascript:alert(1)"
	assert.Empty(t, umamiScript(func(name string) string { return values[name] }))
	assert.Empty(t, googleAnalyticsScript(func(string) string { return `G-X';alert(1)` }))
}
