package middleware

import "github.com/gin-gonic/gin"

// SecurityHeaders applies browser protections without restricting application
// scripts, animations, API CORS, or same-origin embedded views. A handler may
// replace the default CSP with a stronger policy for a downloadable artifact.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "SAMEORIGIN")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "frame-ancestors 'self'; object-src 'none'; base-uri 'self'")
		c.Next()
	}
}
