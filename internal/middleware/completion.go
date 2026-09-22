package middleware

import (
	"net/http"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/observability"
	"github.com/kryneuse/alpha_proxy/internal/requestmeta"
)

// headerWritten is implemented by the status recorder.
type headerWritten interface {
	HeaderWritten() bool
}

// CompletionLogger wraps h and emits exactly one structured completion record
// per request after the handler chain (including recovery) has finished. Only
// safe fields are logged. It creates the request-scoped metadata before the
// request ID middleware so that even a failed ID generation produces a record.
func CompletionLogger(log *observability.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := newStatusRecorder(w)
		meta := &requestmeta.Meta{}
		ctx := requestmeta.With(r.Context(), meta)

		start := time.Now()
		next.ServeHTTP(rec, r.WithContext(ctx))
		dur := time.Since(start)

		route := meta.Route
		if route == "" {
			route = "unmatched"
		}

		args := []any{
			"event", "request_completed",
			"request_id", meta.RequestID,
			"method", r.Method,
			"route", route,
			"status", rec.status,
			"duration_ms", dur.Milliseconds(),
			"error_class", errorClass(meta.ErrorClass, rec.status),
		}
		if meta.ConsumerID != "" {
			args = append(args, "consumer_id", meta.ConsumerID)
		}
		log.Info("request_completed", args...)
	})
}

// errorClass returns the safe error classification. A special class set by the
// handler or recovery wins; otherwise it is derived from the status code.
func errorClass(special string, status int) string {
	if special != "" {
		return special
	}
	switch {
	case status < 400:
		return "none"
	case status < 500:
		return "client_error"
	default:
		return "server_error"
	}
}
