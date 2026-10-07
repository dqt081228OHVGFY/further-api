package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/http/httpguts"
)

func CloseResponseBodyGracefully(response *http.Response) {
	if response != nil && response.Body != nil {
		if err := response.Body.Close(); err != nil {
			common.SysError("close upstream response: " + err.Error())
		}
	}
}

// Instance-owned headers cannot be replaced by provider data. Connection and
// transfer framing are always selected by the downstream HTTP server.
func ShouldCopyUpstreamHeader(c *gin.Context, name string, values []string) bool {
	if strings.EqualFold(name, common.RequestIdKey) {
		if c != nil && len(values) > 0 && values[0] != "" {
			c.Set(common.UpstreamRequestIdKey, values[0])
		}
		return false
	}
	if len(values) == 0 || !httpguts.ValidHeaderFieldName(name) {
		return false
	}
	switch strings.ToLower(name) {
	case "content-length", "connection", "keep-alive", "proxy-authenticate",
		"proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade",
		"set-cookie", "server", "x-powered-by":
		return false
	}
	if strings.HasPrefix(strings.ToLower(name), "access-control-") {
		return false
	}
	for _, value := range values {
		if !httpguts.ValidHeaderFieldValue(value) {
			return false
		}
	}
	return true
}

func copyBufferedUpstreamHeaders(c *gin.Context, response *http.Response) {
	if response == nil {
		return
	}
	local := c.Writer.Header()
	connectionFields := make(map[string]bool)
	for _, value := range response.Header.Values("Connection") {
		for _, name := range strings.Split(value, ",") {
			connectionFields[strings.ToLower(strings.TrimSpace(name))] = true
		}
	}
	for name, values := range response.Header {
		if !ShouldCopyUpstreamHeader(c, name, values) || connectionFields[strings.ToLower(name)] {
			continue
		}
		switch strings.ToLower(name) {
		case "content-encoding", "etag", "content-md5", "digest":
			// The protocol handler supplies decoded or converted bytes.
			continue
		}
		local.Del(name)
		for _, value := range values {
			local.Add(name, value)
		}
	}
}

// Buffering is completed by the caller before any header is committed. This
// boundary writes one final response and preserves every repeated header value.
func IOCopyBytesGracefully(c *gin.Context, response *http.Response, data []byte) {
	if c == nil || c.Writer == nil {
		return
	}
	if c.Request != nil && c.Request.Context().Err() != nil {
		return
	}
	if c.Writer.Written() {
		logger.LogError(c, "buffered upstream response arrived after response commitment")
		return
	}
	status := http.StatusOK
	if response != nil {
		status = response.StatusCode
		if status < 200 || status > 599 {
			status = http.StatusBadGateway
		}
	}
	copyBufferedUpstreamHeaders(c, response)
	header := c.Writer.Header()
	header.Del("Content-Encoding")
	header.Del("Transfer-Encoding")
	header.Del("Connection")
	if status == http.StatusNoContent || status == http.StatusNotModified {
		header.Del("Content-Length")
		c.Writer.WriteHeader(status)
		c.Writer.WriteHeaderNow()
		return
	}
	header.Set("Content-Length", strconv.Itoa(len(data)))
	if header.Get("Content-Type") == "" {
		contentType := http.DetectContentType(data)
		if json.Valid(data) {
			contentType = "application/json; charset=utf-8"
		}
		header.Set("Content-Type", contentType)
	}
	c.Writer.WriteHeader(status)
	if c.Request != nil && c.Request.Method == http.MethodHead {
		c.Writer.WriteHeaderNow()
		return
	}
	if _, err := c.Writer.Write(data); err != nil {
		logger.LogError(c, fmt.Sprintf("write upstream response: %s", err))
		return
	}
	c.Writer.Flush()
}
