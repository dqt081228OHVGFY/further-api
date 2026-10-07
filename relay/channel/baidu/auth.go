package baidu

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/service"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type baiduTokenFlight struct {
	done  chan struct{}
	token string
	err   error
}

var baiduTokens = struct {
	sync.Mutex
	values  map[[32]byte]BaiduAccessToken
	flights map[[32]byte]*baiduTokenFlight
}{values: make(map[[32]byte]BaiduAccessToken), flights: make(map[[32]byte]*baiduTokenFlight)}

func getBaiduAccessToken(key string) (string, error) {
	return getBaiduAccessTokenContext(common.BackgroundContext(), key)
}
func getBaiduAccessTokenContext(ctx context.Context, key string) (string, error) {
	if ctx == nil {
		ctx = common.BackgroundContext()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	identity := sha256.Sum256([]byte(key))
	now := time.Now()
	baiduTokens.Lock()
	for id, token := range baiduTokens.values {
		if !now.Before(token.ExpiresAt) {
			delete(baiduTokens.values, id)
		}
	}
	if token, ok := baiduTokens.values[identity]; ok {
		baiduTokens.Unlock()
		return token.AccessToken, nil
	}
	if flight, ok := baiduTokens.flights[identity]; ok {
		baiduTokens.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-flight.done:
			return flight.token, flight.err
		}
	}
	flight := &baiduTokenFlight{done: make(chan struct{})}
	baiduTokens.flights[identity] = flight
	baiduTokens.Unlock()
	token, err := exchangeBaiduToken(ctx, key)
	baiduTokens.Lock()
	if err == nil {
		baiduTokens.values[identity] = *token
		flight.token = token.AccessToken
	}
	flight.err = err
	delete(baiduTokens.flights, identity)
	close(flight.done)
	baiduTokens.Unlock()
	return flight.token, err
}
func getBaiduAccessTokenHelper(key string) (*BaiduAccessToken, error) {
	return exchangeBaiduToken(common.BackgroundContext(), key)
}
func exchangeBaiduToken(parent context.Context, key string) (*BaiduAccessToken, error) {
	clientID, secret, valid := strings.Cut(key, "|")
	if !valid || clientID == "" || secret == "" || strings.Contains(secret, "|") {
		return nil, errors.New("Baidu credentials require client-id|client-secret")
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {clientID}, "client_secret": {secret}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://aip.baidubce.com/oauth/2.0/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := service.GetHttpClient().Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var token BaiduAccessToken
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token); err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK || token.Error != "" || token.AccessToken == "" {
		return nil, fmt.Errorf("Baidu token exchange failed (HTTP %d, code %s)", response.StatusCode, token.Error)
	}
	lifetime := time.Duration(token.ExpiresIn)*time.Second - time.Minute
	if lifetime < 0 {
		lifetime = 0
	}
	token.ExpiresAt = time.Now().Add(lifetime)
	return &token, nil
}
