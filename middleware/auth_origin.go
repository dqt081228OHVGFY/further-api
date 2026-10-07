package middleware

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/gin-gonic/gin"
)

// SessionCookieOriginGuard protects browser session issuance, refresh and
// logout regardless of the cookie's Secure setting. Local HTTP development
// permits loopback origins on another port and originless non-browser clients.
// It never adds CORS response headers and must not be installed on relay routes.
func SessionCookieOriginGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !common.SessionCookieSecure && len(c.Request.Header.Values("Origin")) == 0 &&
			len(c.Request.Header.Values("Referer")) == 0 && c.GetHeader("Sec-Fetch-Site") != "cross-site" {
			c.Next()
			return
		}
		origin, ok := requestBrowserOrigin(c.Request)
		allowed := ok && (isAllowedSessionOrigin(c.Request, origin) ||
			(!common.SessionCookieSecure && isAllowedDevelopmentSessionOrigin(c.Request, origin)))
		if !allowed {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"code":    "AUTH_ORIGIN_FORBIDDEN",
				"message": "request origin is not allowed",
			})
			return
		}
		c.Next()
	}
}

func isAllowedDevelopmentSessionOrigin(request *http.Request, origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	// Support an HTTPS frontend behind a TLS-terminating proxy while Secure
	// cookies are being configured, without trusting forwarded request headers.
	if request.TLS == nil && parsed.Scheme == "https" {
		target, err := common.NormalizeOrigin("https://" + request.Host)
		if err == nil && target == origin {
			return true
		}
	}
	if request.TLS != nil || parsed.Scheme != "http" {
		return false
	}
	target, err := url.Parse("http://" + request.Host)
	if err != nil {
		return false
	}
	isLoopback := func(host string) bool {
		if strings.EqualFold(host, "localhost") {
			return true
		}
		ip := net.ParseIP(host)
		return ip != nil && ip.IsLoopback()
	}
	return isLoopback(target.Hostname()) && isLoopback(parsed.Hostname())
}

func requestBrowserOrigin(request *http.Request) (string, bool) {
	originValues := request.Header.Values("Origin")
	if len(originValues) > 1 {
		return "", false
	}
	if len(originValues) == 1 {
		if strings.Contains(originValues[0], ",") {
			return "", false
		}
		origin, err := common.NormalizeOrigin(originValues[0])
		return origin, err == nil
	}
	refererValues := request.Header.Values("Referer")
	if len(refererValues) != 1 {
		return "", false
	}
	referer, err := url.Parse(strings.TrimSpace(refererValues[0]))
	if err != nil || referer.Scheme == "" || referer.Host == "" || referer.User != nil {
		return "", false
	}
	origin, err := common.NormalizeOrigin(referer.Scheme + "://" + referer.Host)
	return origin, err == nil
}

func isAllowedSessionOrigin(request *http.Request, origin string) bool {
	requestScheme := "http"
	if request.TLS != nil {
		requestScheme = "https"
	}
	requestOrigin, err := common.NormalizeOrigin(requestScheme + "://" + request.Host)
	if err == nil && subtle.ConstantTimeCompare([]byte(origin), []byte(requestOrigin)) == 1 {
		return true
	}
	for _, trustedOrigin := range common.SessionCookieTrustedURLs {
		if subtle.ConstantTimeCompare([]byte(origin), []byte(trustedOrigin)) == 1 {
			return true
		}
	}
	return false
}
