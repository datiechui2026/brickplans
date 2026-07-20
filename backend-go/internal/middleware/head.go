package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HeadToGet converts HEAD requests to GET so Gin's method-tree routing
// matches them. Without this, Gin returns 404 for HEAD because r.GET()
// only populates the GET method tree (see brickplans-development pitfall #47).
// The response body is suppressed for HEAD per RFC 7231 §4.3.2.
//
// This is critical for SEO: crawlers like Googlebot send HEAD requests to
// check page status, and a 404 on HEAD causes the URL to be de-indexed
// even though GET returns 200.
func HeadToGet() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodHead {
			c.Request.Method = http.MethodGet
			c.Writer = &headWriter{ResponseWriter: c.Writer}
		}
		c.Next()
	}
}

// headWriter wraps gin.ResponseWriter and discards the response body
// while preserving all header writes (Content-Type, Content-Length, etc.).
type headWriter struct {
	gin.ResponseWriter
}

func (w *headWriter) Write(b []byte) (int, error) {
	return len(b), nil // discard body, pretend write succeeded
}

func (w *headWriter) WriteString(s string) (int, error) {
	return len(s), nil
}
