package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/helper"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func imageResponse(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) ([]byte, *dto.Usage, *types.GatewayError) {
	if err := responseContext(c, info, response); err != nil {
		return nil, nil, chatResponseError(err)
	}
	body, err := readMediaResponse(c, response)
	if err != nil {
		return nil, nil, chatResponseError(err)
	}
	var decoded dto.SimpleResponse
	if err := common.Unmarshal(body, &decoded); err != nil || !gjson.ParseBytes(body).IsObject() {
		return nil, nil, chatResponseError(errors.New("image provider returned an invalid JSON object"))
	}
	if value := decoded.GetOpenAIError(); value != nil && (value.Type != "" || value.Message != "") {
		return nil, nil, types.WithOpenAIError(*value, response.StatusCode)
	}
	normalizeOpenAIUsage(&decoded.Usage)
	applyUsagePostProcessing(info, &decoded.Usage, body)
	info.UpdateImageCount(openaiImageResponseCount(body))
	return body, &decoded.Usage, nil
}

func OpenaiImageHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	body, usage, failure := imageResponse(c, info, response)
	if failure != nil {
		return nil, failure
	}
	if err := writeMediaResponse(c, response, body); err != nil {
		return usage, chatResponseError(err)
	}
	return usage, nil
}

func openaiImageDataHasField(item gjson.Result, name string) bool {
	value := item.Get(name)
	return value.Type == gjson.String && value.String() != ""
}

// An upstream can publish both URL and base64 representations of the same
// image in separate rows. Charge the number of images, rather than encodings.
func openaiImageResponseCount(body []byte) int64 {
	data := gjson.GetBytes(body, "data")
	if data.IsObject() {
		if openaiImageDataHasField(data, "url") || openaiImageDataHasField(data, "b64_json") {
			return 1
		}
		return 0
	}
	if !data.IsArray() {
		return 0
	}
	var urls, base64 int64
	for _, image := range data.Array() {
		if openaiImageDataHasField(image, "url") {
			urls++
		}
		if openaiImageDataHasField(image, "b64_json") {
			base64++
		}
	}
	return max(urls, base64)
}

func normalizeOpenAIUsage(usage *dto.Usage) {
	if usage == nil {
		return
	}
	if usage.InputTokens != 0 {
		usage.PromptTokens = usage.InputTokens
	}
	if usage.OutputTokens != 0 {
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

func OpenaiImageStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	if err := responseContext(c, info, response); err != nil {
		return nil, chatResponseError(err)
	}
	if c.Request == nil {
		return nil, chatResponseError(errors.New("image request context is missing"))
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return OpenaiImageHandler(c, info, response)
	}
	if !strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		return openaiImageJSONAsStreamHandler(c, info, response)
	}
	usage := &dto.Usage{}
	var latest []byte
	var completed int64
	seen := make(map[string]struct{})
	helper.StreamScannerHandler(c, response, info, func(data string, result *helper.StreamResult) {
		raw := []byte(data)
		latest = raw
		if !json.Valid(raw) {
			result.Stop(errors.New("image provider returned an invalid stream event"))
			return
		}
		if isOpenAIImageStreamErrorEvent(raw) {
			result.Error(errors.New(extractOpenAIImageStreamErrorMessage(raw)))
		}
		var event struct {
			Type  string     `json:"type"`
			Usage *dto.Usage `json:"usage"`
		}
		if err := common.Unmarshal(raw, &event); err != nil {
			result.Stop(err)
			return
		}
		if event.Usage != nil {
			normalizeOpenAIUsage(event.Usage)
			if service.ValidUsage(event.Usage) {
				usage = dto.MergeUsageNonZero(usage, event.Usage)
			}
		}
		if event.Type == "image_generation.completed" || event.Type == "image_edit.completed" {
			// Identified completion replays must not increase the user's bill twice.
			identity := gjson.GetBytes(raw, "image_id").String()
			if identity == "" {
				identity = gjson.GetBytes(raw, "id").String()
			}
			if _, duplicate := seen[identity]; identity == "" || !duplicate {
				completed++
				if identity != "" {
					seen[identity] = struct{}{}
				}
			}
		}
		if err := writeOpenaiImageStreamChunk(c, raw); err != nil {
			result.Stop(err)
		}
	})
	outcome := info.StreamStatus.OutcomeSnapshot()
	if outcome.EndReason == relaycommon.StreamEndReasonDone && c.Request.Context().Err() == nil {
		_ = writeOpenaiImageStreamDone(c)
	}
	applyUsagePostProcessing(info, usage, latest)
	finished := (outcome.EndReason == relaycommon.StreamEndReasonDone || outcome.EndReason == relaycommon.StreamEndReasonEOF) && !outcome.HasErrors
	// A client disconnect must not lower a quantity already generated upstream.
	if finished || completed > int64(info.RequestedImageCount()) {
		info.UpdateImageCount(completed)
	}
	return usage, nil
}

