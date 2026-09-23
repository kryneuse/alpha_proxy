// Package health provides liveness and readiness handlers for the HTTP contour.
// Readiness state is stored in an atomic flag so it is safe for concurrent use.
// The handlers are self-contained: they call no external components, spawn no
// goroutines and log nothing.
package health

import (
	"net/http"
	"sync/atomic"
)

// Readiness tracks whether the service is ready to serve traffic. It is safe
// for concurrent use.
type Readiness struct {
	ready atomic.Bool
}

// NewReadiness returns a Readiness that is initially not ready.
func NewReadiness() *Readiness {
	return &Readiness{}
}

// SetReady sets the readiness state.
func (r *Readiness) SetReady(ready bool) {
	r.ready.Store(ready)
}

// IsReady reports whether the service is ready.
func (r *Readiness) IsReady() bool {
	return r.ready.Load()
}

// Handler returns an HTTP handler that reports readiness. It returns 200 with
// "ready\n" when ready and 503 with "not ready\n" otherwise.
func (r *Readiness) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if r.IsReady() {
			writePlain(w, http.StatusOK, "ready\n")
			return
		}
		writePlain(w, http.StatusServiceUnavailable, "not ready\n")
	})
}

// LivenessHandler returns an HTTP handler that always reports the process as
// alive with 200 and "ok\n". It does not depend on readiness.
func LivenessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writePlain(w, http.StatusOK, "ok\n")
	})
}

// writePlain writes a plain-text response with the given status and body.
func writePlain(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
