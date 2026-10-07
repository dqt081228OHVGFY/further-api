package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/stretchr/testify/require"
)

type sequenceFetchResolver struct {
	lookups      atomic.Int64
	rebound      bool
	reboundAfter int64
}

func (r *sequenceFetchResolver) LookupIPAddr(_ context.Context, _ string) ([]net.IPAddr, error) {
	lookup := r.lookups.Add(1)
	threshold := r.reboundAfter
	if threshold == 0 {
		threshold = 1
	}
	if r.rebound && lookup > threshold {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
}

func proxySecurityProtection() (*common.SSRFProtection, bool, error) {
	return &common.SSRFProtection{DomainFilterMode: false, IpFilterMode: false, ApplyIPFilterForDomain: true}, true, nil
}

func newProxySecurityClient(t *testing.T, proxyURL string, resolver ssrfResolver) *http.Client {
	t.Helper()
	parsed, err := url.Parse(proxyURL)
	require.NoError(t, err)
	client := newProtectedFetchHTTPClientWithProxy(resolver, nil, proxySecurityProtection, func(*http.Request) (*url.URL, error) { return parsed, nil })
	client.Timeout = 3 * time.Second
	return client
}

func TestProtectedHTTPProxyTunnelsToPinnedIPAndPreservesVirtualHost(t *testing.T) {
	resolver := &sequenceFetchResolver{rebound: true, reboundAfter: 2}
	var hits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Host != "rebind.example" || r.RequestURI != "/media" {
			w.WriteHeader(403)
			return
		}
		_, _ = w.Write([]byte("public-fixture"))
	}))
	defer target.Close()
	proxy := isolatedTunnelProxy(t, "8.8.8.8:80", target.Listener.Addr().String(), false)
	defer proxy.Close()
	client := newProxySecurityClient(t, proxy.URL, resolver)
	defer client.CloseIdleConnections()
	response, err := client.Get("http://rebind.example/media")
	require.NoError(t, err)
	received, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, "public-fixture", string(received))
	require.Equal(t, "rebind.example", response.Request.URL.Host)
	response, err = client.Get("http://rebind.example/after-dns-change")
	require.ErrorContains(t, err, "private IP address not allowed")
	require.Nil(t, response)
	require.EqualValues(t, 1, hits.Load(), "a rebound hostname must be blocked even with a cached proxy connection")
}

func TestProtectedProxyRevalidatesRedirectDestination(t *testing.T) {
	resolver := &sequenceFetchResolver{rebound: true, reboundAfter: 2}
	var hits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "http://rebind.example/private", http.StatusFound)
	}))
	defer target.Close()
	proxy := isolatedTunnelProxy(t, "8.8.8.8:80", target.Listener.Addr().String(), false)
	defer proxy.Close()
	client := newProxySecurityClient(t, proxy.URL, resolver)
	defer client.CloseIdleConnections()
	// The transport protects every request independently of redirect callbacks.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return nil }
	response, err := client.Get("http://rebind.example/public")
	require.ErrorContains(t, err, "private IP address not allowed")
	require.Nil(t, response)
	require.EqualValues(t, 1, hits.Load())
}

func isolatedTunnelProxy(t *testing.T, expectedAuthority, destination string, secure bool) *httptest.Server {
	t.Helper()
	proxy := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != expectedAuthority {
			w.WriteHeader(403)
			return
		}
		upstream, err := net.Dial("tcp", destination)
		if err != nil {
			w.WriteHeader(502)
			return
		}
		incoming, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			_ = upstream.Close()
			return
		}
		_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buffer.Flush()
		go func() { _, _ = io.Copy(upstream, incoming); _ = upstream.Close() }()
		go func() { _, _ = io.Copy(incoming, upstream); _ = incoming.Close() }()
	}))
	if secure {
		proxy.StartTLS()
	} else {
		proxy.Start()
	}
	return proxy
}

