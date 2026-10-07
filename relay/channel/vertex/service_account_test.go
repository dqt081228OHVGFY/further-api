package vertex

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

type tokenTransport func(*http.Request) (*http.Response, error)

func (f tokenTransport) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestVertexAssertionSignsTheConfiguredAccountAndAudience(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	keyText := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}))
	assertion, err := createSignedJWT("account@example.test", strings.ReplaceAll(keyText, "\n", `\n`))
	require.NoError(t, err)
	parsed, err := jwt.Parse(assertion, func(token *jwt.Token) (any, error) {
		require.Equal(t, "RS256", token.Method.Alg())
		return &key.PublicKey, nil
	})
	require.NoError(t, err)
	require.True(t, parsed.Valid)
	claims := parsed.Claims.(jwt.MapClaims)
	require.Equal(t, "account@example.test", claims["iss"])
	require.Equal(t, googleTokenEndpoint, claims["aud"])
	_, err = createSignedJWT("account@example.test", "invalid-key")
	require.Error(t, err)
}

func TestVertexTokenExchangeHonorsCancellationExpiryAndDoesNotExposeResponseSecrets(t *testing.T) {
	client := &http.Client{Transport: tokenTransport(func(request *http.Request) (*http.Response, error) {
		require.Equal(t, "application/x-www-form-urlencoded", request.Header.Get("Content-Type"))
		require.NoError(t, request.ParseForm())
		require.Equal(t, "assertion-value", request.PostForm.Get("assertion"))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"access_token":"token-value","expires_in":120}`)), Header: make(http.Header)}, nil
	})}
	before := time.Now()
	token, err := exchangeToken(context.Background(), client, "assertion-value")
	require.NoError(t, err)
	require.Equal(t, "token-value", token.value)
	require.WithinDuration(t, before.Add(time.Minute), token.expires, 2*time.Second)
	client.Transport = tokenTransport(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(`{"error":"invalid_grant","access_token":"must-not-leak"}`)), Header: make(http.Header)}, nil
	})
	_, err = exchangeToken(context.Background(), client, "assertion-value")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "must-not-leak")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client.Transport = tokenTransport(func(request *http.Request) (*http.Response, error) { return nil, request.Context().Err() })
	_, err = exchangeToken(ctx, client, "assertion-value")
	require.ErrorIs(t, err, context.Canceled)
}

func TestVertexTokenIdentityChangesAfterCredentialRotation(t *testing.T) {
	first := Credentials{ProjectID: "project", ClientEmail: "account@example.test", PrivateKey: "key-one"}
	second := first
	second.PrivateKey = "key-two"
	require.NotEqual(t, credentialFingerprint(first, ""), credentialFingerprint(second, ""))
	require.NotEqual(t, credentialFingerprint(first, ""), credentialFingerprint(first, "http://proxy.example"))
}
