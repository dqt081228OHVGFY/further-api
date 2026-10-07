package oauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

type oauthLocalTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (transport oauthLocalTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL.Scheme, clone.URL.Host = transport.target.Scheme, transport.target.Host
	return transport.base.RoundTrip(clone)
}

func oauthResponseFixture(t *testing.T, handler http.HandlerFunc) *bytes.Buffer {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	endpoint, err := url.Parse(server.URL)
	require.NoError(t, err)
	oldTransport, oldDebug := http.DefaultTransport, common.DebugEnabled
	http.DefaultTransport = oauthLocalTransport{target: endpoint, base: oldTransport}
	common.DebugEnabled = true
	common.LogWriterMu.Lock()
	oldWriter, oldErrorWriter := gin.DefaultWriter, gin.DefaultErrorWriter
	logs := new(bytes.Buffer)
	gin.DefaultWriter, gin.DefaultErrorWriter = logs, logs
	common.LogWriterMu.Unlock()
	oldOIDC, oldAddress := *system_setting.GetOIDCSettings(), system_setting.ServerAddress
	oldTelegram, oldTelegramEnabled := *system_setting.GetTelegramSettings(), common.TelegramOAuthEnabled
	*system_setting.GetOIDCSettings() = system_setting.OIDCSettings{Enabled: true, ClientId: "fixture-client", ClientSecret: "fixture-client-secret", TokenEndpoint: "https://idp.invalid/token", UserInfoEndpoint: "https://idp.invalid/user"}
	*system_setting.GetTelegramSettings() = system_setting.TelegramSettings{ClientID: "fixture-client", ClientSecret: "fixture-client-secret"}
	common.TelegramOAuthEnabled, system_setting.ServerAddress = true, "https://example.com"
	t.Setenv("LINUX_DO_TOKEN_ENDPOINT", "https://idp.invalid/token")
	t.Setenv("LINUX_DO_USER_ENDPOINT", "https://idp.invalid/user")
	oldTrustLevel := common.LinuxDOMinimumTrustLevel
	common.LinuxDOMinimumTrustLevel = 0
	t.Cleanup(func() {
		http.DefaultTransport, common.DebugEnabled = oldTransport, oldDebug
		*system_setting.GetOIDCSettings(), system_setting.ServerAddress = oldOIDC, oldAddress
		*system_setting.GetTelegramSettings(), common.TelegramOAuthEnabled = oldTelegram, oldTelegramEnabled
		common.LinuxDOMinimumTrustLevel = oldTrustLevel
		common.LogWriterMu.Lock()
		gin.DefaultWriter, gin.DefaultErrorWriter = oldWriter, oldErrorWriter
		common.LogWriterMu.Unlock()
	})
	return logs
}

type oauthResponseOperation struct {
	name   string
	valid  string
	invoke func(context.Context) error
}

func oauthResponseOperations(t *testing.T) []oauthResponseOperation {
	t.Helper()
	custom := NewGenericOAuthProvider(&model.CustomOAuthProvider{Name: "Fixture", Slug: "fixture", Enabled: true, ClientId: "fixture-client", ClientSecret: "fixture-client-secret", TokenEndpoint: "https://idp.invalid/token", UserInfoEndpoint: "https://idp.invalid/user", UserIdField: "sub", UsernameField: "name", EmailField: "email"})
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "https://example.com/oauth", nil)
	ctx.Set(TelegramOAuthFlowContextKey, &TelegramOAuthFlow{CodeVerifier: "fixture-verifier", ClientID: "fixture-client", RedirectURI: "https://example.com/oauth/telegram"})
	token := &OAuthToken{AccessToken: "fixture-access-token"}
	operations := []oauthResponseOperation{}
	for _, entry := range []struct {
		name     string
		provider Provider
		user     string
	}{
		{"custom", custom, `{"sub":"42","name":"fixture","email":"fixture@example.com"}`},
		{"github", &GitHubProvider{}, `{"id":42,"login":"fixture","name":"Fixture","email":"fixture@example.com"}`},
		{"oidc", &OIDCProvider{}, `{"sub":"42","name":"Fixture","email":"fixture@example.com","preferred_username":"fixture"}`},
		{"discord", &DiscordProvider{}, `{"id":"42","username":"fixture","global_name":"Fixture"}`},
		{"linuxdo", &LinuxDOProvider{}, `{"id":42,"username":"fixture","name":"Fixture","trust_level":1,"active":true}`},
	} {
		provider := entry.provider
		operations = append(operations,
			oauthResponseOperation{entry.name + "/token", `{"access_token":"fixture-access-token","token_type":"Bearer"}`, func(context context.Context) error {
				_, err := provider.ExchangeToken(context, "fixture-code", ctx)
				return err
			}},
			oauthResponseOperation{entry.name + "/userinfo", entry.user, func(context context.Context) error { _, err := provider.GetUserInfo(context, token); return err }},
		)
	}
	operations = append(operations, oauthResponseOperation{"github/verified-emails", `[{"email":"fixture@example.com","verified":true}]`, func(ctx context.Context) error {
		_, err := (&GitHubProvider{}).GetVerifiedEmails(ctx, token)
		return err
	}})
	operations = append(operations, oauthResponseOperation{"telegram/token", `{"id_token":"fixture-id-token-for-exchange","access_token":"fixture-access-token"}`, func(context context.Context) error {
		_, err := NewTelegramProvider(&http.Client{Timeout: time.Second}).ExchangeToken(context, "fixture-code", ctx)
		return err
	}})
	return operations
}

