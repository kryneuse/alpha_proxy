// Package ratelimit provides a minimal per-consumer rate limiter for the HTTP
// contour. It is transport-level only and does not affect processing state.
package ratelimit

import (
	"net/http"
	"sync"
	"time"
)

// Limiter is a simple fixed-window limiter keyed by consumer.
type Limiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	entries map[string]*entry
}

type entry struct {
	count   int
	resetAt time.Time
}

// New returns a Limiter allowing limit requests per window per consumer.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{
		limit:   limit,
		window:  window,
		entries: make(map[string]*entry),
	}
}

// Allow reports whether a request from key is within the limit.
func (l *Limiter) Allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	e, ok := l.entries[key]
	if !ok || now.After(e.resetAt) {
		e = &entry{count: 0, resetAt: now.Add(l.window)}
		l.entries[key] = e
	}
	if e.count >= l.limit {
		return false
	}
	e.count++
	return true
}

// Middleware rejects requests that exceed the limit with 429.
func (l *Limiter) Middleware(keyFn func(*http.Request) string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.Allow(keyFn(r)) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}