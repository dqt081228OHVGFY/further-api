package jimeng

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const HexPayloadHashKey = "HexPayloadHash"

func SetPayloadHash(c *gin.Context, value any) error {
	if c == nil {
		return errors.New("Jimeng hash context is missing")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	c.Set(HexPayloadHashKey, hex.EncodeToString(digest[:]))
	return nil
}
func getPayloadHash(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return c.GetString(HexPayloadHashKey)
}
func Sign(_ *gin.Context, request *http.Request, key string) error {
	return signAt(request, key, time.Now().UTC())
}
func signingEscape(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}
func signAt(request *http.Request, key string, now time.Time) error {
	if request == nil || request.URL == nil || request.URL.Host == "" {
		return errors.New("Jimeng signing request is missing")
	}
	access, secret, valid := strings.Cut(key, "|")
	access, secret = strings.TrimSpace(access), strings.TrimSpace(secret)
	if !valid || access == "" || secret == "" || strings.Contains(secret, "|") {
		return errors.New("Jimeng credentials require access-key|secret-key")
	}
	var payload []byte
	if request.Body != nil {
		var err error
		payload, err = io.ReadAll(request.Body)
		request.Body.Close()
		if err != nil {
			return err
		}
		request.Body = io.NopCloser(bytes.NewReader(payload))
		request.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(payload)), nil }
		request.ContentLength = int64(len(payload))
	}
	digest := sha256.Sum256(payload)
	bodyHash := hex.EncodeToString(digest[:])
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	timestamp, date := now.UTC().Format("20060102T150405Z"), now.UTC().Format("20060102")
	request.Host = request.URL.Host
	request.Header.Set("Host", request.URL.Host)
	request.Header.Set("X-Date", timestamp)
	request.Header.Set("X-Content-Sha256", bodyHash)
	if request.Header.Get("Content-Type") == "" {
		request.Header.Set("Content-Type", "application/json")
	}
	query := request.URL.Query()
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		values := append([]string(nil), query[key]...)
		sort.Strings(values)
		for _, value := range values {
			pairs = append(pairs, signingEscape(key)+"="+signingEscape(value))
		}
	}
	headerNames := []string{"content-type", "host", "x-content-sha256", "x-date"}
	var headers strings.Builder
	for _, name := range headerNames {
		headers.WriteString(name + ":" + strings.TrimSpace(request.Header.Get(name)) + "\n")
	}
	path := request.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	signedHeaders := strings.Join(headerNames, ";")
	canonical := strings.Join([]string{request.Method, path, strings.Join(pairs, "&"), headers.String(), signedHeaders, bodyHash}, "\n")
	canonicalHash := sha256.Sum256([]byte(canonical))
	scope := date + "/cn-north-1/cv/request"
	toSign := "HMAC-SHA256\n" + timestamp + "\n" + scope + "\n" + hex.EncodeToString(canonicalHash[:])
	signingKey := []byte(secret)
	for _, part := range []string{date, "cn-north-1", "cv", "request"} {
		signingKey = hmacSHA256(signingKey, []byte(part))
	}
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(toSign)))
	request.Header.Set("Authorization", fmt.Sprintf("HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", access, scope, signedHeaders, signature))
	return nil
}
func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(data)
	return mac.Sum(nil)
}
