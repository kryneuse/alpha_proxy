package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/contract"
	"github.com/kryneuse/alpha_proxy/internal/observability"
	"github.com/kryneuse/alpha_proxy/internal/requestmeta"
)

func TestRequestIDPreservesValidInput(t *testing.T) {
	const id = "abc-123.ABC_9"
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := RequestID(r.Context()); got != id {
			t.Errorf("RequestID = %q, want %q", got, id)
		}
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	req.Header.Set("X-Request-ID", id)
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != id {
		t.Errorf("response X-Request-ID = %q, want %q", got, id)
	}
}

func TestRequestIDReplacesInvalid(t *testing.T) {
	invalid := []string{"", "too long: " + strings.Repeat("x", 65), "has space", "has/slash", "unicode-привет"}
	for _, in := range invalid {
		t.Run("input="+in, func(t *testing.T) {
			handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := RequestID(r.Context()); got == in {
					t.Errorf("RequestID = %q, want a generated replacement", got)
				}
			}))

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/process", nil)
			if in != "" {
				req.Header.Set("X-Request-ID", in)
			}
			handler.ServeHTTP(rec, req)

			got := rec.Header().Get("X-Request-ID")
			if got == in {
				t.Errorf("response X-Request-ID = %q, want a generated replacement", got)
			}
			if !validRequestID(got) {
				t.Errorf("generated ID %q is not in the safe format", got)
			}
		})
	}
}

func TestRequestIDGeneratedFormat(t *testing.T) {
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := RequestID(r.Context())
		if len(id) != 32 {
			t.Errorf("generated ID length = %d, want 32", len(id))
		}
		for _, c := range id {
			isDigit := c >= '0' && c <= '9'
			isLowerHex := c >= 'a' && c <= 'f'
			if !isDigit && !isLowerHex {
				t.Errorf("generated ID contains non-hex char %q", c)
			}
		}
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	handler.ServeHTTP(rec, req)
}

func TestRequestIDGeneratorError(t *testing.T) {
	// A reader that always fails.
	failing := &failReader{}
	called := false
	handler := requestIDMiddlewareWithReader(failing, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if called {
		t.Error("downstream handler was called despite generator error")
	}
	if got := rec.Header().Get("X-Request-ID"); got != "" {
		t.Errorf("response X-Request-ID = %q, want empty (no fixed ID)", got)
	}
}

type failReader struct{}

func (failReader) Read([]byte) (int, error) {
	return 0, errors.New("entropy source failed")
}

func TestRequestIDProcessorSeesSameID(t *testing.T) {
	var seen string
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestID(r.Context())
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	handler.ServeHTTP(rec, req)

	if seen == "" {
		t.Fatal("processor did not see a request ID")
	}
	if seen != rec.Header().Get("X-Request-ID") {
		t.Errorf("context ID %q != response header %q", seen, rec.Header().Get("X-Request-ID"))
	}
}

func TestStatusRecorderExplicitAndImplicit(t *testing.T) {
	// Explicit WriteHeader.
	rec := httptest.NewRecorder()
	sr := newStatusRecorder(rec)
	sr.WriteHeader(http.StatusCreated)
	sr.WriteHeader(http.StatusInternalServerError) // must be ignored
	if sr.status != http.StatusCreated {
		t.Errorf("explicit status = %d, want 201", sr.status)
	}
	if !sr.HeaderWritten() {
		t.Error("HeaderWritten = false, want true")
	}

	// Implicit 200 on Write.
	rec2 := httptest.NewRecorder()
	sr2 := newStatusRecorder(rec2)
	_, _ = sr2.Write([]byte("ok"))
	if sr2.status != http.StatusOK {
		t.Errorf("implicit status = %d, want 200", sr2.status)
	}

	// Unwrap returns the underlying writer.
	if sr.Unwrap() != rec {
		t.Error("Unwrap did not return the underlying ResponseWriter")
	}
}

func TestRecoverPanicReturns500AndLogsOnce(t *testing.T) {
	var buf bytes.Buffer
	log := observability.NewLoggerTo(&buf)

	panicValue := "super-secret-panic-value"
	handler := CompletionLogger(log, Recover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(panicValue)
	})))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}

	out := buf.String()
	if got := strings.Count(out, `"event":"request_completed"`); got != 1 {
		t.Errorf("expected exactly one completion record, got %d: %q", got, out)
	}
	if strings.Contains(out, panicValue) {
		t.Errorf("log leaked panic value: %q", out)
	}
	if strings.Contains(out, "goroutine") || strings.Contains(out, ".go:") {
		t.Errorf("log leaked stack trace: %q", out)
	}
	if !strings.Contains(out, `"error_class":"panic"`) {
		t.Errorf("log missing error_class=panic: %q", out)
	}
	if !strings.Contains(out, `"status":500`) {
		t.Errorf("log missing status=500: %q", out)
	}
}

