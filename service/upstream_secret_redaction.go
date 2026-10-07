package service

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"

	taskdto "github.com/dqt081228OHVGFY/further-api/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
)

func secretFieldName(name string) bool {
	name = strings.ToLower(name)
	return strings.Contains(name, "key") || strings.Contains(name, "token") || strings.Contains(name, "secret") || strings.Contains(name, "auth") || strings.Contains(name, "password") || strings.Contains(name, "signature") || strings.Contains(name, "credential")
}

// Credentials have arbitrary formats. Match values actually sent upstream
// instead of relying on provider-specific key prefixes or error wording.
func UpstreamRequestSecrets(request *http.Request) []string {
	if request == nil {
		return nil
	}
	var secrets []string
	for name, values := range request.Header {
		if !secretFieldName(name) {
			continue
		}
		for _, value := range values {
			secrets = append(secrets, value)
			if strings.EqualFold(name, "Authorization") {
				if _, token, present := strings.Cut(value, " "); present {
					secrets = append(secrets, token)
				}
			}
		}
	}
	if _, password, present := request.BasicAuth(); present {
		secrets = append(secrets, password)
	}
	if request.URL != nil {
		for name, values := range request.URL.Query() {
			if secretFieldName(name) {
				secrets = append(secrets, values...)
			}
		}
		if request.URL.User != nil {
			if password, present := request.URL.User.Password(); present {
				secrets = append(secrets, password)
			}
		}
	}
	return secrets
}

func UpstreamCredentialSecrets(raw string) []string {
	if raw == "" {
		return nil
	}
	secrets := []string{raw}
	if parsed, err := url.Parse(raw); err == nil && parsed.User != nil {
		if password, present := parsed.User.Password(); present {
			secrets = append(secrets, password)
		}
	}
	var value any
	if len(raw) <= 256<<10 && json.Unmarshal([]byte(raw), &value) == nil {
		var visit func(any)
		visit = func(node any) {
			switch typed := node.(type) {
			case map[string]any:
				for name, child := range typed {
					if text, ok := child.(string); ok && secretFieldName(name) {
						secrets = append(secrets, text)
					}
					visit(child)
				}
			case []any:
				for _, child := range typed {
					visit(child)
				}
			}
		}
		visit(value)
	}
	return secrets
}

func RedactKnownUpstreamSecrets(content string, secrets ...string) string {
	if content == "" {
		return content
	}
	unique := make(map[string]struct{})
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		for _, variant := range []string{secret, url.QueryEscape(secret), url.PathEscape(secret)} {
			unique[variant] = struct{}{}
		}
		if encoded, err := json.Marshal(secret); err == nil && len(encoded) >= 2 {
			unique[string(encoded[1:len(encoded)-1])] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(unique))
	for secret := range unique {
		ordered = append(ordered, secret)
	}
	sort.Slice(ordered, func(i, j int) bool { return len(ordered[i]) > len(ordered[j]) })
	for _, secret := range ordered {
		content = strings.ReplaceAll(content, secret, "[REDACTED]")
	}
	return content
}

type redactedUpstreamCause struct {
	cause   error
	message string
}

func (err *redactedUpstreamCause) Error() string { return err.message }
func (err *redactedUpstreamCause) Unwrap() error { return err.cause }

func RedactUpstreamGatewayError(failure *types.GatewayError, secrets ...string) *types.GatewayError {
	if failure == nil {
		return nil
	}
	failure.SetMessage(RedactKnownUpstreamSecrets(failure.Error(), secrets...))
	if failure.Err != nil {
		message := RedactKnownUpstreamSecrets(failure.Err.Error(), secrets...)
		if message != failure.Err.Error() {
			failure.Err = &redactedUpstreamCause{cause: failure.Err, message: message}
		}
	}
	failure.Metadata = redactUpstreamJSON(failure.Metadata, secrets)
	switch wire := failure.RelayError.(type) {
	case types.OpenAIError:
		encoded, err := json.Marshal(wire)
		if err == nil && json.Unmarshal(redactUpstreamJSON(encoded, secrets), &wire) == nil {
			failure.RelayError = wire
		} else {
			failure.RelayError = nil
		}
	case types.ClaudeError:
		wire.Message = RedactKnownUpstreamSecrets(wire.Message, secrets...)
		failure.RelayError = wire
	}
	return failure
}

func redactUpstreamJSON(data json.RawMessage, secrets []string) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	redacted := []byte(RedactKnownUpstreamSecrets(string(data), secrets...))
	if !json.Valid(redacted) {
		return nil
	}
	return redacted
}

func RedactUpstreamTaskError(failure *taskdto.TaskError, secrets ...string) *taskdto.TaskError {
	if failure == nil {
		return nil
	}
	failure.Message = RedactKnownUpstreamSecrets(failure.Message, secrets...)
	if failure.Error != nil {
		message := RedactKnownUpstreamSecrets(failure.Error.Error(), secrets...)
		if message != failure.Error.Error() {
			failure.Error = &redactedUpstreamCause{cause: failure.Error, message: message}
		}
	}
	if failure.Data != nil {
		encoded, err := json.Marshal(failure.Data)
		if err != nil || json.Unmarshal(redactUpstreamJSON(encoded, secrets), &failure.Data) != nil {
			failure.Data = nil
		}
	}
	return failure
}
