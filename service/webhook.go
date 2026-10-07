package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
)

type WebhookPayload struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Values    []any  `json:"values,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

func generateSignature(secret string, payload []byte) string {
	signer := hmac.New(sha256.New, []byte(secret))
	_, _ = signer.Write(payload)
	return hex.EncodeToString(signer.Sum(nil))
}

func SendWebhookNotify(target, secret string, data dto.Notify) error {
	return SendWebhookNotifyContext(context.Background(), target, secret, data)
}

func SendWebhookNotifyContext(ctx context.Context, target, secret string, data dto.Notify) error {
	payload := WebhookPayload{Type: data.Type, Title: data.Title, Content: renderNotificationContent(data), Values: data.Values, Timestamp: time.Now().Unix()}
	raw, err := common.Marshal(payload)
	if err != nil {
		return err
	}
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "further-api/1.0")
	if secret != "" {
		headers.Set("X-Webhook-Signature", generateSignature(secret, raw))
	}
	return sendNotificationRequest(ctx, target, http.MethodPost, headers, raw)
}
