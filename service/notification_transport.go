package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
)

const notificationRequestTimeout = 15 * time.Second

func renderNotificationContent(data dto.Notify) string {
	result := data.Content
	for _, value := range data.Values {
		result = strings.Replace(result, dto.ContentValueParam, fmt.Sprint(value), 1)
	}
	return result
}

// User-selected destinations use the protected fetch transport, and operator
// worker destinations use its configured relay transport. Both paths share a
// bounded request lifetime and the same target-URL validation.
func sendNotificationRequest(parent context.Context, target, method string, header http.Header, payload []byte) error {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, notificationRequestTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateSSRFProtectedFetchURL(target); err != nil {
		return fmt.Errorf("通知地址不可用：%w", err)
	}
	var response *http.Response
	var err error
	if system_setting.EnableWorker() {
		headers := make(map[string]string, len(header))
		for name := range header {
			headers[name] = header.Get(name)
		}
		response, err = DoWorkerRequestContext(ctx, &WorkerRequest{
			URL: target, Key: system_setting.WorkerValidKey, Method: method, Headers: headers, Body: payload,
		})
	} else {
		request, buildErr := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(payload))
		if buildErr != nil {
			return buildErr
		}
		request.Header = header.Clone()
		client := GetSSRFProtectedHTTPClient()
		if client == nil {
			return errors.New("通知 HTTP 服务尚未初始化。")
		}
		response, err = client.Do(request)
	}
	if err != nil {
		return fmt.Errorf("通知发送失败：%w", err)
	}
	if response == nil || response.Body == nil {
		return errors.New("通知服务未返回有效响应。")
	}
	defer response.Body.Close()
	// Bound discarded acknowledgement data while reusing ordinary connections.
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("通知服务返回状态 %d。", response.StatusCode)
	}
	return nil
}
