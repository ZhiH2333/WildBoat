package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/ZhiH2333/WildBoat/internal/httpapi/response"
)

// RateLimiter is a simple per-key fixed-window limiter for abuse-prone routes.
type RateLimiter struct {
	mu     sync.Mutex
	window time.Duration
	limit  int
	hits   map[string]bucket
	now    func() time.Time
}

type bucket struct {
	count int
	reset time.Time
}

// NewRateLimiter constructs a fixed-window limiter.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	if limit <= 0 {
		limit = 20
	}
	if window <= 0 {
		window = time.Minute
	}
	return &RateLimiter{
		window: window,
		limit:  limit,
		hits:   make(map[string]bucket),
		now:    time.Now,
	}
}

// Allow reports whether key may proceed and consumes one unit when allowed.
func (l *RateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, ok := l.hits[key]
	if !ok || !b.reset.After(now) {
		l.hits[key] = bucket{count: 1, reset: now.Add(l.window)}
		return true
	}
	if b.count >= l.limit {
		return false
	}
	b.count++
	l.hits[key] = b
	return true
}

// Limit applies the limiter using client IP + route path as the key.
func (l *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := clientIP(r) + "|" + r.Method + "|" + r.URL.Path
		if !l.Allow(key) {
			response.Error(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests", RequestIDFromContext(r.Context()), nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
