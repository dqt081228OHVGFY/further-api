package oauth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOAuthRequestsNeverForwardCredentialsToAnotherOrigin(t *testing.T) {
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		for _, method := range []string{http.MethodPost, http.MethodGet} {
			t.Run(http.StatusText(status)+"/"+method, func(t *testing.T) {
				var destinationRequests atomic.Int32
				destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					destinationRequests.Add(1)
					_, _ = io.Copy(io.Discard, r.Body)
					_, _ = io.WriteString(w, `{"access_token":"fixture-token"}`)
				}))
				defer destination.Close()
				source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Redirect(w, r, destination.URL+"/collect", status)
				}))
				defer source.Close()
				request, err := http.NewRequestWithContext(context.Background(), method, source.URL+"/token", strings.NewReader("client_secret=fixture-secret&code=fixture-code"))
				require.NoError(t, err)
				request.Header.Set("Authorization", "Bearer fixture-access-token")
				response, err := doOAuthRequest(request.Context(), source.Client(), request)
				if response != nil {
					response.Body.Close()
				}
				require.ErrorIs(t, err, errOAuthRequestFailed)
				require.Zero(t, destinationRequests.Load(), "cross-origin destination must receive neither the credentials nor the authorization code")
			})
		}
	}
}

func TestOAuthRequestsRejectHTTPSDowngrade(t *testing.T) {
	var destinationRequests atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destinationRequests.Add(1)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer destination.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	request, err := http.NewRequest(http.MethodPost, source.URL, strings.NewReader("client_secret=fixture-secret"))
	require.NoError(t, err)
	response, err := doOAuthRequest(context.Background(), source.Client(), request)
	if response != nil {
		response.Body.Close()
	}
	require.ErrorIs(t, err, errOAuthRequestFailed)
	require.Zero(t, destinationRequests.Load())
}

func TestOAuthSameOriginRedirectsRetainBodyAndClientPolicy(t *testing.T) {
	var destinationRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			http.Redirect(w, r, "/canonical-token", http.StatusTemporaryRedirect)
			return
		}
		destinationRequests.Add(1)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, "client_secret=fixture-secret", string(body))
		_, _ = io.WriteString(w, `{"access_token":"fixture-token"}`)
	}))
	defer server.Close()
	client := server.Client()
	var redirects atomic.Int32
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		redirects.Add(1)
		return nil
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/token", strings.NewReader("client_secret=fixture-secret"))
	require.NoError(t, err)
	response, err := doOAuthRequest(context.Background(), client, request)
	require.NoError(t, err)
	response.Body.Close()
	require.EqualValues(t, 1, destinationRequests.Load())
	require.EqualValues(t, 1, redirects.Load())
	require.NoError(t, client.CheckRedirect(nil, nil), "the shared client's redirect policy must not be replaced")

	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	request, err = http.NewRequest(http.MethodPost, server.URL+"/token", strings.NewReader("client_secret=fixture-secret"))
	require.NoError(t, err)
	response, err = doOAuthRequest(context.Background(), client, request)
	require.NoError(t, err)
	response.Body.Close()
	require.Equal(t, http.StatusTemporaryRedirect, response.StatusCode)
	require.EqualValues(t, 1, destinationRequests.Load())

	client.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("fixture policy rejects redirect") }
	request, err = http.NewRequest(http.MethodPost, server.URL+"/token", strings.NewReader("client_secret=fixture-secret"))
	require.NoError(t, err)
	_, err = doOAuthRequest(context.Background(), client, request)
	require.ErrorIs(t, err, errOAuthRequestFailed)
	require.EqualValues(t, 1, destinationRequests.Load())
}

func TestOAuthSameOriginRedirectsHaveFiniteLimit(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Redirect(w, r, "/loop", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return nil }
	request, err := http.NewRequest(http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	_, err = doOAuthRequest(context.Background(), client, request)
	require.ErrorIs(t, err, errOAuthRequestFailed)
	require.EqualValues(t, 10, requests.Load())
}
