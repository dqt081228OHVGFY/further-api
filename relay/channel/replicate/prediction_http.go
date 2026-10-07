package replicate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

const replicateJSONLimit int64 = 4 << 20
const replicateImageLimit int64 = 32 << 20
const replicateUploadLimit int64 = 32 << 20
const replicatePollInterval = 500 * time.Millisecond
const replicatePredictionTimeout = 2 * time.Minute

func replicateContext(c *gin.Context) context.Context {
	if c != nil && c.Request != nil {
		return c.Request.Context()
	}
	return context.Background()
}
func replicateAPIBase(info *relaycommon.RelayInfo) (*url.URL, error) {
	if info == nil || !info.HasChannelMeta() {
		return nil, errors.New("Replicate 通道信息缺失")
	}
	value := strings.TrimSpace(info.ChannelBaseUrl)
	if value == "" {
		value = constant.GetChannelBaseURL(constant.ChannelTypeReplicate)
	}
	base, err := url.Parse(value)
	if err != nil || base.Host == "" || base.User != nil || (base.Scheme != "http" && base.Scheme != "https") || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("Replicate API 地址无效")
	}
	return base, nil
}
func replicateAPIURL(info *relaycommon.RelayInfo, path string) (string, error) {
	base, err := replicateAPIBase(info)
	if err != nil {
		return "", err
	}
	if path == "" {
		return base.String(), nil
	}
	if strings.Contains(path, "?") || strings.Contains(path, "#") || strings.Contains(path, "\\") {
		return "", errors.New("Replicate API 路径无效")
	}
	return base.JoinPath(strings.TrimLeft(path, "/")).String(), nil
}
func sameReplicateOrigin(base, target *url.URL) bool {
	return base.Scheme == target.Scheme && strings.EqualFold(base.Host, target.Host) && target.User == nil
}
func approvedReplicatePollURL(info *relaycommon.RelayInfo, input string) (string, error) {
	base, err := replicateAPIBase(info)
	if err != nil {
		return "", err
	}
	target, err := url.Parse(strings.TrimSpace(input))
	if err != nil {
		return "", errors.New("Replicate 轮询地址无效")
	}
	target = base.ResolveReference(target)
	if !sameReplicateOrigin(base, target) || target.Fragment != "" {
		return "", errors.New("Replicate 轮询地址须属于当前 API 通道")
	}
	return target.String(), nil
}

func replicateAuthenticatedHTTP(ctx context.Context, info *relaycommon.RelayInfo, method, target, contentType string, body io.Reader) (*http.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if info == nil || info.ApiKey == "" {
		return nil, errors.New("Replicate 密钥尚未配置")
	}
	target, err := approvedReplicatePollURL(info, target)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+info.ApiKey)
	request.Header.Set("Accept", "application/json")
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	client, err := service.GetHttpClientWithProxySettings(info.ChannelSetting.Proxy, info.ChannelSetting)
	if err != nil || client == nil {
		if err == nil {
			err = errors.New("Replicate HTTP 客户端不可用")
		}
		return nil, err
	}
	selected := *client
	// API redirects are not followed, even when a standard client would forward
	// Authorization to a subdomain. Poll destinations are independently bound.
	selected.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return selected.Do(request)
}

func readReplicateResponse(ctx context.Context, response *http.Response, limit int64) ([]byte, error) {
	if response == nil || response.Body == nil {
		return nil, errors.New("Replicate 未返回响应")
	}
	defer response.Body.Close()
	stop := context.AfterFunc(ctx, func() { _ = response.Body.Close() })
	defer stop()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, errors.New("Replicate 响应超过允许大小")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Replicate 请求失败，HTTP %d", response.StatusCode)
	}
	return raw, nil
}

func decodeReplicatePrediction(raw []byte) (PredictionResponse, error) {
	var prediction PredictionResponse
	if err := json.Unmarshal(raw, &prediction); err != nil {
		return prediction, fmt.Errorf("Replicate 预测响应格式无效: %w", err)
	}
	return prediction, nil
}

func awaitReplicatePrediction(ctx context.Context, info *relaycommon.RelayInfo, prediction PredictionResponse) (PredictionResponse, error) {
	for {
		if err := ctx.Err(); err != nil {
			return prediction, err
		}
		if prediction.Error != nil {
			return prediction, errors.New("Replicate 图片预测失败")
		}
		switch strings.ToLower(strings.TrimSpace(prediction.Status)) {
		case "succeeded":
			return prediction, nil
		case "failed", "canceled", "cancelled":
			return prediction, fmt.Errorf("Replicate 预测已结束：%s", prediction.Status)
		case "starting", "processing", "queued":
		case "":
			if prediction.Output != nil {
				return prediction, nil
			}
			return prediction, errors.New("Replicate 预测缺少状态与结果")
		default:
			return prediction, errors.New("Replicate 预测状态无效")
		}
		next := prediction.Urls.Get
		if next == "" && prediction.ID != "" {
			next, _ = replicateAPIURL(info, "/v1/predictions/"+url.PathEscape(prediction.ID))
		}
		if next == "" {
			return prediction, errors.New("Replicate 未返回轮询地址或预测编号")
		}
		target, err := approvedReplicatePollURL(info, next)
		if err != nil {
			return prediction, err
		}
		timer := time.NewTimer(replicatePollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return prediction, ctx.Err()
		case <-timer.C:
		}
		response, err := replicateAuthenticatedHTTP(ctx, info, http.MethodGet, target, "", nil)
		if err != nil {
			return prediction, err
		}
		raw, err := readReplicateResponse(ctx, response, replicateJSONLimit)
		if err != nil {
			return prediction, err
		}
		updated, err := decodeReplicatePrediction(raw)
		if err != nil {
			return prediction, err
		}
		if prediction.ID != "" && updated.ID != "" && prediction.ID != updated.ID {
			return prediction, errors.New("Replicate 轮询预测编号不一致")
		}
		if updated.ID == "" {
			updated.ID = prediction.ID
		}
		if updated.Urls.Get == "" {
			updated.Urls.Get = prediction.Urls.Get
		}
		prediction = updated
	}
}
