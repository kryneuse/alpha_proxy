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

	"alpha_proxy/internal/requestmeta"
)

// Recover wraps h and converts panics into safe 500 responses. The panic value
// and stack trace are never logged; the completion logger records status=500 and
// error_class="panic". If the response has already started, the status and body
// are left untouched.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				if meta := requestmeta.From(r.Context()); meta != nil {
					meta.ErrorClass = "panic"
				}
				if hw, ok := w.(headerWritten); ok && hw.HeaderWritten() {
					return
				}
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ParallelLimit wraps h and limits the number of concurrently processed requests.
// It is not part of the active HTTP path.
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
