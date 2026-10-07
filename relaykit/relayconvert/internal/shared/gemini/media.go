package gemini

import (
	"context"
	"fmt"
	"mime"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	relaymedia "github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert/internal/media"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
)

func ResolveMediaPart(ctx context.Context, source types.FileSource, reason string) (dto.GeminiPart, error) {
	if source == nil {
		return dto.GeminiPart{}, fmt.Errorf("Gemini media source is nil")
	}
	data, mimeType, err := relaymedia.ResolveBase64Data(ctx, source, reason)
	if err != nil {
		return dto.GeminiPart{}, fmt.Errorf("get file data from '%s' failed: %w", source.GetIdentifier(), err)
	}
	return inlineDataPart(mimeType, data, source.GetIdentifier())
}

func inlineDataPart(mimeType, data, identifier string) (dto.GeminiPart, error) {
	normalizedMimeType := strings.ToLower(strings.TrimSpace(mimeType))
	if parsed, _, err := mime.ParseMediaType(normalizedMimeType); err == nil {
		normalizedMimeType = parsed
	}
	if !SupportedMimeTypes[normalizedMimeType] {
		return dto.GeminiPart{}, fmt.Errorf("mime type is not supported by Gemini: '%s', url: '%s', supported types are: %v", mimeType, identifier, SupportedMimeTypesList())
	}
	return dto.GeminiPart{InlineData: &dto.GeminiInlineData{MimeType: normalizedMimeType, Data: data}}, nil
}

// TextAndDataImageParts preserves every text span around embedded Markdown
// data images while turning those images into Gemini inlineData parts.
func TextAndDataImageParts(text string) ([]dto.GeminiPart, error) {
	if text == "" {
		return nil, nil
	}
	parts := make([]dto.GeminiPart, 0, 1)
	appendText := func(value string) {
		if value != "" {
			parts = append(parts, dto.GeminiPart{Text: value})
		}
	}
	consumed, search := 0, 0
	for search < len(text) {
		startOffset := strings.Index(text[search:], "![")
		if startOffset < 0 {
			break
		}
		start := search + startOffset
		altEndOffset := strings.IndexByte(text[start+2:], ']')
		if altEndOffset < 0 {
			search = start + 2
			continue
		}
		altEnd := start + 2 + altEndOffset
		urlStart := altEnd + 2
		if urlStart > len(text) || text[altEnd+1] != '(' || !strings.HasPrefix(text[urlStart:], "data:") {
			search = start + 2
			continue
		}
		urlEndOffset := strings.IndexByte(text[urlStart:], ')')
		if urlEndOffset < 0 {
			search = start + 2
			continue
		}
		urlEnd := urlStart + urlEndOffset
		dataURL := text[urlStart:urlEnd]
		mimeType, data, err := relaymedia.DecodeBase64FileData(dataURL)
		if err != nil {
			return nil, fmt.Errorf("decode markdown base64 image data failed: %w", err)
		}
		imagePart, err := inlineDataPart(mimeType, data, dataURL)
		if err != nil {
			return nil, err
		}
		appendText(text[consumed:start])
		parts = append(parts, imagePart)
		consumed = urlEnd + 1
		search = consumed
	}
	appendText(text[consumed:])
	return parts, nil
}
