package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
)

type catalogSnapshot struct {
	ETag string
	Body []byte
}

var catalogCache = struct {
	sync.RWMutex
	entries map[string]catalogSnapshot
}{entries: make(map[string]catalogSnapshot)}

var catalogHTTPClient = func() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 10 * time.Second
	transport.MaxIdleConnsPerHost = 4
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}
}()

// decodeCatalog accepts a plain JSON array or an explicit success envelope.
// An empty object, null or failure envelope is never accepted as a catalog.
func decodeCatalog[T any](body []byte, out *upstreamEnvelope[T]) error {
	if out == nil {
		return errors.New("catalog destination is nil")
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return errors.New("模型资料源返回了空内容")
	}
	var result upstreamEnvelope[T]
	if body[0] == '[' {
		if err := json.Unmarshal(body, &result.Data); err != nil {
			return fmt.Errorf("解析模型资料: %w", err)
		}
		result.Success = true
	} else {
		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Errorf("解析模型资料: %w", err)
		}
		if !result.Success || result.Data == nil {
			return errors.New("模型资料源返回了无效目录或失败响应")
		}
	}
	*out = result
	return nil
}

func fetchJSON[T any](ctx context.Context, endpoint string, out *upstreamEnvelope[T]) error {
	attempts := min(max(common.GetEnvOrDefault("SYNC_HTTP_RETRY", 3), 1), 5)
	maxBytes := int64(min(max(common.GetEnvOrDefault("SYNC_HTTP_MAX_MB", 10), 1), 32)) << 20
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if attempt > 0 {
			timer := time.NewTimer(time.Duration(100*(1<<(attempt-1))) * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "further-api/metadata")
		catalogCache.RLock()
		cached := catalogCache.entries[endpoint]
		catalogCache.RUnlock()
		if cached.ETag != "" {
			req.Header.Set("If-None-Match", cached.ETag)
		}
		response, err := catalogHTTPClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if response.StatusCode == http.StatusNotModified {
			response.Body.Close()
			if len(cached.Body) == 0 {
				return errors.New("模型资料源返回 304，但本地没有有效缓存")
			}
			return decodeCatalog(cached.Body, out)
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			lastErr = fmt.Errorf("模型资料源返回 HTTP %d", response.StatusCode)
			if response.StatusCode != http.StatusTooManyRequests && response.StatusCode < 500 {
				return lastErr
			}
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
		response.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if int64(len(body)) > maxBytes {
			return errors.New("模型资料超过允许大小")
		}
		if err := decodeCatalog(body, out); err != nil {
			return err
		}
		// Commit only parsed content. Keep the cache bounded and remove stale
		// validators when a source stops sending ETag.
		catalogCache.Lock()
		if len(catalogCache.entries) >= 32 {
			clear(catalogCache.entries)
		}
		catalogCache.entries[endpoint] = catalogSnapshot{ETag: response.Header.Get("ETag"), Body: body}
		catalogCache.Unlock()
		return nil
	}
	return lastErr
}
