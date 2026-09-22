// Package middleware provides HTTP middleware for the contour: safe request
// logging, panic recovery, a parallel-processing limit, a processing timeout and
// a body-size limit. It does not implement any data-protection logic.
//
// Logging is deliberately minimal: only safe event classes are emitted. No
// payload, payload_id, headers, API keys, URL paths or Processor error text are
// ever logged.
package middleware

import (
	"context"
	"net/http"
	"time"

	"alpha_proxy/internal/observability"
)

// Logging wraps h and logs only a safe event class and the duration.
func Logging(log *observability.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Info("request", "dur", time.Since(start).String())
	})
}

// Recover wraps h and converts panics into 500 responses. Only the safe event
// class is logged; the panic value and stack trace are never logged.
func Recover(log *observability.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				log.Error("panic_recovered")
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ParallelLimit wraps h and limits the number of concurrently processed requests.
func ParallelLimit(limit int, next http.Handler) http.Handler {
	sem := make(chan struct{}, limit)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
		default:
			http.Error(w, "too many concurrent requests", http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ProcessingTimeout wraps h and bounds the request processing time.
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