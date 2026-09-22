package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"alpha_proxy/internal/observability"
)

func TestRecoverReturns500AndLogsSafeClass(t *testing.T) {
	var buf bytes.Buffer
	log := observability.NewLoggerTo(&buf)

	panicValue := "super-secret-panic-value"
	handler := Recover(log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(panicValue)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}

	out := buf.String()
	if !strings.Contains(out, "panic_recovered") {
		t.Errorf("log does not contain safe event class: %q", out)
	}
	if strings.Contains(out, panicValue) {
		t.Errorf("log leaked panic value: %q", out)
	}
	if strings.Contains(out, "goroutine") || strings.Contains(out, ".go:") {
		t.Errorf("log leaked stack trace: %q", out)
	}
}

func TestLoggingDoesNotLogPath(t *testing.T) {
	var buf bytes.Buffer
	log := observability.NewLoggerTo(&buf)

	handler := Logging(log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	req.Header.Set("X-API-Key", "secret-key")
	handler.ServeHTTP(rec, req)

	out := buf.String()
	if strings.Contains(out, "/process") {
		t.Errorf("log leaked URL path: %q", out)
	}
	if strings.Contains(out, "secret-key") {
		t.Errorf("log leaked header value: %q", out)
	}
}