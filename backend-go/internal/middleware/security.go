package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// SecurityHeaders sets browser security headers on every response. Note HSTS is
// expected to be added at the nginx layer (TLS termination).
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// object-src 'none' blocks <object>/<embed> of arbitrary content; frame-ancestors
		// 'none' prevents the site from being framed (clickjacking).
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"img-src 'self' data: https:; "+
				"media-src 'self'; "+
				"object-src 'none'; "+
				"frame-ancestors 'none'; "+
				"base-uri 'self'")
		c.Next()
	}
}

// Recovery swallows panics and returns a generic 500 without leaking stack traces.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		_ = recovered
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"detail": "internal server error"})
	})
}
