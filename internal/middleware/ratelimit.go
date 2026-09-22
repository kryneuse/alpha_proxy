package middleware

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/auth"
	"github.com/kryneuse/alpha_proxy/internal/ratelimit"
	"github.com/kryneuse/alpha_proxy/internal/requestmeta"
)

// GlobalRateLimit applies a global token bucket before authentication. A nil
// bucket disables the limiter.
func GlobalRateLimit(bucket *ratelimit.TokenBucket, next http.Handler) http.Handler {
	if bucket == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, delay := bucket.Allow()
		if !ok {
			writeTooManyRequests(w, r, delay, "rate_limited")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ConsumerRateLimit applies a per-consumer token bucket after authentication. A
// nil limiter disables it. The key is the confirmed auth.ConsumerID; unknown
// consumers are not limited and never create state.
func ConsumerRateLimit(limiter *ratelimit.ConsumerLimiter, next http.Handler) http.Handler {
	if limiter == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, delay := limiter.Allow(auth.ConsumerID(r.Context()))
		if !ok {
			writeTooManyRequests(w, r, delay, "rate_limited")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// writeTooManyRequests writes a 429 with a Retry-After header and records the
// error class in the logging metadata.
func writeTooManyRequests(w http.ResponseWriter, r *http.Request, delay time.Duration, errorClass string) {
	if meta := requestmeta.From(r.Context()); meta != nil {
		meta.ErrorClass = errorClass
	}
	seconds := int64(math.Ceil(delay.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
	http.Error(w, "too many requests", http.StatusTooManyRequests)
}
