package jsplugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/model"
	pluginruntime "github.com/dqt081228OHVGFY/further-api/pkg/jsplugin"
	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const querySecurityPlugin = `
export const meta = {apiVersion:1,key:"query-security",name:"Query Security",version:"1.0.0",author:{name:"Test"},models:["m"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx){return {url:ctx.baseUrl};}
export function parseSubmitResponse(){return {taskId:"task"};}
export function buildQueryRequest(ctx){return {url:ctx.baseUrl+"/query",headers:{"X-Api-Key":ctx.apiKey}};}
export function buildBatchQueryRequest(ctx){return buildQueryRequest(ctx);}
export function parseTaskResult(){return {status:"SUCCESS"};}
`

func TestTaskQueryContextCancelsSingleAndBatchHTTPRequests(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(fmt.Sprintf("batch=%t", batch), func(t *testing.T) {
			started := make(chan struct{})
			cancelled := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(cancelled) }))
			defer server.Close()
			plugin, err := pluginruntime.NewRegistry().Register(querySecurityPlugin, pluginruntime.Options{})
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				adaptor := New(plugin)
				var response *http.Response
				var err error
				if batch {
					response, err = adaptor.FetchTaskBatchContext(ctx, server.URL, "secret", []*model.Task{{TaskID: "task"}}, "")
				} else {
					response, err = adaptor.FetchTaskContext(ctx, server.URL, "secret", &model.Task{TaskID: "task"}, "")
				}
				if response != nil && response.Body != nil {
					response.Body.Close()
				}
				result <- err
			}()
			select {
			case <-started:
			case err := <-result:
				t.Fatalf("query returned before request: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("query did not start")
			}
			cancel()
			select {
			case err := <-result:
				require.True(t, errors.Is(err, context.Canceled), "got %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("query did not cancel")
			}
			select {
			case <-cancelled:
			case <-time.After(5 * time.Second):
				t.Fatal("upstream request remained active")
			}
		})
	}
}

func TestTaskQueryRevalidatesDestinationAfterCustomRedirectCallback(t *testing.T) {
	var hits atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.WriteHeader(http.StatusOK) }))
	defer destination.Close()
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/next", http.StatusFound) }))
	defer gateway.Close()
	plugin, err := pluginruntime.NewRegistry().Register(querySecurityPlugin, pluginruntime.Options{})
	require.NoError(t, err)
	client, err := service.GetHttpClientWithProxy("")
	require.NoError(t, err)
	previous := client.CheckRedirect
	t.Cleanup(func() { client.CheckRedirect = previous })
	client.CheckRedirect = func(next *http.Request, _ []*http.Request) error {
		target, err := url.Parse(destination.URL + "/replaced")
		if err != nil {
			return err
		}
		next.URL = target
		return nil
	}
	response, err := New(plugin).FetchTaskContext(context.Background(), gateway.URL, "secret", &model.Task{TaskID: "task"}, "")
	require.Error(t, err)
	require.Nil(t, response)
	require.Zero(t, hits.Load())
}

func TestTaskQueryValidatesEveryRedirect(t *testing.T) {
	for _, batch := range []bool{false, true} {
		for _, scenario := range []string{"same host", "unapproved host", "approved host", "second hop", "userinfo", "fragment"} {
			t.Run(fmt.Sprintf("batch=%t/%s", batch, scenario), func(t *testing.T) {
				var destinationHits, finalHits atomic.Int32
				final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					finalHits.Add(1)
					w.WriteHeader(http.StatusOK)
				}))
				defer final.Close()
				destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					destinationHits.Add(1)
					assert.Equal(t, "secret", r.Header.Get("X-Api-Key"))
					if scenario == "second hop" {
						http.Redirect(w, r, final.URL+"/done", http.StatusFound)
						return
					}
					_, _ = io.WriteString(w, "done")
				}))
				defer destination.Close()
				target := destination.URL + "/done"
				switch scenario {
				case "same host":
					target = "/done"
				case "userinfo":
					target = strings.Replace(target, "http://", "http://user:pass@", 1)
				case "fragment":
					target += "#preview"
				}
				gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/done" {
						assert.Equal(t, "secret", r.Header.Get("X-Api-Key"))
						_, _ = io.WriteString(w, "done")
						return
					}
					http.Redirect(w, r, target, http.StatusFound)
				}))
				defer gateway.Close()
				plugin, err := pluginruntime.NewRegistry().Register(querySecurityPlugin, pluginruntime.Options{})
				require.NoError(t, err)
				if scenario != "unapproved host" {
					plugin.Meta.AllowedHosts = []string{strings.TrimPrefix(destination.URL, "http://")}
				}
				client, err := service.GetHttpClientWithProxy("")
				require.NoError(t, err)
				previous := client.CheckRedirect
				policyCalls := 0
				client.CheckRedirect = func(*http.Request, []*http.Request) error {
					policyCalls++
					return nil
				}
				t.Cleanup(func() { client.CheckRedirect = previous })
				adaptor := New(plugin)
				var response *http.Response
				if batch {
					response, err = adaptor.FetchBatchTasks(gateway.URL, "secret", []*model.Task{{TaskID: "task"}}, "")
				} else {
					response, err = adaptor.FetchTask(gateway.URL, "secret", &model.Task{TaskID: "task"}, "")
				}
				if scenario == "same host" || scenario == "approved host" {
					require.NoError(t, err)
					require.NotNil(t, response)
					body, readErr := io.ReadAll(response.Body)
					require.NoError(t, response.Body.Close())
					require.NoError(t, readErr)
					assert.Equal(t, "done", string(body))
				} else {
					require.Error(t, err)
					assert.Nil(t, response)
				}
				wantHits := int32(0)
				wantPolicyCalls := 0
				if scenario == "approved host" || scenario == "second hop" {
					wantHits = 1
					wantPolicyCalls = 1
				} else if scenario == "same host" {
					wantPolicyCalls = 1
				}
				assert.Equal(t, wantHits, destinationHits.Load())
				assert.Zero(t, finalHits.Load(), "a later redirect must not bypass the allowlist")
				assert.Equal(t, wantPolicyCalls, policyCalls)
				require.NoError(t, client.CheckRedirect(nil, nil), "the shared client policy must remain unchanged")
				assert.Equal(t, wantPolicyCalls+1, policyCalls)
			})
		}
	}
}