func TestOAuthProviderCompleteResponsesAreBoundedAndValidClaimsRemainUsable(t *testing.T) {
	for _, operation := range oauthResponseOperations(t) {
		for _, shape := range []string{"valid", "exact_limit", "one_byte_over", "chunked_over"} {
			t.Run(operation.name+"/"+shape, func(t *testing.T) {
				body := operation.valid
				if shape != "valid" {
					size := oauthResponseBodyLimit
					if shape != "exact_limit" {
						size++
					}
					body += strings.Repeat(" ", size-len(body))
				}
				logs := oauthResponseFixture(t, func(writer http.ResponseWriter, request *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					if shape == "chunked_over" {
						writer.WriteHeader(http.StatusOK)
						writer.(http.Flusher).Flush()
					}
					_, _ = io.WriteString(writer, body)
				})
				err := operation.invoke(context.Background())
				if shape == "valid" || shape == "exact_limit" {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, ErrOAuthResponseTooLarge)
				}
				require.NotContains(t, logs.String(), "fixture-access-token")
				require.NotContains(t, logs.String(), "fixture@example.com")
				if err != nil {
					require.NotContains(t, err.Error(), "fixture-access-token")
				}
			})
		}
	}
}

func TestOAuthProviderErrorsDoNotReflectBodiesClaimsOrEndpointSecrets(t *testing.T) {
	for _, operation := range oauthResponseOperations(t) {
		t.Run(operation.name, func(t *testing.T) {
			logs := oauthResponseFixture(t, func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(writer, `{"error":"invalid_grant","error_description":"TOKEN_PRIVATE_DO_NOT_ECHO","message":"EMAIL_PRIVATE_DO_NOT_ECHO","access_token":"TOKEN_PRIVATE_DO_NOT_ECHO"}`)
			})
			err := operation.invoke(context.Background())
			require.Error(t, err)
			for _, secret := range []string{"TOKEN_PRIVATE_DO_NOT_ECHO", "EMAIL_PRIVATE_DO_NOT_ECHO", "fixture-client-secret"} {
				require.NotContains(t, err.Error(), secret)
				require.NotContains(t, logs.String(), secret)
			}
		})
	}
}

func TestOAuthProviderCanceledRequestsStopBodyReads(t *testing.T) {
	for _, operation := range oauthResponseOperations(t) {
		t.Run(operation.name, func(t *testing.T) {
			started := make(chan struct{})
			oauthResponseFixture(t, func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(http.StatusOK)
				writer.(http.Flusher).Flush()
				close(started)
				<-request.Context().Done()
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() { <-started; cancel() }()
			err := operation.invoke(ctx)
			require.ErrorIs(t, err, context.Canceled)
		})
	}
}

func TestOAuthCustomFormTokenCompatibilityAndSafeRefusals(t *testing.T) {
	for _, body := range []string{"access_token=fixture-token&token_type=Bearer&scope=profile", "error=invalid_grant&error_description=TOKEN_PRIVATE_DO_NOT_ECHO&access_token=untrusted-token"} {
		t.Run(body[:12], func(t *testing.T) {
			logs := oauthResponseFixture(t, func(writer http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(writer, body) })
			provider := NewGenericOAuthProvider(&model.CustomOAuthProvider{Name: "Fixture", Slug: "fixture", TokenEndpoint: "https://idp.invalid/token"})
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			token, err := provider.ExchangeToken(context.Background(), "fixture-code", ctx)
			if strings.HasPrefix(body, "access_token") {
				require.NoError(t, err)
				require.Equal(t, "fixture-token", token.AccessToken)
			} else {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "TOKEN_PRIVATE_DO_NOT_ECHO")
			}
			require.NotContains(t, logs.String(), "TOKEN_PRIVATE_DO_NOT_ECHO")
		})
	}
}

func TestTelegramJWKSResponseCannotBypassTheBodyLimit(t *testing.T) {
	oauthResponseFixture(t, func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, `{"keys":[]}`+strings.Repeat(" ", oauthResponseBodyLimit))
	})
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	claims := jwt.MapClaims{"iss": TelegramIssuer, "sub": "fixture-subject", "aud": "fixture-client", "exp": time.Now().Add(time.Minute).Unix(), "id": 42}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	require.NoError(t, err)
	_, err = NewTelegramProvider(&http.Client{Timeout: time.Second}).GetUserInfo(context.Background(), &OAuthToken{IDToken: signed, ClientID: "fixture-client"})
	require.ErrorIs(t, err, ErrTelegramOAuthFailed)
	require.NotContains(t, err.Error(), signed)
	// The OIDC verifier may wrap its remote-key error text, but never succeeds
	// on the finite response failure or exposes raw key-response bytes.
	require.False(t, errors.Is(err, context.Canceled))
}
