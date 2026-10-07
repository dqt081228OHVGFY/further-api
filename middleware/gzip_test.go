package middleware

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func gzipRequestBytes(t *testing.T, plain []byte) []byte {
	t.Helper()
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write(plain)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return compressed.Bytes()
}

func TestRequestDecoderHandlesLayeredEncodingAndReplacesCompressedLength(t *testing.T) {
	plain := []byte(`{"model":"test","input":"完整请求"}`)
	var compressed bytes.Buffer
	outer := brotli.NewWriter(&compressed)
	_, err := outer.Write(gzipRequestBytes(t, plain))
	require.NoError(t, err)
	require.NoError(t, outer.Close())
	engine := gin.New()
	engine.Use(DecompressRequestMiddleware())
	engine.POST("/test", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.Equal(t, plain, body)
		require.EqualValues(t, -1, c.Request.ContentLength)
		require.Empty(t, c.GetHeader("Content-Encoding"))
		require.Empty(t, c.GetHeader("Content-Length"))
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(compressed.Bytes()))
	req.Header.Set("Content-Encoding", "GZip, BR")
	req.Header.Set("Content-Length", "123")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestRequestDecoderEnforcesExpandedBodyLimit(t *testing.T) {
	old := constant.MaxRequestBodyMB
	constant.MaxRequestBodyMB = 1
	t.Cleanup(func() { constant.MaxRequestBodyMB = old })
	engine := gin.New()
	engine.Use(DecompressRequestMiddleware())
	engine.POST("/test", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		var tooLarge *http.MaxBytesError
		require.True(t, errors.As(err, &tooLarge))
		require.Len(t, body, 1<<20)
		c.Status(http.StatusRequestEntityTooLarge)
	})
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(gzipRequestBytes(t, bytes.Repeat([]byte("x"), 2<<20))))
	req.Header.Set("Content-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
}

func TestRequestDecoderRejectsInvalidEncodingBeforeHandler(t *testing.T) {
	for _, encoding := range []string{"gzip", "unknown", "gzip, gzip, gzip, gzip, gzip"} {
		engine := gin.New()
		engine.Use(DecompressRequestMiddleware())
		engine.POST("/test", func(c *gin.Context) { t.Fatal("invalid compressed request reached handler") })
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("invalid compression"))
		req.Header.Set("Content-Encoding", encoding)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		require.True(t, recorder.Code == http.StatusBadRequest || recorder.Code == http.StatusUnsupportedMediaType)
		require.Contains(t, recorder.Body.String(), "request_encoding_invalid")
	}
}
