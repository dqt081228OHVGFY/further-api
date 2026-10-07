package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
)

type WorkerRequest struct {
	URL     string            `json:"url"`
	Key     string            `json:"key"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    json.RawMessage   `json:"body,omitempty"`
}

var fileDownloadTimeout = 2 * time.Minute

type downloadResponseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
	once   sync.Once
}

func (body *downloadResponseBody) Close() error {
	body.once.Do(body.cancel)
	return body.ReadCloser.Close()
}

func DoWorkerRequest(input *WorkerRequest) (*http.Response, error) {
	return DoWorkerRequestContext(context.Background(), input)
}

func DoWorkerRequestContext(ctx context.Context, input *WorkerRequest) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !system_setting.EnableWorker() {
		return nil, errors.New("文件转发 Worker 尚未启用。")
	}
	if input == nil {
		return nil, errors.New("Worker 请求内容缺失。")
	}
	target, err := url.Parse(input.URL)
	if err != nil || target.Hostname() == "" {
		return nil, errors.New("Worker 目标地址无效。")
	}
	if target.Scheme != "https" && !(target.Scheme == "http" && system_setting.WorkerAllowHttpImageRequestEnabled) {
		return nil, errors.New("Worker 目标仅支持已允许的 HTTP 或 HTTPS 地址。")
	}
	if err := ValidateSSRFProtectedFetchURL(input.URL); err != nil {
		return nil, fmt.Errorf("Worker 目标地址不可用：%w", err)
	}
	endpoint := strings.TrimRight(system_setting.WorkerUrl, "/") + "/"
	destination, err := url.Parse(endpoint)
	if err != nil || destination.Hostname() == "" || (destination.Scheme != "http" && destination.Scheme != "https") {
		return nil, errors.New("Worker 服务地址无效。")
	}
	payload, err := common.Marshal(input)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "further-api/1.0")
	client := GetHttpClient()
	if client == nil {
		return nil, errors.New("Worker HTTP 服务尚未初始化。")
	}
	return client.Do(request)
}

func DoDownloadRequest(origin string, reason ...string) (*http.Response, error) {
	return DoDownloadRequestContext(context.Background(), origin, reason...)
}

func DoDownloadRequestContext(ctx context.Context, origin string, reason ...string) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, fileDownloadTimeout)
	keepLifetime := false
	defer func() {
		if !keepLifetime {
			cancel()
		}
	}()
	if err := ValidateSSRFProtectedFetchURL(origin); err != nil {
		return nil, fmt.Errorf("下载地址不可用：%w", err)
	}
	common.SysLog(fmt.Sprintf("下载文件：%s，原因：%s", common.MaskSensitiveInfo(origin), strings.Join(reason, "，")))
	var response *http.Response
	var err error
	if system_setting.EnableWorker() {
		response, err = DoWorkerRequestContext(ctx, &WorkerRequest{URL: origin, Key: system_setting.WorkerValidKey, Method: http.MethodGet})
	} else {
		request, buildErr := http.NewRequestWithContext(ctx, http.MethodGet, origin, nil)
		if buildErr != nil {
			return nil, buildErr
		}
		request.Header.Set("User-Agent", "further-api/1.0")
		client := GetSSRFProtectedHTTPClient()
		if client == nil {
			return nil, errors.New("下载 HTTP 服务尚未初始化。")
		}
		response, err = client.Do(request)
	}
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, err
	}
	if response == nil || response.Body == nil {
		return nil, errors.New("下载服务未返回有效响应。")
	}
	response.Body = &downloadResponseBody{ReadCloser: response.Body, cancel: cancel}
	keepLifetime = true
	return response, nil
}
