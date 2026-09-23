// Package middleware provides HTTP middleware for the contour: request ID,
// structured completion logging, panic recovery, a processing timeout and a
// body-size limit. It does not implement any data-protection logic.
//
// Logging is deliberately minimal: only safe event classes and fields are
// emitted. No payload, payload_id, headers, API keys, URL paths, query strings,
// Processor error text, panic values or stack traces are ever logged.
package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/requestmeta"
)

// Recover wraps h and converts panics into safe 500 responses. The panic value
// and stack trace are never logged; the completion logger records status=500 and
// error_class="panic". If the response has already started, the status and body
// are left untouched.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func(ctx context.Context) {
			if recover() != nil {
				if meta := requestmeta.From(ctx); meta != nil {
					meta.ErrorClass = "panic"
				}
				if hw, ok := w.(headerWritten); ok && hw.HeaderWritten() {
					return
				}
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}(r.Context())
		next.ServeHTTP(w, r)
	})
}

// ParallelLimit bounds the number of concurrently processed requests with a
// fixed-capacity semaphore. A slot is acquired without waiting or queueing; when
// full, the request is rejected immediately with 429. The slot is released via
// defer on success, error, panic and timeout. No goroutine is spawned.
func ParallelLimit(limit int, retryAfter time.Duration, next http.Handler) http.Handler {
	sem := make(chan struct{}, limit)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
		default:
			writeTooManyRequests(w, r, retryAfter, "concurrency_limited")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ProcessingTimeout wraps h and bounds the request processing time via a
// context deadline. It does not use http.TimeoutHandler and does not spawn a
// goroutine.
func ProcessingTimeout(d time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), d)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// BodyLimit wraps h and limits the request body size.
func BodyLimit(n int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, n)
		next.ServeHTTP(w, r)
	})
}
