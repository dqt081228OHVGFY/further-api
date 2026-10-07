package replicate

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

var replicateModelSegment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

func prepareReplicateImage(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if err := replicateContext(c).Err(); err != nil {
		return nil, err
	}
	if info == nil || !info.HasChannelMeta() {
		return nil, errors.New("Replicate 通道信息缺失")
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" && c != nil {
		prompt = strings.TrimSpace(c.PostForm("prompt"))
	}
	if prompt == "" {
		return nil, errors.New("请提供图片提示词")
	}
	model := strings.TrimSpace(info.UpstreamModelName)
	if model == "" {
		model = strings.TrimSpace(request.Model)
	}
	if model == "" {
		model = ModelFlux11Pro
	}
	segments := strings.Split(model, "/")
	if len(segments) != 2 || !replicateModelSegment.MatchString(segments[0]) || !replicateModelSegment.MatchString(segments[1]) {
		return nil, errors.New("Replicate 模型须为有效的 owner/model 编号")
	}
	payload := map[string]any{"prompt": prompt}
	if size := strings.TrimSpace(request.Size); size != "" && !strings.EqualFold(size, "auto") {
		aspect, width, height, valid := mapOpenAISizeToFlux(size)
		if !valid {
			return nil, errors.New("图片尺寸无效")
		}
		payload["aspect_ratio"] = aspect
		if aspect == "custom" {
			payload["width"], payload["height"] = width, height
		}
	}
	if len(request.OutputFormat) > 0 {
		var format string
		if err := json.Unmarshal(request.OutputFormat, &format); err != nil {
			return nil, errors.New("图片输出格式无效")
		}
		if format != "" {
			payload["output_format"] = format
		}
	}
	if request.N != nil && *request.N > 0 {
		payload["num_outputs"] = *request.N
	}
	if strings.EqualFold(request.Quality, "hd") || strings.EqualFold(request.Quality, "high") {
		payload["prompt_upsampling"] = true
	}
	if len(request.ExtraFields) > 0 {
		var fields map[string]any
		if err := json.Unmarshal(request.ExtraFields, &fields); err != nil {
			return nil, fmt.Errorf("图片附加参数格式无效: %w", err)
		}
		for key, value := range fields {
			payload[key] = value
		}
	}
	for key, raw := range request.Extra {
		if raw == nil {
			continue
		}
		if strings.EqualFold(key, "input") {
			var fields map[string]any
			if err := json.Unmarshal(raw, &fields); err != nil {
				return nil, fmt.Errorf("图片 input 参数格式无效: %w", err)
			}
			for name, value := range fields {
				payload[name] = value
			}
			continue
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, fmt.Errorf("图片参数 %s 格式无效: %w", key, err)
		}
		payload[key] = value
	}
	if info.RelayMode == relayconstant.RelayModeImagesEdits {
		image, err := uploadReplicateImage(c, info)
		if err != nil {
			return nil, err
		}
		if image == "" {
			return nil, errors.New("图片编辑请求缺少 image 文件")
		}
		payload["image_prompt"] = image
	}
	info.UpstreamModelName = model
	info.RequestURLPath = "/v1/models/" + model + "/predictions"
	return map[string]any{"input": payload}, nil
}