func writeOpenaiImageStreamChunk(c *gin.Context, body []byte) error {
	kind := strings.TrimSpace(gjson.GetBytes(body, "type").String())
	if kind == "" {
		return helper.StringData(c, string(body))
	}
	return helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: kind}, string(body))
}
func isOpenAIImageStreamErrorEvent(body []byte) bool {
	if !json.Valid(body) {
		return false
	}
	kind := strings.ToLower(gjson.GetBytes(body, "type").String())
	if kind == "error" || kind == "upstream_error" {
		return true
	}
	value := gjson.GetBytes(body, "error")
	return value.Exists() && value.Type != gjson.Null && value.Raw != "{}" && value.Raw != `""`
}
func extractOpenAIImageStreamErrorMessage(body []byte) string {
	for _, key := range []string{"error.message", "message"} {
		if value := strings.TrimSpace(gjson.GetBytes(body, key).String()); value != "" {
			return value
		}
	}
	if value := gjson.GetBytes(body, "error"); value.Type == gjson.String && value.String() != "" {
		return value.String()
	}
	return "upstream image stream returned an error event"
}

func openaiImageJSONAsStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, response *http.Response) (*dto.Usage, *types.GatewayError) {
	body, usage, failure := imageResponse(c, info, response)
	if failure != nil {
		return nil, failure
	}
	if c.Request.Context().Err() != nil {
		return usage, chatResponseError(c.Request.Context().Err())
	}
	created := gjson.GetBytes(body, "created").Int()
	if created == 0 {
		created = time.Now().Unix()
	}
	var usageJSON []byte
	if service.ValidUsage(usage) {
		var err error
		usageJSON, err = common.Marshal(usage)
		if err != nil {
			return nil, chatResponseError(err)
		}
	}
	helper.SetEventStreamHeaders(c)
	c.Status(http.StatusOK)
	info.SetFirstResponseTime()
	emitted := 0
	for _, image := range gjson.GetBytes(body, "data").Array() {
		if !openaiImageDataHasField(image, "url") && !openaiImageDataHasField(image, "b64_json") {
			continue
		}
		var payload bytes.Buffer
		payload.Grow(len(image.Raw) + len(usageJSON) + 128)
		payload.WriteString(`{"type":"image_generation.completed","created_at":`)
		payload.WriteString(strconv.FormatInt(created, 10))
		if len(usageJSON) != 0 {
			payload.WriteString(`,"usage":`)
			payload.Write(usageJSON)
		}
		for _, field := range []string{"url", "revised_prompt", "b64_json"} {
			value := image.Get(field)
			if value.Type != gjson.String || value.String() == "" {
				continue
			}
			payload.WriteString(fmt.Sprintf(",%q:", field))
			payload.WriteString(value.Raw)
		}
		payload.WriteByte('}')
		if err := writeOpenaiImageStreamChunk(c, payload.Bytes()); err != nil {
			if info.StreamStatus == nil {
				info.StreamStatus = relaycommon.NewStreamStatus()
			}
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, err)
			return usage, nil
		}
		emitted++
	}
	if err := writeOpenaiImageStreamDone(c); err != nil {
		if info.StreamStatus == nil {
			info.StreamStatus = relaycommon.NewStreamStatus()
		}
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, err)
		return usage, nil
	}
	info.ReceivedResponseCount += emitted
	if info.StreamStatus == nil {
		info.StreamStatus = relaycommon.NewStreamStatus()
	}
	info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	return usage, nil
}
func writeOpenaiImageStreamDone(c *gin.Context) error { return helper.StringData(c, "[DONE]") }