func TestCompletionLogIsValidJSONWithAllowedFields(t *testing.T) {
	var buf bytes.Buffer
	log := observability.NewLoggerTo(&buf)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if meta := requestmeta.From(r.Context()); meta != nil {
			meta.ConsumerID = "consumer-1"
			meta.Route = "POST /process"
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := CompletionLogger(log, RequestIDMiddleware(inner))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	handler.ServeHTTP(rec, req)

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("log is not valid JSON: %v (%q)", err, buf.String())
	}
	if m["event"] != "request_completed" {
		t.Errorf("event = %v, want request_completed", m["event"])
	}
	if m["status"] != float64(http.StatusOK) {
		t.Errorf("status = %v, want 200", m["status"])
	}
	if m["consumer_id"] != "consumer-1" {
		t.Errorf("consumer_id = %v, want consumer-1", m["consumer_id"])
	}
	if m["route"] != "POST /process" {
		t.Errorf("route = %v, want POST /process", m["route"])
	}
	if m["error_class"] != "none" {
		t.Errorf("error_class = %v, want none", m["error_class"])
	}
	if _, ok := m["duration_ms"].(float64); !ok {
		t.Errorf("duration_ms is not a number: %v", m["duration_ms"])
	}
	if got, _ := m["request_id"].(string); got == "" {
		t.Errorf("request_id missing: %v", m["request_id"])
	}
}

func TestCompletionLogDoesNotLeakSensitiveData(t *testing.T) {
	var buf bytes.Buffer
	log := observability.NewLoggerTo(&buf)

	const (
		payload   = "super-secret-payload"
		payloadID = "super-secret-id"
		apiKey    = "super-secret-api-key"
		procErr   = "super-secret-processor-error"
		query     = "super-secret-query"
	)

	// A processor that returns a secret error text.
	proc := &errProcessor{err: errors.New(procErr)}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = proc.Process(r.Context(), contract.ProcessRequest{})
		w.WriteHeader(http.StatusInternalServerError)
	})
	handler := CompletionLogger(log, RequestIDMiddleware(inner))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process?"+query, strings.NewReader(
		fmt.Sprintf(`{"payload":%q,"payload_id":%q}`, payload, payloadID)))
	req.Header.Set("X-API-Key", apiKey)
	handler.ServeHTTP(rec, req)

	out := buf.String()
	for _, leak := range []string{payload, payloadID, apiKey, procErr, query, "/process"} {
		if strings.Contains(out, leak) {
			t.Errorf("log leaked %q: %q", leak, out)
		}
	}
}

// errProcessor returns a fixed error.
type errProcessor struct {
	err error
}

func (e *errProcessor) Process(context.Context, contract.ProcessRequest) (contract.ProcessResponse, error) {
	return contract.ProcessResponse{}, e.err
}

func TestProcessingTimeoutDeadline(t *testing.T) {
	handler := ProcessingTimeout(20*time.Millisecond, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		if err := r.Context().Err(); err != context.DeadlineExceeded {
			t.Errorf("ctx.Err() = %v, want DeadlineExceeded", err)
		}
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	handler.ServeHTTP(rec, req)
}
