package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// ipLimiter is a per-(IP, route) token bucket. Entries are swept periodically so
// the map cannot grow without bound under attack.
type ipLimiter struct {
	mu       sync.Mutex
	limiters map[string]*entry
	rpm      int
	burst    int
}

type entry struct {
	limiter *rate.Limiter
	lastHit time.Time
}

func newIPLimiter(rpm, burst int) *ipLimiter {
	l := &ipLimiter{
		limiters: make(map[string]*entry),
		rpm:      rpm,
		burst:    burst,
	}
	go l.sweep()
	return l
}

func (l *ipLimiter) get(key string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e, ok := l.limiters[key]; ok {
		e.lastHit = time.Now()
		return e.limiter
	}
	e := &entry{limiter: rate.NewLimiter(rate.Limit(float64(l.rpm)/60.0), l.burst), lastHit: time.Now()}
	l.limiters[key] = e
	return e.limiter
}

// sweep drops idle entries (unused for >10 min) every minute.
func (l *ipLimiter) sweep() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		cutoff := time.Now().Add(-10 * time.Minute)
		l.mu.Lock()
		for k, e := range l.limiters {
			if e.lastHit.Before(cutoff) {
				delete(l.limiters, k)
			}
		}
		l.mu.Unlock()
	}
}

// RateLimit limits requests per client IP + matched route to `perMinute` with a
// burst allowance. Apply to sensitive groups (login, register, upload, report).
func RateLimit(perMinute, burst int) gin.HandlerFunc {
	l := newIPLimiter(perMinute, burst)
	return func(c *gin.Context) {
		key := c.ClientIP() + "|" + c.FullPath()
		if !l.get(key).Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"detail": "请求过于频繁，请稍后再试"})
			return
		}
		c.Next()
	}
}
