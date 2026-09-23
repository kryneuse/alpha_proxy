package middleware

import (
	"net/http"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/observability"
	"github.com/kryneuse/alpha_proxy/internal/requestmeta"
)

// Metrics wraps h and records HTTP request metrics (count, duration, in-flight
// gauge and timeouts) into metrics. A nil metrics disables observation and
// returns next unchanged. It never reads the request body, never logs data,
// spawns no goroutine, does not recover panics and does not alter the response
// status or body. Only the normalized method, route and status are recorded; no
// payload, payload_id, consumer, request ID, URL path, query string or error
// text is ever used as a label.
func Metrics(metrics *observability.Metrics, next http.Handler) http.Handler {
	return metricsWithClock(metrics, next, time.Now)
}

// metricsWithClock is the internal variant used by tests to inject a
// deterministic clock. A nil clock falls back to time.Now.
func metricsWithClock(metrics *observability.Metrics, next http.Handler, now func() time.Time) http.Handler {
	if metrics == nil {
		return next
	}
	if now == nil {
		now = time.Now
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := newStatusRecorder(w)
		metrics.IncHTTPInFlight()
		defer metrics.DecHTTPInFlight()

		start := now()
		next.ServeHTTP(rec, r)
		duration := now().Sub(start)

		metrics.ObserveHTTPRequest(r.Method, routeOf(r), rec.status, duration)

		if meta := requestmeta.From(r.Context()); meta != nil && meta.ErrorClass == "timeout" {
			metrics.IncHTTPTimeout()
		}
	})
}

// routeOf returns the recorded route pattern from the request metadata, or
// "unmatched" when none is recorded. It never reads r.URL.Path, RawPath,
// RawQuery or RequestURI.
func routeOf(r *http.Request) string {
	if meta := requestmeta.From(r.Context()); meta != nil && meta.Route != "" {
		return meta.Route
	}
	return "unmatched"
}
