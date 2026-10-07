package middleware

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/gin-gonic/gin"
)

const RouteTagKey = "route_tag"

func RouteTag(tag string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(RouteTagKey, tag)
		c.Next()
	}
}

func SetUpLogger(server *gin.Engine) {
	server.Use(redactTaskArtifactAccessQuery())
	server.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		var requestID string
		if param.Keys != nil {
			requestID, _ = param.Keys[common.RequestIdKey].(string)
		}
		tag, _ := param.Keys[RouteTagKey].(string)
		if tag == "" {
			tag = "web"
		}
		path := redactLoggedRequestPath(param.Path)
		// OAuth callbacks carry one-time codes and state in the query string.
		// Redact the log value only; the handler still needs the original query.
		if strings.HasPrefix(path, "/api/oauth/") || strings.HasPrefix(path, "/oauth/") {
			path, _, _ = strings.Cut(path, "?")
		}
		return fmt.Sprintf("[GIN] %s | %s | %s | %3d | %13v | %15s | %7s %s\n",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			tag,
			requestID,
			param.StatusCode,
			param.Latency,
			param.ClientIP,
			param.Method,
			path,
		)
	}))
}

func redactLoggedRequestPath(path string) string {
	base, rawQuery, present := strings.Cut(path, "?")
	if !present {
		return path
	}
	if strings.HasPrefix(base, "/api/oauth/") || strings.HasPrefix(base, "/oauth/") {
		return base
	}
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return base
	}
	changed := false
	for name := range query {
		switch strings.ToLower(name) {
		case "key", "api_key", "api-key", "token", "access_token", "refresh_token", "access", "password", "secret", "code", "state", "nonce", "signature", "sign", "jwt", "ticket":
			query.Set(name, "[REDACTED]")
			changed = true
		}
	}
	if !changed {
		return path
	}
	return base + "?" + query.Encode()
}
