package middleware

import (
	"compress/gzip"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
)

type requestBodyDecoder struct {
	io.Reader
	closers  []io.Closer
	once     sync.Once
	closeErr error
}

func (body *requestBodyDecoder) Close() error {
	body.once.Do(func() {
		for index := len(body.closers) - 1; index >= 0; index-- {
			body.closeErr = errors.Join(body.closeErr, body.closers[index].Close())
		}
	})
	return body.closeErr
}

type decoderCloseFunc func() error

func (close decoderCloseFunc) Close() error { return close() }

func gatewayRequestBodyLimit() int64 {
	mb := int64(constant.MaxRequestBodyMB)
	if mb <= 0 || mb > math.MaxInt64>>20 {
		mb = 32
	}
	return mb << 20
}

func decodeRequestBody(c *gin.Context) (*requestBodyDecoder, bool, int, error) {
	source := c.Request.Body
	body := &requestBodyDecoder{Reader: http.MaxBytesReader(c.Writer, source, gatewayRequestBodyLimit()), closers: []io.Closer{source}}
	raw := strings.Join(c.Request.Header.Values("Content-Encoding"), ",")
	if strings.TrimSpace(raw) == "" {
		return body, false, 0, nil
	}
	encodings := strings.Split(raw, ",")
	if len(encodings) > 4 {
		_ = body.Close()
		return nil, false, http.StatusUnsupportedMediaType, errors.New("请求体压缩层数超过限制。")
	}
	decoded := false
	// Encodings are applied from left to right on the wire and decoded in reverse.
	for index := len(encodings) - 1; index >= 0; index-- {
		encoding := strings.ToLower(strings.TrimSpace(encodings[index]))
		var reader io.Reader
		switch encoding {
		case "identity":
			continue
		case "gzip":
			gzipReader, err := gzip.NewReader(body.Reader)
			if err != nil {
				_ = body.Close()
				return nil, false, http.StatusBadRequest, errors.New("请求体 gzip 压缩数据无效。")
			}
			reader = gzipReader
			body.closers = append(body.closers, gzipReader)
		case "br":
			reader = brotli.NewReader(body.Reader)
		case "zstd":
			zstdReader, err := zstd.NewReader(body.Reader, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(uint64(max(gatewayRequestBodyLimit(), 8<<20))))
			if err != nil {
				_ = body.Close()
				return nil, false, http.StatusBadRequest, errors.New("请求体 zstd 压缩数据无效。")
			}
			reader = zstdReader
			body.closers = append(body.closers, decoderCloseFunc(func() error { zstdReader.Close(); return nil }))
		default:
			_ = body.Close()
			return nil, false, http.StatusUnsupportedMediaType, errors.New("请求体仅支持 gzip、br 与 zstd 压缩。")
		}
		decoded = true
		body.Reader = http.MaxBytesReader(c.Writer, io.NopCloser(reader), gatewayRequestBodyLimit())
	}
	return body, decoded, 0, nil
}

// Both compressed and expanded input are bounded. All decoder resources are
// request-scoped, including malformed inputs and requests canceled downstream.
func DecompressRequestMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil || c.Request.Body == nil || c.Request.Body == http.NoBody {
			c.Next()
			return
		}
		body, decoded, status, err := decodeRequestBody(c)
		if err != nil {
			c.AbortWithStatusJSON(status, gin.H{"error": gin.H{
				"message": err.Error(), "type": "gateway_request_error", "code": "request_encoding_invalid",
			}})
			return
		}
		defer body.Close()
		c.Request.Body = body
		if decoded {
			c.Request.ContentLength = -1
			c.Request.GetBody = nil
			c.Request.Header.Del("Content-Encoding")
			c.Request.Header.Del("Content-Length")
		}
		c.Next()
	}
}
