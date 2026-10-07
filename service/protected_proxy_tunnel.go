package service

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"golang.org/x/net/proxy"
)

type protectedProxyTunnelDialer struct {
	proxyURL    *url.URL
	dialContext func(context.Context, string, string) (net.Conn, error)
	tlsConfig   *tls.Config
}

func basicProxyAuthorization(username, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}

type proxyContextForwardDialer struct {
	dialContext func(context.Context, string, string) (net.Conn, error)
}

func (d proxyContextForwardDialer) Dial(network, address string) (net.Conn, error) {
	return d.dialContext(context.Background(), network, address)
}
func (d proxyContextForwardDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return d.dialContext(ctx, network, address)
}

type bufferedProxyConnection struct {
	net.Conn
	reader *bufio.Reader
}

func (connection *bufferedProxyConnection) Read(data []byte) (int, error) {
	return connection.reader.Read(data)
}

func (d *protectedProxyTunnelDialer) DialContext(parent context.Context, network, targetAddress string) (net.Conn, error) {
	if d.proxyURL.Scheme == "socks5" || d.proxyURL.Scheme == "socks5h" {
		dialer, err := proxy.FromURL(d.proxyURL, proxyContextForwardDialer{dialContext: d.dialContext})
		if err != nil {
			return nil, fmt.Errorf("configure fetch SOCKS proxy")
		}
		contextDialer, ok := dialer.(proxy.ContextDialer)
		if !ok {
			return nil, fmt.Errorf("fetch SOCKS proxy lacks cancellation support")
		}
		return contextDialer.DialContext(parent, network, targetAddress)
	}
	if d.proxyURL.Scheme != "http" && d.proxyURL.Scheme != "https" {
		return nil, fmt.Errorf("unsupported fetch proxy protocol")
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	port := d.proxyURL.Port()
	if port == "" {
		if d.proxyURL.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	connection, err := d.dialContext(ctx, "tcp", net.JoinHostPort(d.proxyURL.Hostname(), port))
	if err != nil {
		return nil, fmt.Errorf("connect fetch proxy: %w", err)
	}
	success := false
	defer func() {
		if !success {
			_ = connection.Close()
		}
	}()
	rawConnection := connection
	stop := context.AfterFunc(ctx, func() { _ = rawConnection.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}
	if d.proxyURL.Scheme == "https" {
		config := &tls.Config{ServerName: d.proxyURL.Hostname(), MinVersion: tls.VersionTLS12}
		if d.tlsConfig != nil {
			config = d.tlsConfig.Clone()
			config.ServerName = d.proxyURL.Hostname()
		} else if common.TLSInsecureSkipVerify {
			config = common.InsecureTLSConfig.Clone()
			config.ServerName = d.proxyURL.Hostname()
		}
		secure := tls.Client(connection, config)
		if err := secure.HandshakeContext(ctx); err != nil {
			return nil, fmt.Errorf("fetch proxy TLS handshake: %w", err)
		}
		connection = secure
	}
	request := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: targetAddress}, Host: targetAddress, Header: make(http.Header)}
	if credentials := d.proxyURL.User; credentials != nil {
		password, _ := credentials.Password()
		request.Header.Set("Proxy-Authorization", basicProxyAuthorization(credentials.Username(), password))
	}
	if err := request.Write(connection); err != nil {
		return nil, fmt.Errorf("write fetch proxy tunnel request: %w", err)
	}
	reader := bufio.NewReader(connection)
	headers, err := readBoundedProxyHeaders(reader)
	if err != nil {
		return nil, fmt.Errorf("read fetch proxy tunnel response: %w", err)
	}
	response, err := http.ReadResponse(bufio.NewReader(strings.NewReader(headers)), request)
	if err != nil {
		return nil, fmt.Errorf("fetch proxy returned invalid tunnel response")
	}
	if response.Body != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		_ = response.Body.Close()
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch proxy must allow CONNECT to the validated target port (HTTP %d)", response.StatusCode)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_ = connection.SetDeadline(time.Time{})
	success = true
	return &bufferedProxyConnection{Conn: connection, reader: reader}, nil
}

func readBoundedProxyHeaders(reader *bufio.Reader) (string, error) {
	var headers strings.Builder
	for {
		fragment, err := reader.ReadSlice('\n')
		if headers.Len()+len(fragment) > 64<<10 {
			return "", fmt.Errorf("fetch proxy response headers exceed limit")
		}
		headers.Write(fragment)
		if err != nil && err != bufio.ErrBufferFull {
			return "", err
		}
		if err == nil && (string(fragment) == "\r\n" || string(fragment) == "\n") {
			return headers.String(), nil
		}
	}
}
