package vertex

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/golang-jwt/jwt/v5"
)

const googleTokenEndpoint = "https://www.googleapis.com/oauth2/v4/token"

type Credentials struct {
	ProjectID    string `json:"project_id"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	ClientEmail  string `json:"client_email"`
	ClientID     string `json:"client_id"`
}

type cachedToken struct {
	value   string
	expires time.Time
}
type tokenFlight struct {
	done  chan struct{}
	token cachedToken
	err   error
}

var tokens = struct {
	sync.Mutex
	values  map[string]cachedToken
	flights map[string]*tokenFlight
}{values: make(map[string]cachedToken), flights: make(map[string]*tokenFlight)}

func credentialFingerprint(credentials Credentials, proxy string) string {
	payload, _ := json.Marshal(struct {
		Credentials Credentials
		Proxy       string
	}{credentials, proxy})
	return fmt.Sprintf("%x", sha256.Sum256(payload))
}

func getAccessToken(a *Adaptor, info *relaycommon.RelayInfo) (string, error) {
	return getAccessTokenWithContext(context.Background(), a, info)
}
func getAccessTokenWithContext(ctx context.Context, a *Adaptor, info *relaycommon.RelayInfo) (string, error) {
	if a == nil || info == nil || !info.HasChannelMeta() {
		return "", errors.New("Vertex authentication configuration is missing")
	}
	client, err := service.GetHttpClientWithProxySettings(info.ChannelSetting.Proxy, info.ChannelSetting)
	if err != nil {
		return "", err
	}
	key := credentialFingerprint(a.AccountCredentials, info.ChannelSetting.Proxy)
	tokens.Lock()
	now := time.Now()
	for cacheKey, token := range tokens.values {
		if !now.Before(token.expires) {
			delete(tokens.values, cacheKey)
		}
	}
	if existing, ok := tokens.values[key]; ok && time.Now().Before(existing.expires) {
		tokens.Unlock()
		return existing.value, nil
	}
	if flight, ok := tokens.flights[key]; ok {
		tokens.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-flight.done:
			return flight.token.value, flight.err
		}
	}
	flight := &tokenFlight{done: make(chan struct{})}
	tokens.flights[key] = flight
	tokens.Unlock()
	signed, err := createSignedJWT(a.AccountCredentials.ClientEmail, a.AccountCredentials.PrivateKey)
	var token cachedToken
	if err == nil {
		token, err = exchangeToken(ctx, client, signed)
	}
	tokens.Lock()
	flight.token, flight.err = token, err
	if err == nil {
		tokens.values[key] = token
	}
	delete(tokens.flights, key)
	close(flight.done)
	tokens.Unlock()
	return token.value, err
}

func createSignedJWT(email, keyPEM string) (string, error) {
	if strings.TrimSpace(email) == "" {
		return "", errors.New("Vertex service account email is missing")
	}
	keyPEM = strings.ReplaceAll(keyPEM, `\n`, "\n")
	block, _ := pem.Decode([]byte(strings.TrimSpace(keyPEM)))
	if block == nil {
		return "", errors.New("Vertex service account RSA key is not valid PEM")
	}
	var key *rsa.PrivateKey
	if block.Type == "RSA PRIVATE KEY" {
		parsed, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return "", err
		}
		key = parsed
	} else {
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return "", err
		}
		var ok bool
		key, ok = parsed.(*rsa.PrivateKey)
		if !ok {
			return "", errors.New("Vertex service account requires an RSA private key")
		}
	}
	now := time.Now()
	assertion := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": email, "scope": "https://www.googleapis.com/auth/cloud-platform", "aud": googleTokenEndpoint, "iat": now.Unix(), "exp": now.Add(35 * time.Minute).Unix()})
	return assertion.SignedString(key)
}

func exchangeToken(ctx context.Context, client *http.Client, signed string) (cachedToken, error) {
	values := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {signed}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return cachedToken{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return cachedToken{}, err
	}
	defer response.Body.Close()
	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
		Error       string `json:"error"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&body); err != nil {
		return cachedToken{}, fmt.Errorf("Vertex OAuth response is not valid JSON: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || body.AccessToken == "" {
		return cachedToken{}, fmt.Errorf("Vertex OAuth token exchange failed (HTTP %d, code %s)", response.StatusCode, body.Error)
	}
	lifetime := 30 * time.Minute
	if body.ExpiresIn > 0 {
		lifetime = time.Duration(body.ExpiresIn)*time.Second - time.Minute
		if lifetime < 0 {
			lifetime = 0
		}
	}
	return cachedToken{value: body.AccessToken, expires: time.Now().Add(lifetime)}, nil
}

func exchangeJwtForAccessToken(signed string, info *relaycommon.RelayInfo) (string, error) {
	if info == nil || !info.HasChannelMeta() {
		return "", errors.New("Vertex provider configuration is missing")
	}
	client, err := service.GetHttpClientWithProxySettings(info.ChannelSetting.Proxy, info.ChannelSetting)
	if err != nil {
		return "", err
	}
	token, err := exchangeToken(context.Background(), client, signed)
	return token.value, err
}

func AcquireAccessToken(credentials Credentials, proxy string) (string, error) {
	signed, err := createSignedJWT(credentials.ClientEmail, credentials.PrivateKey)
	if err != nil {
		return "", err
	}
	return exchangeJwtForAccessTokenWithProxy(signed, proxy)
}
func exchangeJwtForAccessTokenWithProxy(signed, proxy string) (string, error) {
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return "", err
	}
	token, err := exchangeToken(context.Background(), client, signed)
	return token.value, err
}
