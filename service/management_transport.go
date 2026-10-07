package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Management requests use a caller deadline and bounded response ownership;
// callers project provider schemas before persisting any application state.
func ReadManagementResponseContext(parent context.Context, method, target, proxy string, headers http.Header) ([]byte, error) {
	if parent == nil {
		parent = context.Background()
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	endpoint, err := url.Parse(target)
	if err != nil || endpoint.Hostname() == "" || endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return nil, errors.New("渠道管理接口地址无效。")
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header = headers.Clone()
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	if host := request.Header.Get("Host"); host != "" {
		request.Host = host
		request.Header.Del("Host")
	}
	client, err := GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	if response == nil || response.Body == nil {
		return nil, errors.New("渠道管理接口未返回内容。")
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", response.StatusCode)
	}
	const limit = 16 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err == nil && len(body) > limit {
		err = errors.New("渠道管理接口响应超过容量限制。")
	}
	return body, err
}
