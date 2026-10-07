package claude

import (
	"fmt"
	"mime"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
)

func MediaMessage(data, mediaType string) (dto.ClaudeMediaMessage, error) {
	normalized := strings.ToLower(strings.TrimSpace(mediaType))
	if parsed, _, err := mime.ParseMediaType(normalized); err == nil {
		normalized = parsed
	}
	if normalized == "image/jpg" {
		normalized = "image/jpeg"
	}
	message := dto.ClaudeMediaMessage{
		Source: &dto.ClaudeMessageSource{
			Type:      "base64",
			MediaType: normalized,
			Data:      data,
		},
	}
	switch normalized {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		message.Type = "image"
	case "application/pdf":
		message.Type = "document"
	default:
		return dto.ClaudeMediaMessage{}, fmt.Errorf("unsupported Anthropic media type %q", mediaType)
	}
	return message, nil
}
