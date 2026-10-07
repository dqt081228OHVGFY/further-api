package openai

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
)

const mediaResponseLimit = 256 << 20

func readMediaResponse(c *gin.Context, response *http.Response) ([]byte, error) {
	if c == nil || c.Request == nil || response == nil || response.Body == nil {
		return nil, errors.New("media response context or body is missing")
	}
	defer service.CloseResponseBodyGracefully(response)
	ctx := c.Request.Context()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = response.Body.Close() })
	defer stop()
	body, err := io.ReadAll(io.LimitReader(response.Body, mediaResponseLimit+1))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(body) > mediaResponseLimit {
		return nil, errors.New("media response exceeds the gateway body limit")
	}
	return body, nil
}

func writeMediaResponse(c *gin.Context, response *http.Response, body []byte) error {
	if c == nil || c.Writer == nil || c.Request == nil {
		return errors.New("media response writer is missing")
	}
	if err := c.Request.Context().Err(); err != nil {
		return err
	}
	if c.Writer.Written() {
		return errors.New("media response was already committed")
	}
	for name, values := range response.Header {
		if !service.ShouldCopyUpstreamHeader(c, name, values) {
			continue
		}
		c.Writer.Header().Del(name)
		for _, value := range values {
			c.Writer.Header().Add(name, value)
		}
	}
	for _, name := range []string{"Content-Encoding", "Transfer-Encoding", "Connection"} {
		c.Writer.Header().Del(name)
	}
	c.Writer.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if c.Writer.Header().Get("Content-Type") == "" {
		c.Writer.Header().Set("Content-Type", http.DetectContentType(body))
	}
	c.Writer.WriteHeader(response.StatusCode)
	_, err := c.Writer.Write(body)
	return err
}

func audioResponseError(err error) *types.GatewayError {
	return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
}

func normalizeAudioUsage(usage *dto.Usage) {
	if usage.PromptTokens == 0 {
		usage.PromptTokens = usage.InputTokens
	}
	if usage.CompletionTokens == 0 {
		usage.CompletionTokens = usage.OutputTokens
	}
	if usage.InputTokensDetails != nil {
		usage.PromptTokensDetails = usage.InputTokensDetails.Clone()
	}
	if usage.OutputTokensDetails != nil {
		usage.CompletionTokenDetails = *usage.OutputTokensDetails
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
}

func completeAudioUsage(usage *dto.Usage, speech bool) {
	normalizeAudioUsage(usage)
	if usage.PromptTokensDetails.TextTokens+usage.PromptTokensDetails.AudioTokens == 0 {
		if speech {
			usage.PromptTokensDetails.TextTokens = usage.PromptTokens
		} else {
			usage.PromptTokensDetails.AudioTokens = usage.PromptTokens
		}
	}
	if usage.CompletionTokenDetails.TextTokens+usage.CompletionTokenDetails.AudioTokens == 0 {
		if speech {
			usage.CompletionTokenDetails.AudioTokens = usage.CompletionTokens
		} else {
			usage.CompletionTokenDetails.TextTokens = usage.CompletionTokens
		}
	}
}

func OpenaiTTSHandler(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.GatewayError) {
	if err := responseContext(c, info, response); err != nil {
		return nil, audioResponseError(err)
	}
	if c.Request == nil || c.Writer == nil {
		return nil, audioResponseError(errors.New("speech request context is missing"))
	}
	estimate := &dto.Usage{PromptTokens: info.GetEstimatePromptTokens(), TotalTokens: info.GetEstimatePromptTokens()}
	estimate.PromptTokensDetails.TextTokens = estimate.PromptTokens
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		usage := estimate
		var failure error
		reported := false
		helper.StreamScannerHandler(c, response, info, func(data string, result *helper.StreamResult) {
			var event struct {
				Usage *dto.Usage `json:"usage"`
			}
			if err := common.Unmarshal([]byte(data), &event); err != nil {
				failure = err
				result.Stop(err)
				return
			}
			if event.Usage != nil {
				completeAudioUsage(event.Usage, true)
				if service.ValidUsage(event.Usage) {
					if !reported {
						usage = &dto.Usage{}
					}
					reported = true
					usage = dto.MergeUsageNonZero(usage, event.Usage)
				}
			}
			if err := helper.StringData(c, data); err != nil {
				failure = err
				result.Stop(err)
			}
			if isOpenAIImageStreamErrorEvent([]byte(data)) {
				failure = errors.New("speech provider returned an error event")
				result.Error(failure)
			}
		})
		if failure != nil {
			return usage, audioResponseError(failure)
		}
		if err := c.Request.Context().Err(); err != nil {
			return usage, audioResponseError(err)
		}
		if info.StreamStatus != nil && info.StreamStatus.HasErrors() {
			return usage, audioResponseError(errors.New("audio stream did not complete"))
		}
		if info.StreamStatus != nil && info.StreamStatus.EndReason == relaycommon.StreamEndReasonDone {
			if err := helper.StringData(c, "[DONE]"); err != nil {
				return usage, audioResponseError(err)
			}
		}
		if !reported {
			common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
		}
		return usage, nil
	}
	body, err := readMediaResponse(c, response)
	if err != nil {
		return nil, audioResponseError(err)
	}
	if len(body) == 0 {
		return nil, audioResponseError(errors.New("speech provider returned empty audio"))
	}
	format := "mp3"
	if request, ok := info.Request.(*dto.AudioRequest); ok && request.ResponseFormat != "" {
		format = request.ResponseFormat
	}
	duration := float64(0)
	if format == "pcm" {
		duration = float64(len(body)) / 48000
	} else {
		duration, err = common.GetAudioDuration(c.Request.Context(), bytes.NewReader(body), "."+format)
	}
	completion := common.QuotaRound(math.Ceil(duration) * 1000 / 60)
	if err != nil || math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 {
		completion = common.QuotaRound(math.Ceil(float64(len(body)) / 1000))
	}
	estimate.CompletionTokens = completion
	estimate.CompletionTokenDetails.AudioTokens = completion
	estimate.TotalTokens = estimate.PromptTokens + completion
	common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
	if err := writeMediaResponse(c, response, body); err != nil {
		return estimate, audioResponseError(err)
	}
	return estimate, nil
}

func OpenaiSTTHandler(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo, _ string) (*types.GatewayError, *dto.Usage) {
	if err := responseContext(c, info, response); err != nil {
		return audioResponseError(err), nil
	}
	body, err := readMediaResponse(c, response)
	if err != nil {
		return audioResponseError(err), nil
	}
	usage := &dto.Usage{PromptTokens: info.GetEstimatePromptTokens(), TotalTokens: info.GetEstimatePromptTokens()}
	usage.PromptTokensDetails.AudioTokens = usage.PromptTokens
	var payload struct {
		Usage *dto.Usage `json:"usage"`
	}
	if common.Unmarshal(body, &payload) == nil && payload.Usage != nil {
		completeAudioUsage(payload.Usage, false)
		if service.ValidUsage(payload.Usage) {
			usage = payload.Usage
		}
	}
	if err := writeMediaResponse(c, response, body); err != nil {
		return audioResponseError(err), usage
	}
	return nil, usage
}
