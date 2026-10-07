package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
)

type ssrfResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type protectedFetchDialer struct {
	resolver      ssrfResolver
	dialContext   func(ctx context.Context, network, address string) (net.Conn, error)
	getProtection func() (*common.SSRFProtection, bool, error)
}

type ssrfProtectedRoundTripper struct {
	resolver      ssrfResolver
	dialContext   func(ctx context.Context, network, address string) (net.Conn, error)
	getProtection func() (*common.SSRFProtection, bool, error)
	proxy         func(*http.Request) (*url.URL, error)

	mutex      sync.Mutex
	transports map[string]*http.Transport
}

func currentFetchProtection() (*common.SSRFProtection, bool, error) {
	fetchSetting := system_setting.GetFetchSetting()
	if !fetchSetting.EnableSSRFProtection {
		return nil, false, nil
	}

	protection, err := common.NewSSRFProtectionFromFetchSetting(
		fetchSetting.AllowPrivateIp,
		fetchSetting.DomainFilterMode,
		fetchSetting.IpFilterMode,
		fetchSetting.DomainList,
		fetchSetting.IpList,
		fetchSetting.AllowedPorts,
		fetchSetting.ApplyIPFilterForDomain,
	)
	if err != nil {
		return nil, true, err
	}
	return protection, true, nil
}

func newProtectedFetchHTTPClient() *http.Client {
	return newProtectedFetchHTTPClientWithDialer(nil, nil, nil)
}

func newProtectedFetchHTTPClientWithDialer(resolver ssrfResolver, dialContext func(ctx context.Context, network, address string) (net.Conn, error), getProtection func() (*common.SSRFProtection, bool, error)) *http.Client {
	return newProtectedFetchHTTPClientWithProxy(resolver, dialContext, getProtection, http.ProxyFromEnvironment)
}

func newProtectedFetchHTTPClientWithProxy(resolver ssrfResolver, dialContext func(ctx context.Context, network, address string) (net.Conn, error), getProtection func() (*common.SSRFProtection, bool, error), proxy func(*http.Request) (*url.URL, error)) *http.Client {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if dialContext == nil {
		netDialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		dialContext = netDialer.DialContext
	}
	if getProtection == nil {
		getProtection = currentFetchProtection
	}
	if proxy == nil {
		proxy = http.ProxyFromEnvironment
	}

	client := &http.Client{
		Transport: &ssrfProtectedRoundTripper{
			resolver:      resolver,
			dialContext:   dialContext,
			getProtection: getProtection,
			proxy:         proxy,
			transports:    make(map[string]*http.Transport),
		},
		CheckRedirect: checkProtectedFetchRedirect,
	}
	if common.RelayTimeout != 0 {
		client.Timeout = time.Duration(common.RelayTimeout) * time.Second
	}
	return client
}

func (t *ssrfProtectedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("invalid request")
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return nil, fmt.Errorf("unsupported fetch protocol")
	}
	_, err := t.resolveTarget(req.Context(), req.URL)
	if err != nil {
		return nil, err
	}

	proxyURL, err := t.proxy(req)
	if err != nil {
		return nil, err
	}
	if proxyURL == nil {
		return t.transportFor(nil).RoundTrip(req)
	}
	// Tunnel both HTTP and HTTPS to the validated IP. Forward proxies must
	// replace Host with the absolute-form authority, so forwarding to a pinned
	// IP would break virtual hosts; tunneling preserves the origin-form Host.
	return t.tunnelTransportFor(proxyURL).RoundTrip(req)
}

func fetchURLPort(target *url.URL) string {
	if port := target.Port(); port != "" {
		return port
	}
	if target.Scheme == "https" {
		return "443"
	}
	return "80"
}

func (t *ssrfProtectedRoundTripper) resolveTarget(ctx context.Context, target *url.URL) ([]net.IP, error) {
	protection, enabled, err := t.getProtection()
	if err != nil || !enabled {
		return nil, err
	}
	port, err := strconv.Atoi(fetchURLPort(target))
	if err != nil {
		return nil, fmt.Errorf("invalid fetch port")
	}
	host := target.Hostname()
	if err := protection.ValidateNetworkTarget(host, port); err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	if !protection.ApplyIPFilterForDomain {
		return nil, nil
	}
	resolved, err := t.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("DNS resolution failed for %s: %w", host, err)
	}
	ips := make([]net.IP, 0, len(resolved))
	for _, candidate := range resolved {
		if candidate.IP == nil {
			continue
		}
		if err := protection.ValidateResolvedIP(host, candidate.IP); err != nil {
			return nil, err
		}
		ips = append(ips, candidate.IP)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("DNS resolution returned no usable addresses")
	}
	return ips, nil
}

