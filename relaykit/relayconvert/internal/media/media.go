package media

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	"context"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
)

type MediaResolver struct {
	GetBase64Data        func(c context.Context, source types.FileSource, reason ...string) (string, string, error)
	DecodeBase64FileData func(base64String string) (string, string, error)
}

var (
	mediaResolverMu sync.RWMutex
	mediaResolver   MediaResolver
)

func SetMediaResolver(resolver MediaResolver) {
	mediaResolverMu.Lock()
	defer mediaResolverMu.Unlock()

	mediaResolver = resolver
}

func ResolveBase64Data(c context.Context, source types.FileSource, reason ...string) (string, string, error) {
	if source == nil {
		return "", "", errors.New("media source is nil")
	}
	mediaResolverMu.RLock()
	resolver := mediaResolver.GetBase64Data
	mediaResolverMu.RUnlock()
	if resolver == nil {
		// Self-contained media can be converted by standalone relaykit users.
		// Remote URLs continue to require the host's downloader and limits.
		if data, mimeType, ok, err := inlineSourceData(source); ok || err != nil {
			return data, mimeType, err
		}
		return "", "", errors.New("relayconvert media resolver is not configured")
	}
	return resolver(c, source, reason...)
}

func DecodeBase64FileData(base64String string) (string, string, error) {
	mediaResolverMu.RLock()
	resolver := mediaResolver.DecodeBase64FileData
	mediaResolverMu.RUnlock()
	if resolver == nil {
		data, mimeType, ok, err := decodeInlineDataURL(base64String)
		if ok || err != nil {
			return mimeType, data, err
		}
		return "", "", errors.New("relayconvert media resolver is not configured")
	}
	return resolver(base64String)
}

func inlineSourceData(source types.FileSource) (data, mimeType string, handled bool, err error) {
	base64Source, ok := source.(*types.Base64Source)
	if !ok {
		return "", "", false, nil
	}
	raw := strings.TrimSpace(base64Source.GetRawData())
	if strings.HasPrefix(strings.ToLower(raw), "data:") {
		return decodeInlineDataURL(raw)
	}
	if raw == "" || base64Source.MimeType == "" {
		return "", "", false, nil
	}
	if _, err := decodeBase64(raw); err != nil {
		return "", "", true, fmt.Errorf("invalid inline media data: %w", err)
	}
	return raw, base64Source.MimeType, true, nil
}

func decodeInlineDataURL(value string) (data, mimeType string, handled bool, err error) {
	raw := strings.TrimSpace(value)
	if !strings.HasPrefix(strings.ToLower(raw), "data:") {
		return "", "", false, nil
	}
	comma := strings.IndexByte(raw, ',')
	if comma < 0 {
		return "", "", true, errors.New("inline data URL is missing a payload")
	}
	fields := strings.Split(raw[len("data:"):comma], ";")
	if len(fields) == 0 || strings.TrimSpace(fields[0]) == "" {
		return "", "", true, errors.New("inline data URL is missing a MIME type")
	}
	mimeType = strings.TrimSpace(fields[0])
	base64Payload := false
	for _, field := range fields[1:] {
		if strings.EqualFold(strings.TrimSpace(field), "base64") {
			base64Payload = true
			break
		}
	}
	if !base64Payload {
		return "", "", true, errors.New("inline data URL must use base64 encoding")
	}
	payload := raw[comma+1:]
	if _, err := decodeBase64(payload); err != nil {
		return "", "", true, fmt.Errorf("invalid inline data URL payload: %w", err)
	}
	return payload, mimeType, true, nil
}

func decodeBase64(value string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err == nil {
		return decoded, nil
	}
	return base64.RawStdEncoding.DecodeString(value)
}