func TestProtectedHTTPFetchThroughHTTPSProxy(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "rebind.example" {
			w.WriteHeader(403)
			return
		}
		_, _ = w.Write([]byte("https-proxy-fixture"))
	}))
	defer target.Close()
	proxy := isolatedTunnelProxy(t, "8.8.8.8:80", target.Listener.Addr().String(), true)
	defer proxy.Close()
	client := newProxySecurityClient(t, proxy.URL, &sequenceFetchResolver{})
	defer client.CloseIdleConnections()
	parsed, err := url.Parse(proxy.URL)
	require.NoError(t, err)
	client.Transport.(*ssrfProtectedRoundTripper).transportFor(parsed).TLSClientConfig = proxy.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	response, err := client.Get("http://rebind.example/media")
	require.NoError(t, err)
	content, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, "https-proxy-fixture", string(content))
}

func TestProtectedHTTPProxyRejectsUnsupportedTunnelWithoutUnsafeFallback(t *testing.T) {
	var forwardCalls atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			forwardCalls.Add(1)
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer proxy.Close()
	client := newProxySecurityClient(t, proxy.URL, &sequenceFetchResolver{})
	response, err := client.Get("http://rebind.example/media")
	require.ErrorContains(t, err, "must allow CONNECT")
	require.Nil(t, response)
	require.Zero(t, forwardCalls.Load())
}

func proxyTLSCertificate(t *testing.T, hostname string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	template := &x509.Certificate{SerialNumber: big.NewInt(84020), Subject: pkix.Name{CommonName: hostname}, DNSNames: []string{hostname}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certificate, err := tls.X509KeyPair(certificatePEM, keyPEM)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(certificatePEM))
	return certificate, roots
}

func TestProtectedCONNECTPinsIPPreservesSNIAndChecksCertificate(t *testing.T) {
	certificate, roots := proxyTLSCertificate(t, "rebind.example")
	var serverName atomic.Value
	target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "rebind.example", r.Host)
		_, _ = w.Write([]byte("authenticated-tls-fixture"))
	}))
	target.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		serverName.Store(hello.ServerName)
		return nil, nil
	}}
	target.StartTLS()
	defer target.Close()
	var connectTargets []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			t.Errorf("expected CONNECT, got %s", r.Method)
			w.WriteHeader(400)
			return
		}
		connectTargets = append(connectTargets, r.Host)
		if r.Host != "8.8.8.8:443" {
			w.WriteHeader(403)
			return
		}
		upstream, err := net.Dial("tcp", target.Listener.Addr().String())
		if err != nil {
			w.WriteHeader(502)
			return
		}
		incoming, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			_ = upstream.Close()
			return
		}
		_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buffer.Flush()
		go func() { _, _ = io.Copy(upstream, incoming); _ = upstream.Close() }()
		go func() { _, _ = io.Copy(incoming, upstream); _ = incoming.Close() }()
	}))
	defer proxy.Close()
	parsedProxy, err := url.Parse(proxy.URL)
	require.NoError(t, err)
	resolver := &sequenceFetchResolver{}
	client := newProxySecurityClient(t, proxy.URL, resolver)
	transport := client.Transport.(*ssrfProtectedRoundTripper).tunnelTransportFor(parsedProxy)
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	response, err := client.Get("https://rebind.example/media")
	require.NoError(t, err)
	content, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, "authenticated-tls-fixture", string(content))
	require.Equal(t, "rebind.example", serverName.Load())
	require.Equal(t, []string{"8.8.8.8:443"}, connectTargets)
	transport.CloseIdleConnections()
	response, err = client.Get("https://wrong-name.example/media")
	require.Error(t, err)
	require.Nil(t, response)
	require.Contains(t, strings.ToLower(err.Error()), "certificate")
}

func TestProtectedCONNECTRejectsDNSRebindingBeforeContactingProxy(t *testing.T) {
	var hits atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.WriteHeader(403) }))
	defer proxy.Close()
	client := newProxySecurityClient(t, proxy.URL, &sequenceFetchResolver{rebound: true})
	response, err := client.Get("https://rebind.example/media")
	require.ErrorContains(t, err, "private IP address not allowed")
	require.Nil(t, response)
	require.Zero(t, hits.Load(), "the second DNS answer must be checked and pinned before CONNECT")
}