func (t *ssrfProtectedRoundTripper) tunnelTransportFor(proxyURL *url.URL) *http.Transport {
	var proxyTLSConfig *tls.Config
	if proxyURL.Scheme == "https" {
		if config := t.transportFor(proxyURL).TLSClientConfig; config != nil {
			proxyTLSConfig = config.Clone()
		}
	}
	key := "tunnel\x00" + proxyURL.String()
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if transport, exists := t.transports[key]; exists {
		return transport
	}
	proxyDialer := &protectedProxyTunnelDialer{proxyURL: proxyURL, dialContext: t.dialContext, tlsConfig: proxyTLSConfig}
	protectedDialer := &protectedFetchDialer{resolver: t.resolver, dialContext: proxyDialer.DialContext, getProtection: t.getProtection}
	transport := t.newTransport(nil)
	transport.DialContext = protectedDialer.DialContext
	t.transports[key] = transport
	return transport
}

func (t *ssrfProtectedRoundTripper) CloseIdleConnections() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	for _, transport := range t.transports {
		transport.CloseIdleConnections()
	}
}

func (t *ssrfProtectedRoundTripper) transportFor(proxyURL *url.URL) *http.Transport {
	// 只按代理地址分组：代理来自环境变量，取值有限，map 有界；
	// 目标 origin 是用户可控输入，不能作为缓存 key。
	key := "direct"
	if proxyURL != nil {
		key = proxyURL.String()
	}
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if transport, ok := t.transports[key]; ok {
		return transport
	}

	transport := t.newTransport(proxyURL)
	t.transports[key] = transport
	return transport
}

func (t *ssrfProtectedRoundTripper) newTransport(proxyURL *url.URL) *http.Transport {
	dialContext := t.dialContext
	proxyFunc := http.ProxyURL(proxyURL)
	if proxyURL == nil {
		protectedDialer := &protectedFetchDialer{
			resolver:      t.resolver,
			dialContext:   t.dialContext,
			getProtection: t.getProtection,
		}
		dialContext = protectedDialer.DialContext
		proxyFunc = nil
	}

	transport := &http.Transport{
		MaxIdleConns:           common.RelayMaxIdleConns,
		MaxIdleConnsPerHost:    common.RelayMaxIdleConnsPerHost,
		IdleConnTimeout:        time.Duration(common.RelayIdleConnTimeout) * time.Second,
		ForceAttemptHTTP2:      true,
		Proxy:                  proxyFunc,
		DialContext:            dialContext,
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  60 * time.Second,
		MaxResponseHeaderBytes: 64 << 10,
	}
	if common.TLSInsecureSkipVerify {
		transport.TLSClientConfig = common.InsecureTLSConfig
	}
	return transport
}

func (d *protectedFetchDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	protection, enabled, err := d.getProtection()
	if err != nil {
		return nil, err
	}
	if !enabled {
		return d.dialContext(ctx, network, addr)
	}

	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid dial address %s: %w", addr, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %s", portText)
	}
	if err := protection.ValidateNetworkTarget(host, port); err != nil {
		return nil, err
	}

	if ip := net.ParseIP(host); ip != nil {
		return d.dialContext(ctx, network, net.JoinHostPort(ip.String(), portText))
	}
	if !protection.ApplyIPFilterForDomain {
		return d.dialContext(ctx, network, addr)
	}

	resolved, err := d.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("DNS resolution failed for %s: %v", host, err)
	}

	var candidateIPs []net.IP
	for _, ipAddr := range resolved {
		ip := ipAddr.IP
		if ip == nil || !networkAllowsIP(network, ip) {
			continue
		}
		if err := protection.ValidateResolvedIP(host, ip); err != nil {
			return nil, err
		}
		candidateIPs = append(candidateIPs, ip)
	}

	var lastDialErr error
	for _, ip := range candidateIPs {
		conn, err := d.dialContext(ctx, network, net.JoinHostPort(ip.String(), portText))
		if err == nil {
			return conn, nil
		}
		lastDialErr = err
	}

	if lastDialErr != nil {
		return nil, lastDialErr
	}
	return nil, fmt.Errorf("DNS resolution for %s returned no usable IP addresses", host)
}

func networkAllowsIP(network string, ip net.IP) bool {
	switch network {
	case "tcp4":
		return ip.To4() != nil
	case "tcp6":
		return ip.To4() == nil
	default:
		return true
	}
}
