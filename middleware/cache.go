package middleware

import (
	"github.com/gin-gonic/gin"
)

// CacheHeaders also lets the embedded asset fast path retain the same cache
// policy without advancing into the page rate limiter.
func CacheHeaders(c *gin.Context) {
	if c.Request.RequestURI == "/" {
		c.Header("Cache-Control", "no-cache")
	} else {
		c.Header("Cache-Control", "max-age=604800") // one week
	}
	c.Header("Cache-Version", "b688f2fb5be447c25e5aa3bd063087a83db32a288bf6a4f35f2d8db310e40b14")
}

func Cache() func(c *gin.Context) {
	return func(c *gin.Context) {
		CacheHeaders(c)
		c.Next()
	}
}
