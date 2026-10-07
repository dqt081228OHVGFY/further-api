package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/logger"
)

const oauthResponseBodyLimit = 1 << 20

var ErrOAuthResponseTooLarge = errors.New("OAuth provider response exceeds the allowed size")
var errOAuthResponseRead = errors.New("OAuth provider response could not be read")
var errOAuthResponseInvalid = errors.New("OAuth provider response is invalid")
var errOAuthRequestFailed = errors.New("OAuth provider request failed")

type oauthContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader oauthContextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}

// Read one byte beyond the cap, including for chunked/decompressed bodies.
// A valid JSON prefix cannot conceal an oversized trailing response.
func boundOAuthResponse(ctx context.Context, response *http.Response) error {
	if response == nil || response.Body == nil {
		return errOAuthResponseRead
	}
	original := response.Body
	defer original.Close()
	body, err := io.ReadAll(io.LimitReader(oauthContextReader{ctx, original}, oauthResponseBodyLimit+1))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return context.DeadlineExceeded
		}
		return errOAuthResponseRead
	}
	if len(body) > oauthResponseBodyLimit {
		return ErrOAuthResponseTooLarge
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	return nil
}

func doOAuthRequest(ctx context.Context, client *http.Client, request *http.Request) (*http.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// OAuth requests carry client secrets, authorization codes, or bearer
	// tokens. Go may replay POST bodies on 307/308 and retain Authorization
	// across ports/subdomains; keep every redirect at the original origin.
	// Clone the client so shared provider clients retain their own policy.
	guarded := *client
	redirectPolicy := client.CheckRedirect
	guarded.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) == 0 || len(via) >= 10 || next.URL.User != nil {
			return errOAuthRequestFailed
		}
		original := via[0].URL
		origin, originErr := common.NormalizeOrigin(original.Scheme + "://" + original.Host)
		nextOrigin, nextErr := common.NormalizeOrigin(next.URL.Scheme + "://" + next.URL.Host)
		if originErr != nil || nextErr != nil || origin != nextOrigin {
			return errOAuthRequestFailed
		}
		if redirectPolicy != nil {
			return redirectPolicy(next, via)
		}
		return nil
	}
	response, err := guarded.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, ErrOAuthResponseTooLarge) {
			return nil, ErrOAuthResponseTooLarge
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		return nil, errOAuthRequestFailed
	}
	if err := boundOAuthResponse(ctx, response); err != nil {
		return nil, err
	}
	return response, nil
}

func decodeOAuthResponse(ctx context.Context, response *http.Response, destination any) error {
	body, err := io.ReadAll(response.Body) // doOAuthRequest already bounded the complete body.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return errOAuthResponseRead
	}
	if !json.Valid(body) || common.Unmarshal(body, destination) != nil {
		return errOAuthResponseInvalid
	}
	return nil
}

func safeOAuthCause(ctx context.Context, cause error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(cause, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if errors.Is(cause, ErrOAuthResponseTooLarge) {
		return ErrOAuthResponseTooLarge
	}
	return errOAuthResponseInvalid
}

// The OIDC library's JWKS fetch uses its own client.Do/read path. Guard that
// client too; token, userinfo and key responses have the same finite bound.
type oauthBoundedTransport struct{ transport http.RoundTripper }

func (transport oauthBoundedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.transport.RoundTrip(request)
	if err != nil {
		return nil, errOAuthRequestFailed
	}
	if err := boundOAuthResponse(request.Context(), response); err != nil {
		return nil, err
	}
	return response, nil
}

func providerFailure(ctx context.Context, provider, stage, messageKey string, params map[string]any, cause error) *OAuthError {
	category, safeCause := "request_failed", error(errOAuthRequestFailed)
	switch {
	case errors.Is(cause, context.Canceled):
		category, safeCause = "request_canceled", context.Canceled
	case errors.Is(cause, context.DeadlineExceeded):
		category, safeCause = "request_timeout", context.DeadlineExceeded
	case errors.Is(cause, ErrOAuthResponseTooLarge):
		category, safeCause = "response_too_large", ErrOAuthResponseTooLarge
	case errors.Is(cause, errOAuthResponseRead):
		category, safeCause = "response_read_failed", errOAuthResponseRead
	case errors.Is(cause, errOAuthResponseInvalid):
		category, safeCause = "response_invalid", errOAuthResponseInvalid
	}
	logger.LogWarn(ctx, "OAuth request rejected: provider=%s stage=%s category=%s", provider, stage, category)
	return &OAuthError{MsgKey: messageKey, Params: params, RawError: category, Cause: safeCause}
}