func TestTaskQueryPreservesRedirectPolicy(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	}))
	defer gateway.Close()
	plugin, err := pluginruntime.NewRegistry().Register(querySecurityPlugin, pluginruntime.Options{})
	require.NoError(t, err)
	client, err := service.GetHttpClientWithProxy("")
	require.NoError(t, err)
	previous := client.CheckRedirect
	t.Cleanup(func() { client.CheckRedirect = previous })
	for _, keepResponse := range []bool{false, true} {
		t.Run(fmt.Sprintf("keepResponse=%t", keepResponse), func(t *testing.T) {
			client.CheckRedirect = nil
			if keepResponse {
				client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			}
			response, fetchErr := New(plugin).FetchTask(gateway.URL, "secret", &model.Task{}, "")
			if keepResponse {
				require.NoError(t, fetchErr)
				require.NotNil(t, response)
				assert.Equal(t, http.StatusFound, response.StatusCode)
				require.NoError(t, response.Body.Close())
			} else {
				require.ErrorContains(t, fetchErr, "stopped after 10 redirects")
			}
		})
	}
}

func TestTaskQueryRejectsHTTPSRedirectDowngrade(t *testing.T) {
	var destinationHits atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destinationHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer destination.Close()
	gateway := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+"/done", http.StatusFound)
	}))
	defer gateway.Close()
	plugin, err := pluginruntime.NewRegistry().Register(querySecurityPlugin, pluginruntime.Options{})
	require.NoError(t, err)
	plugin.Meta.AllowedHosts = []string{strings.TrimPrefix(destination.URL, "http://")}
	client, err := service.GetHttpClientWithProxy("")
	require.NoError(t, err)
	previousTransport, previousRedirect := client.Transport, client.CheckRedirect
	client.Transport = gateway.Client().Transport
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return nil }
	t.Cleanup(func() {
		client.Transport, client.CheckRedirect = previousTransport, previousRedirect
	})
	response, err := New(plugin).FetchTask(gateway.URL, "secret", &model.Task{}, "")
	require.ErrorContains(t, err, "must use HTTPS")
	assert.Nil(t, response)
	assert.Zero(t, destinationHits.Load(), "even an approved host must not receive a downgraded request")
}

func TestCredentiallessArtifactURLBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, url string
		invalid   bool
	}{
		{"signed CDN", "https://cdn.example/video%2Fpart.mp4?Signature=abc%2Bdef&x=1&x=2", false},
		{"local CDN", "http://127.0.0.1:8080/video.mp4", false},
		{"userinfo", "https://user:pass@cdn.example/video.mp4", true},
		{"empty userinfo", "https://@cdn.example/video.mp4", true},
		{"fragment", "https://cdn.example/video.mp4#preview", true},
		{"empty fragment", "https://cdn.example/video.mp4#", true},
		{"empty hostname", "https://:443/video.mp4", true},
		{"empty port", "https://cdn.example:/video.mp4", true},
		{"port zero", "https://cdn.example:0/video.mp4", true},
		{"port overflow", "https://cdn.example:65536/video.mp4", true},
		{"whitespace", "https://cdn.example/video file.mp4", true},
		{"control", "https://cdn.example/video.mp4?sig=\n", true},
		{"relative", "/video.mp4", true},
		{"network path", "//cdn.example/video.mp4", true},
		{"opaque", "https:cdn.example/video.mp4", true},
		{"non-http", "ftp://cdn.example/video.mp4", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := strings.Replace(mockPlugin, `export function buildContentRequest() { throw new Error("artifact_not_found"); }`,
				`export function buildContentRequest(){return {url:`+strconv.Quote(tc.url)+`,method:"GET",credentialless:true};}`, 1)
			plugin, err := pluginruntime.NewRegistry().Register(source, pluginruntime.Options{})
			require.NoError(t, err)
			adaptor := New(plugin)
			adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://provider.example", ApiKey: "secret"}})
			descriptor, err := adaptor.BuildContentRequest(&model.Task{TaskID: "task"}, "video", channel.TaskArtifactClientRequest{Method: http.MethodGet})
			if tc.invalid {
				require.Error(t, err)
				assert.Nil(t, descriptor)
			} else {
				require.NoError(t, err)
				require.NotNil(t, descriptor)
				assert.Equal(t, tc.url, descriptor.URL)
				assert.True(t, descriptor.Credentialless)
				assert.Empty(t, descriptor.Headers)
			}
		})
	}
}
