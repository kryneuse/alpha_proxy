package middleware

import "net/http"

// statusRecorder records the response status without buffering the body. It
// unwraps to the underlying ResponseWriter for http.ResponseController.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

// newStatusRecorder wraps w with an initial status of 200.
func newStatusRecorder(w http.ResponseWriter) *statusRecorder {
	return &statusRecorder{ResponseWriter: w, status: http.StatusOK}
}

// WriteHeader records the first explicit status and forwards it. Repeated calls
// do not change the recorded status.
func (r *statusRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.status = code
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(code)
}

// Write records an implicit 200 and forwards the body.
func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap returns the underlying ResponseWriter for http.ResponseController.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// HeaderWritten reports whether the status header has been sent.
func (r *statusRecorder) HeaderWritten() bool {
	return r.wroteHeader
}
