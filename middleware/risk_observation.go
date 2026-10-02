package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	riskModelContextKey              = "risk_observation_model"
	riskChannelUnavailableContextKey = "risk_observation_channel_unavailable"
	riskQuotaExhaustedContextKey     = "risk_observation_quota_exhausted"
	riskMaxBodyBytes                 = 64 << 10
)

func collectRiskObservation(c *gin.Context, occurredAt time.Time) (service.RiskObservationInput, bool) {
	if c.GetString(RouteTagKey) != "relay" || c.GetInt("id") <= 0 || !isRiskModelRequest(c) {
		return service.RiskObservationInput{}, false
	}

	input := service.RiskObservationInput{
		UserID:             c.GetInt("id"),
		TokenID:            c.GetInt("token_id"),
		OccurredAt:         occurredAt,
		Model:              c.GetString(riskModelContextKey),
		Stream:             common.GetContextKeyBool(c, constant.ContextKeyIsStream),
		PromptTokens:       common.GetContextKeyInt(c, constant.ContextKeyPromptTokens),
		StatusCode:         c.Writer.Status(),
		QuotaExhausted:     c.GetBool(riskQuotaExhaustedContextKey),
		ChannelUnavailable: c.GetBool(riskChannelUnavailableContextKey),
		RequestID:          c.GetString(common.RequestIdKey),
	}
	if input.Model == "" {
		input.Model = common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	}
	if input.Model == "" {
		input.Model = c.Param("model")
	}
	if input.StatusCode == 101 || strings.HasPrefix(c.Writer.Header().Get("Content-Type"), "text/event-stream") {
		input.Stream = true
	}
	if input.StatusCode == http.StatusPaymentRequired {
		input.QuotaExhausted = true
	}
	if input.StatusCode >= http.StatusBadRequest {
		// Retries can exhaust the remaining channels after Distribute has
		// selected the first one. Inspect the existing routing result, never
		// the response body, to capture that terminal failure as well.
		for _, event := range service.RequestPolicy(c).Events() {
			if event.Decision.Action != "stop" {
				continue
			}
			if event.ErrorCode == string(types.ErrorCodeGetChannelFailed) || event.ErrorCode == string(types.ErrorCodeModelNotFound) {
				input.ChannelUnavailable = true
			}
			if event.ErrorCode == string(types.ErrorCodeInsufficientUserQuota) {
				input.QuotaExhausted = true
			}
		}
	}

	if ip := net.ParseIP(c.ClientIP()); ip != nil {
		input.IPFingerprint = common.GenerateHMACWithKey(
			[]byte(common.CryptoSecret),
			fmt.Sprintf("risk-ip:%d:%s", input.UserID, ip.String()),
		)
	}

	if body := riskJSONBody(c); len(body) > 0 && gjson.ValidBytes(body) {
		root := gjson.ParseBytes(body)
		// Hash the complete request so different histories, system prompts,
		// tools or generation options do not become identical just because
		// their last user message is the same. Only the HMAC leaves this scope.
		input.InputSignature = common.GenerateHMACWithKey(
			[]byte(common.CryptoSecret),
			fmt.Sprintf("risk-input:%d:%s", input.UserID, body),
		)
		if input.Model == "" {
			input.Model = root.Get("model").String()
		}
		if root.Get("stream").Bool() {
			input.Stream = true
		}
		prompt := riskPromptValue(root)
		if prompt.Exists() && prompt.Raw != "" && prompt.Raw != "null" {
			input.LivenessKeywordHit = riskLivenessKeyword(riskPromptText(prompt))
		}
	}
	return input, true
}

func isRiskModelRequest(c *gin.Context) bool {
	// Model discovery, task polling and artifact downloads are ordinary SDK
	// operations, not new generation requests. Counting them would inflate
	// regularity and non-streaming scores for normal clients.
	return c.Request.Method == http.MethodPost ||
		(c.Request.Method == http.MethodGet && strings.EqualFold(c.GetHeader("Upgrade"), "websocket"))
}

// riskJSONBody only inspects existing replay storage before outer cleanup.
// Reading an untouched HTTP body after an early authentication rejection
// could block the response while the client is still uploading.
func riskJSONBody(c *gin.Context) []byte {
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if !strings.HasPrefix(contentType, "application/json") && !strings.Contains(contentType, "+json") {
		return nil
	}
	if value, exists := c.Get(common.KeyBodyStorage); exists {
		if storage, ok := value.(common.BodyStorage); ok {
			if storage.Size() > riskMaxBodyBytes || storage.IsDisk() {
				return nil
			}
			body, err := storage.Bytes()
			if err == nil {
				return body
			}
		}
	}
	if value, exists := c.Get(common.KeyRequestBody); exists {
		if body, ok := value.([]byte); ok && len(body) <= riskMaxBodyBytes {
			return body
		}
	}
	return nil
}

func riskPromptValue(root gjson.Result) gjson.Result {
	if messages := root.Get("messages"); messages.IsArray() {
		items := messages.Array()
		for i := len(items) - 1; i >= 0; i-- {
			if items[i].Get("role").String() == "user" {
				if content := items[i].Get("content"); content.Exists() {
					return content
				}
			}
		}
	}
	for _, key := range [...]string{"input", "prompt", "contents", "text"} {
		if value := root.Get(key); value.Exists() {
			return value
		}
	}
	return gjson.Result{}
}

func riskPromptText(value gjson.Result) string {
	if !value.Exists() {
		return ""
	}
	if value.Type == gjson.String {
		return value.String()
	}
	if value.IsArray() {
		items := value.Array()
		for i := len(items) - 1; i >= 0; i-- {
			if text := riskPromptText(items[i]); text != "" {
				return text
			}
		}
		return ""
	}
	for _, key := range [...]string{"content", "text", "parts", "message"} {
		if text := riskPromptText(value.Get(key)); text != "" {
			return text
		}
	}
	return ""
}

func riskLivenessKeyword(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	text = strings.Trim(text, " \t\r\n.!?。！？,，")
	switch text {
	case "ping", "pong", "hello", "hi", "test", "测试", "测活", "在吗", "可用吗", "能用吗", "are you there":
		return true
	default:
		return false
	}
}
