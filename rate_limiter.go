// rate_limiter.go
// Per-client token-bucket rate limiting for Go endpoints, using golang.org/x/time/rate.
//
// Node.js's own endpoints (e.g. POST /api/auth/login) are rate-limited separately in
// routes/auth.js — this file only covers routes registered directly against the Go
// router in main.go. Named to match admin_auth.go's convention (root-level file,
// package main) rather than a middleware/ subpackage, since that directory is the
// Node.js middleware tree, not shared with Go.
package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// rateLimiterEntry pairs a token bucket with the last time it was used, so
// ipRateLimiter can evict idle entries and bound memory under many distinct callers.
type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// keyedRateLimiter tracks one token bucket per key (client IP or authenticated user ID).
type keyedRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rateLimiterEntry
	rps      rate.Limit
	burst    int
}

// newKeyedRateLimiter creates a limiter allowing requestsPerMinute sustained rate with
// the given burst capacity, per distinct key. Starts a background goroutine that evicts
// entries idle for more than 30 minutes so long-running processes don't leak memory.
func newKeyedRateLimiter(requestsPerMinute, burst int) *keyedRateLimiter {
	rl := &keyedRateLimiter{
		limiters: make(map[string]*rateLimiterEntry),
		rps:      rate.Limit(float64(requestsPerMinute) / 60.0),
		burst:    burst,
	}
	go rl.evictIdleLoop()
	return rl
}

func (rl *keyedRateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	entry, ok := rl.limiters[key]
	if !ok {
		entry = &rateLimiterEntry{limiter: rate.NewLimiter(rl.rps, rl.burst)}
		rl.limiters[key] = entry
	}
	entry.lastSeen = time.Now()
	return entry.limiter.Allow()
}

func (rl *keyedRateLimiter) evictIdleLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-30 * time.Minute)
		rl.mu.Lock()
		for key, entry := range rl.limiters {
			if entry.lastSeen.Before(cutoff) {
				delete(rl.limiters, key)
			}
		}
		rl.mu.Unlock()
	}
}

// rateLimitByUserOrIP keys by the authenticated caller's X-User-ID (forwarded by the
// Node.js proxy — see app.js's forwardToGo) when present, falling back to client IP for
// unauthenticated or direct-to-Go requests.
func rateLimitByUserOrIP(c *gin.Context) string {
	if uid := c.GetHeader("X-User-ID"); uid != "" {
		return "user:" + uid
	}
	return "ip:" + c.ClientIP()
}

// rateLimitMiddleware returns Gin middleware enforcing requestsPerMinute (with burst)
// per key, where key is derived from each request via keyFn.
func rateLimitMiddleware(requestsPerMinute, burst int, keyFn func(*gin.Context) string) gin.HandlerFunc {
	limiter := newKeyedRateLimiter(requestsPerMinute, burst)
	return func(c *gin.Context) {
		if !limiter.allow(keyFn(c)) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"error":   "rate limit exceeded — please slow down",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
