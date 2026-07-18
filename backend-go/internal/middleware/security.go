package middleware

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"brickplans/internal/config"
)

// SecurityHeaders sets browser security headers on every response. Note HSTS is
// expected to be added at the nginx layer (TLS termination).
//
// connect-src is widened to include the COS host when STORAGE_BACKEND=tencent_cos,
// so PDF.js can fetch blueprint PDFs directly from COS (cross-origin) without
// tunneling the bytes through the backend. PDF traffic stays on COS bandwidth.
func SecurityHeaders(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// object-src 'none' blocks <object>/<embed> of arbitrary content;
		// frame-ancestors 'none' prevents the site from being framed (clickjacking).
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"img-src 'self' data: https:; "+
				"media-src 'self'; "+
				"object-src 'none'; "+
				"frame-ancestors 'none'; "+
				"base-uri 'self'; "+
				"connect-src 'self'"+cosConnectSrc(cfg))
		c.Next()
	}
}

// cosConnectSrc returns the COS origin to append to connect-src (e.g.
// " https://bucket.cos.region.myqcloud.com"), or "" when COS is not in use.
// Derived from TencentCOSPublicBaseURL (custom CDN) or bucket+region.
func cosConnectSrc(cfg *config.Config) string {
	if cfg.StorageBackend != "tencent_cos" {
		return ""
	}
	host := ""
	if cfg.TencentCOSPublicBaseURL != "" {
		if u, err := url.Parse(cfg.TencentCOSPublicBaseURL); err == nil && u.Host != "" {
			host = u.Host
		}
	}
	if host == "" && cfg.TencentCOSBucket != "" && cfg.TencentCOSRegion != "" {
		host = fmt.Sprintf("%s.cos.%s.myqcloud.com", cfg.TencentCOSBucket, cfg.TencentCOSRegion)
	}
	if host == "" {
		return ""
	}
	return " https://" + host
}

// Recovery swallows panics and returns a generic 500 without leaking stack traces.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		_ = recovered
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"detail": "internal server error"})
	})
}
