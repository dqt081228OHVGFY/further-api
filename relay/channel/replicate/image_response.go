package replicate

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

func replicateImageError(err error) *types.GatewayError {
	return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
}
func replicateOutputURLs(output any) ([]string, error) {
	values := []string{}
	appendValue := func(value string) error {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return errors.New("Replicate 图片结果地址无效")
		}
		values = append(values, value)
		if len(values) > dto.MaxImageN {
			return errors.New("Replicate 图片结果数量超过允许范围")
		}
		return nil
	}
	switch output := output.(type) {
	case string:
		if err := appendValue(output); err != nil {
			return nil, err
		}
	case []any:
		for _, item := range output {
			value, valid := item.(string)
			if !valid {
				return nil, errors.New("Replicate 图片结果类型无效")
			}
			if err := appendValue(value); err != nil {
				return nil, err
			}
		}
	case []string:
		for _, value := range output {
			if err := appendValue(value); err != nil {
				return nil, err
			}
		}
	default:
		return nil, errors.New("Replicate 未返回图片结果")
	}
	if len(values) == 0 {
		return nil, errors.New("Replicate 未返回可用图片")
	}
	return values, nil
}

func downloadReplicateImageContext(ctx context.Context, origin string) (string, error) {
	response, err := service.DoDownloadRequestContext(ctx, origin, "Replicate 图片结果")
	if err != nil {
		return "", err
	}
	raw, err := readReplicateResponse(ctx, response, replicateImageLimit)
	if err != nil {
		return "", err
	}
	if len(raw) == 0 {
		return "", errors.New("Replicate 图片下载内容为空")
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

func finishReplicateImage(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (any, *types.GatewayError) {
	if c == nil || c.Request == nil || info == nil || !info.HasChannelMeta() {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, replicateImageError(errors.New("Replicate 图片响应上下文缺失"))
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), replicatePredictionTimeout)
	defer cancel()
	raw, err := readReplicateResponse(ctx, response, replicateJSONLimit)
	if err != nil {
		return nil, replicateImageError(err)
	}
	prediction, err := decodeReplicatePrediction(raw)
	if err != nil {
		return nil, replicateImageError(err)
	}
	prediction, err = awaitReplicatePrediction(ctx, info, prediction)
	if err != nil {
		return nil, replicateImageError(err)
	}
	images, err := replicateOutputURLs(prediction.Output)
	if err != nil {
		return nil, replicateImageError(err)
	}
	wantsBase64 := false
	if request, valid := info.Request.(*dto.ImageRequest); valid {
		wantsBase64 = strings.EqualFold(request.ResponseFormat, "b64_json")
	}
	output := dto.ImageResponse{Created: common.GetTimestamp(), Data: make([]dto.ImageData, 0, len(images))}
	totalBase64 := 0
	for _, origin := range images {
		if err := ctx.Err(); err != nil {
			return nil, replicateImageError(err)
		}
		item := dto.ImageData{Url: origin}
		if wantsBase64 {
			encoded, err := downloadReplicateImageContext(ctx, origin)
			if err != nil {
				return nil, replicateImageError(err)
			}
			totalBase64 += len(encoded)
			if totalBase64 > 128<<20 {
				return nil, replicateImageError(errors.New("Replicate 图片总内容超过允许大小"))
			}
			item = dto.ImageData{B64Json: encoded}
		}
		output.Data = append(output.Data, item)
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return nil, replicateImageError(err)
	}
	if err := ctx.Err(); err != nil {
		return nil, replicateImageError(err)
	}
	info.UpdateImageCount(int64(len(output.Data)))
	c.Header("Content-Type", "application/json")
	c.Status(http.StatusOK)
	written, err := c.Writer.Write(encoded)
	if err != nil {
		return nil, replicateImageError(err)
	}
	if written != len(encoded) {
		return nil, replicateImageError(fmt.Errorf("Replicate 图片响应写入不完整"))
	}
	common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
	return &dto.Usage{}, nil
}
