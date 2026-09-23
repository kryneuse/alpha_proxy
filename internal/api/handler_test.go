package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/auth"
	"github.com/kryneuse/alpha_proxy/internal/config"
	"github.com/kryneuse/alpha_proxy/internal/contract"
	"github.com/kryneuse/alpha_proxy/internal/middleware"
	"github.com/kryneuse/alpha_proxy/internal/observability"
	"github.com/kryneuse/alpha_proxy/internal/pii"
)

// fakeProcessor records calls and returns a configurable result or error.
type fakeProcessor struct {
	mu    sync.Mutex
	calls []contract.ProcessRequest
	ctxs  []context.Context
	resp  contract.ProcessResponse
	err   error
}

func (f *fakeProcessor) Process(ctx context.Context, req contract.ProcessRequest) (contract.ProcessResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req)
	f.ctxs = append(f.ctxs, ctx)
	resp := f.resp
	if resp.Result == "" {
		resp.Result = req.Payload
	}
	return resp, f.err
}

func (f *fakeProcessor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeProcessor) lastCall() (contract.ProcessRequest, context.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return contract.ProcessRequest{}, nil
	}
	return f.calls[len(f.calls)-1], f.ctxs[len(f.ctxs)-1]
}

func TestProcessStatusAndBody(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		bodyLimit   int64
		procErr     error
		wantStatus  int
		wantBody    string
		wantCalled  bool
	}{
		{
			name:        "valid request exact response",
			contentType: "application/json",
			body:        `{"payload":"строка","payload_id":"id-1"}`,
			bodyLimit:   1 << 20,
			wantStatus:  http.StatusOK,
			wantBody:    "{\"result\":\"строка\"}\n",
			wantCalled:  true,
		},
		{
			name:        "json with charset",
			contentType: "application/json; charset=utf-8",
			body:        `{"payload":"a","payload_id":"id-1"}`,
			bodyLimit:   1 << 20,
			wantStatus:  http.StatusOK,
			wantBody:    "{\"result\":\"a\"}\n",
			wantCalled:  true,
		},
		{
			name:        "missing content type",
			contentType: "",
			body:        `{"payload":"a","payload_id":"id-1"}`,
			bodyLimit:   1 << 20,
			wantStatus:  http.StatusBadRequest,
			wantBody:    "unsupported media type\n",
			wantCalled:  false,
		},
		{
			name:        "wrong content type",
			contentType: "text/plain",
			body:        `{"payload":"a","payload_id":"id-1"}`,
			bodyLimit:   1 << 20,
			wantStatus:  http.StatusBadRequest,
			wantBody:    "unsupported media type\n",
			wantCalled:  false,
		},
		{
			name:        "invalid content type",
			contentType: "not-a-media-type",
			body:        `{"payload":"a","payload_id":"id-1"}`,
			bodyLimit:   1 << 20,
			wantStatus:  http.StatusBadRequest,
			wantBody:    "unsupported media type\n",
			wantCalled:  false,
		},
		{
			name:        "missing payload",
			contentType: "application/json",
			body:        `{"payload_id":"id-1"}`,
			bodyLimit:   1 << 20,
			wantStatus:  http.StatusBadRequest,
			wantBody:    "missing payload\n",
			wantCalled:  false,
		},
		{
			name:        "present empty payload",
			contentType: "application/json",
			body:        `{"payload":"","payload_id":"id-1"}`,
			bodyLimit:   1 << 20,
			wantStatus:  http.StatusOK,
			wantBody:    "{\"result\":\"\"}\n",
			wantCalled:  true,
		},
		{
			name:        "missing payload_id",
			contentType: "application/json",
			body:        `{"payload":"a"}`,
			bodyLimit:   1 << 20,
			wantStatus:  http.StatusBadRequest,
			wantBody:    "missing payload_id\n",
			wantCalled:  false,
		},
		{
			name:        "empty payload_id",
			contentType: "application/json",
			body:        `{"payload":"a","payload_id":""}`,
			bodyLimit:   1 << 20,
			wantStatus:  http.StatusBadRequest,
			wantBody:    "empty payload_id\n",
			wantCalled:  false,
		},
		{
			name:        "non-string payload_id",
			contentType: "application/json",
			body:        `{"payload":"a","payload_id":123}`,
			bodyLimit:   1 << 20,
			wantStatus:  http.StatusBadRequest,
			wantBody:    "invalid json\n",
			wantCalled:  false,
		},
		{
			name:        "wrong field type",
			contentType: "application/json",
			body:        `{"payload":123,"payload_id":"id-1"}`,
			bodyLimit:   1 << 20,
			wantStatus:  http.StatusBadRequest,
			wantBody:    "invalid json\n",
			wantCalled:  false,
		},
		{
			name:        "empty body",
			contentType: "application/json",
			body:        ``,
			bodyLimit:   1 << 20,
			wantStatus:  http.StatusBadRequest,
			wantBody:    "invalid json\n",
			wantCalled:  false,
		},
		{
			name:        "broken json",
			contentType: "application/json",
			body:        `{"payload":"a","payload_id":`,
			bodyLimit:   1 << 20,
			wantStatus:  http.StatusBadRequest,
			wantBody:    "invalid json\n",
			wantCalled:  false,
		},
		{
			name:        "second json object",
			contentType: "application/json",
			body:        `{"payload":"a","payload_id":"id-1"} {"payload":"b","payload_id":"id-2"}`,
			bodyLimit:   1 << 20,
			wantStatus:  http.StatusBadRequest,
			wantBody:    "invalid json\n",
			wantCalled:  false,
		},
		{
			name:        "trailing garbage",
			contentType: "application/json",
			body:        `{"payload":"a","payload_id":"id-1"} garbage`,
			bodyLimit:   1 << 20,
			wantStatus:  http.StatusBadRequest,
			wantBody:    "invalid json\n",
			wantCalled:  false,
		},
		{
			name:        "body larger than limit",
			contentType: "application/json",
			body:        `{"payload":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","payload_id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`,
			bodyLimit:   16,
			wantStatus:  http.StatusRequestEntityTooLarge,
			wantBody:    "request too large\n",
			wantCalled:  false,
		},
		{
			name:        "trailing whitespace over limit",
			contentType: "application/json",
			body:        `{"payload":"a","payload_id":"id-1"}` + strings.Repeat(" ", 20),
			bodyLimit:   40,
			wantStatus:  http.StatusRequestEntityTooLarge,
			wantBody:    "request too large\n",
			wantCalled:  false,
		},
		{
			name:        "second object over limit",
			contentType: "application/json",
			body:        `{"payload":"a","payload_id":"id-1"} {"payload":"b","payload_id":"id-2"}`,
			bodyLimit:   50,
			wantStatus:  http.StatusRequestEntityTooLarge,
			wantBody:    "request too large\n",
			wantCalled:  false,
		},
		{
			name:        "wrapped ErrUnavailable",
			contentType: "application/json",
			body:        `{"payload":"a","payload_id":"id-1"}`,
			bodyLimit:   1 << 20,
			procErr:     fmt.Errorf("wrapped: %w", contract.ErrUnavailable),
			wantStatus:  http.StatusServiceUnavailable,
			wantBody:    "processor unavailable\n",
			wantCalled:  true,
		},
		{
			name:        "plain ErrUnavailable",
			contentType: "application/json",
			body:        `{"payload":"a","payload_id":"id-1"}`,
			bodyLimit:   1 << 20,
			procErr:     contract.ErrUnavailable,
			wantStatus:  http.StatusServiceUnavailable,
			wantBody:    "processor unavailable\n",
			wantCalled:  true,
		},
		{
			name:        "internal error",
			contentType: "application/json",
			body:        `{"payload":"a","payload_id":"id-1"}`,
			bodyLimit:   1 << 20,
			procErr:     errors.New("boom"),
			wantStatus:  http.StatusInternalServerError,
			wantBody:    "internal error\n",
			wantCalled:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeProcessor{}
			fake.err = tt.procErr

			handler := newTestHandler(fake, tt.bodyLimit, verifyAuthConfig())

			req := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body=%q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Fatalf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
			if got := fake.callCount(); got != 0 && !tt.wantCalled {
				t.Fatalf("processor called %d times, want 0", got)
			}
			if tt.wantCalled && fake.callCount() != 1 {
				t.Fatalf("processor called %d times, want 1", fake.callCount())
			}
		})
	}
}

func TestProcessPassesFieldsToProcessor(t *testing.T) {
	fake := &fakeProcessor{resp: contract.ProcessResponse{Result: "masked"}}
	handler := newTestHandler(fake, 1<<20, apiKeyAuthConfig())

	req := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"hello","payload_id":"id-42"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "test-key-a")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	call, _ := fake.lastCall()
	if call.Payload != "hello" {
		t.Errorf("Payload = %q, want hello", call.Payload)
	}
	if call.PayloadID != "id-42" {
		t.Errorf("PayloadID = %q, want id-42", call.PayloadID)
	}
	if call.ConsumerID != "sys-a" {
		t.Errorf("ConsumerID = %q, want sys-a", call.ConsumerID)
	}
}

func TestProcessMaskKindsAbsent(t *testing.T) {
	fake := &fakeProcessor{resp: contract.ProcessResponse{Result: "masked"}}
	handler := newTestHandler(fake, 1<<20, verifyAuthConfig())

	req := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"hello","payload_id":"id-42"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	call, _ := fake.lastCall()
	if call.MaskKindsSet {
		t.Errorf("MaskKindsSet = true, want false")
	}
	if call.MaskKinds != nil {
		t.Errorf("MaskKinds = %v, want nil", call.MaskKinds)
	}
}

func TestProcessMaskKindsEmptyArray(t *testing.T) {
	fake := &fakeProcessor{resp: contract.ProcessResponse{Result: "masked"}}
	handler := newTestHandler(fake, 1<<20, verifyAuthConfig())

	req := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"hello","payload_id":"id-42","mask_kinds":[]}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	call, _ := fake.lastCall()
	if !call.MaskKindsSet {
		t.Errorf("MaskKindsSet = false, want true")
	}
	if call.MaskKinds == nil || len(call.MaskKinds) != 0 {
		t.Errorf("MaskKinds = %v, want empty non-nil slice", call.MaskKinds)
	}
}

func TestProcessMaskKindsList(t *testing.T) {
	fake := &fakeProcessor{resp: contract.ProcessResponse{Result: "masked"}}
	handler := newTestHandler(fake, 1<<20, verifyAuthConfig())

	req := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"hello","payload_id":"id-42","mask_kinds":["phone","email"]}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	call, _ := fake.lastCall()
	if !call.MaskKindsSet {
		t.Errorf("MaskKindsSet = false, want true")
	}
	if len(call.MaskKinds) != 2 || call.MaskKinds[0] != "phone" || call.MaskKinds[1] != "email" {
		t.Errorf("MaskKinds = %v, want [phone email]", call.MaskKinds)
	}
}

func TestProcessMaskKindsInvalidKind(t *testing.T) {
	fake := &fakeProcessor{err: fmt.Errorf("wrapped: %w", pii.ErrInvalidMaskKind)}
	handler := newTestHandler(fake, 1<<20, verifyAuthConfig())

	req := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"hello","payload_id":"id-42","mask_kinds":["unknown"]}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if rec.Body.String() != "invalid mask kind\n" {
		t.Errorf("body = %q, want invalid mask kind", rec.Body.String())
	}
}

func TestProcessMaskKindsPolicyRejected(t *testing.T) {
	fake := &fakeProcessor{err: fmt.Errorf("wrapped: %w", pii.ErrPolicyRejected)}
	handler := newTestHandler(fake, 1<<20, verifyAuthConfig())

	req := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"hello","payload_id":"id-42","mask_kinds":["email"]}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if rec.Body.String() != "policy rejected\n" {
		t.Errorf("body = %q, want policy rejected", rec.Body.String())
	}
}

func TestProcessDemaskingDisabled(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"plain", pii.ErrDemaskingDisabled},
		{"wrapped", fmt.Errorf("wrapped: %w", pii.ErrDemaskingDisabled)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeProcessor{err: tt.err}
			handler := newTestHandler(fake, 1<<20, verifyAuthConfig())

			req := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"hello","payload_id":"id-42"}`))
			req.Header.Set("Content-Type", "application/json")

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", rec.Code)
			}
			if rec.Body.String() != "detokenization disabled\n" {
				t.Errorf("body = %q, want detokenization disabled", rec.Body.String())
			}
		})
	}
}

func TestProcessPreservesContext(t *testing.T) {
	type markerKey struct{}
	const marker = "marker-value"

	fake := &fakeProcessor{resp: contract.ProcessResponse{Result: "r"}}
	handler := newTestHandler(fake, 1<<20, verifyAuthConfig())

	req := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"a","payload_id":"id-1"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), markerKey{}, marker))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	_, ctx := fake.lastCall()
	if got, _ := ctx.Value(markerKey{}).(string); got != marker {
		t.Errorf("context marker = %q, want %q", got, marker)
	}
	if got := auth.ConsumerID(ctx); got != auth.VerifyConsumerID {
		t.Errorf("context ConsumerID = %q, want %q", got, auth.VerifyConsumerID)
	}
}

func TestProcessNotCalledOnInvalidRequest(t *testing.T) {
	invalid := []struct {
		name        string
		contentType string
		body        string
	}{
		{"missing content type", "", `{"payload":"a","payload_id":"id-1"}`},
		{"missing payload", "application/json", `{"payload_id":"id-1"}`},
		{"missing payload_id", "application/json", `{"payload":"a"}`},
		{"empty payload_id", "application/json", `{"payload":"a","payload_id":""}`},
		{"non-string payload_id", "application/json", `{"payload":"a","payload_id":123}`},
		{"broken json", "application/json", `{`},
		{"empty body", "application/json", ``},
		{"second object", "application/json", `{"payload":"a","payload_id":"id-1"} {}`},
		{"trailing whitespace over limit", "application/json", `{"payload":"a","payload_id":"id-1"}` + strings.Repeat(" ", 20)},
		{"second object over limit", "application/json", `{"payload":"a","payload_id":"id-1"} {"payload":"b","payload_id":"id-2"}`},
	}

	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeProcessor{resp: contract.ProcessResponse{Result: "x"}}
			bodyLimit := int64(1 << 20)
			if strings.Contains(tt.name, "over limit") {
				bodyLimit = 40
			}
			handler := newTestHandler(fake, bodyLimit, verifyAuthConfig())

			req := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if fake.callCount() != 0 {
				t.Fatalf("processor called %d times on invalid request, want 0", fake.callCount())
			}
		})
	}
}

func TestProcessDoesNotLeakSensitiveData(t *testing.T) {
	const (
		secretErr = "database connection failed: secret-dsn"
		payload   = "super-secret-payload"
		payloadID = "super-secret-id"
		apiKey    = "test-key-a"
		query     = "super-secret-query"
	)

	var buf bytes.Buffer
	log := observability.NewLoggerTo(&buf)
	fake := &fakeProcessor{err: errors.New(secretErr)}
	handler := newTestHandlerWithLogger(fake, 1<<20, apiKeyAuthConfig(), 0, log)

	req := httptest.NewRequest(http.MethodPost, "/process?"+query, strings.NewReader(
		fmt.Sprintf(`{"payload":%q,"payload_id":%q}`, payload, payloadID)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if rec.Body.String() != "internal error\n" {
		t.Errorf("body = %q, want stable message", rec.Body.String())
	}

	out := buf.String()
	if !strings.Contains(out, `"route":"POST /process"`) {
		t.Errorf("log missing route=POST /process: %q", out)
	}
	for _, leak := range []string{secretErr, payload, payloadID, apiKey, query} {
		if strings.Contains(out, leak) {
			t.Errorf("completion log leaked %q: %q", leak, out)
		}
	}
}

func TestCompletionLogUnknownRoute(t *testing.T) {
	const secretPath = "/super-secret-unknown-path"

	var buf bytes.Buffer
	log := observability.NewLoggerTo(&buf)
	fake := &fakeProcessor{resp: contract.ProcessResponse{Result: "r"}}
	handler := newTestHandlerWithLogger(fake, 1<<20, verifyAuthConfig(), 0, log)

	req := httptest.NewRequest(http.MethodPost, secretPath, nil)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	out := buf.String()
	if strings.Contains(out, secretPath) {
		t.Errorf("completion log leaked raw path %q: %q", secretPath, out)
	}
	if !strings.Contains(out, `"route":"unmatched"`) {
		t.Errorf("log missing route=unmatched: %q", out)
	}
}

func TestProcessSuccessContentType(t *testing.T) {
	fake := &fakeProcessor{resp: contract.ProcessResponse{Result: "r"}}
	handler := newTestHandler(fake, 1<<20, verifyAuthConfig())

	req := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"a","payload_id":"id-1"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestProcessTimeoutReturns503(t *testing.T) {
	// A processor that blocks on ctx.Done() and returns the wrapped deadline
	// error. No goroutine is used.
	blocking := &blockingProcessor{}
	var buf bytes.Buffer
	log := observability.NewLoggerTo(&buf)
	handler := newTestHandlerWithLogger(blocking, 1<<20, verifyAuthConfig(), 20*time.Millisecond, log)

	req := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"a","payload_id":"id-1"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if rec.Body.String() != "request timed out\n" {
		t.Errorf("body = %q, want stable timeout message", rec.Body.String())
	}
	out := buf.String()
	if !strings.Contains(out, `"error_class":"timeout"`) {
		t.Errorf("completion log missing error_class=timeout: %q", out)
	}
	if !strings.Contains(out, `"status":503`) {
		t.Errorf("completion log missing status=503: %q", out)
	}
}

// blockingProcessor waits on ctx.Done() and returns the wrapped context error.
type blockingProcessor struct{}

func (blockingProcessor) Process(ctx context.Context, _ contract.ProcessRequest) (contract.ProcessResponse, error) {
	<-ctx.Done()
	return contract.ProcessResponse{}, fmt.Errorf("wrapped: %w", ctx.Err())
}

func TestProcessProcessorSeesRequestID(t *testing.T) {
	fake := &fakeProcessor{resp: contract.ProcessResponse{Result: "r"}}
	handler := newTestHandler(fake, 1<<20, verifyAuthConfig())

	req := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"a","payload_id":"id-1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "req-123")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	_, ctx := fake.lastCall()
	if got := middleware.RequestID(ctx); got != "req-123" {
		t.Errorf("Processor request ID = %q, want req-123", got)
	}
	if got := rec.Header().Get("X-Request-ID"); got != "req-123" {
		t.Errorf("response X-Request-ID = %q, want req-123", got)
	}
}

func TestCompletionLogConsumerID(t *testing.T) {
	tests := []struct {
		name         string
		authCfg      config.Config
		apiKey       string
		wantStatus   int
		wantConsumer bool
		wantErrClass string
	}{
		{"authorized verify", verifyAuthConfig(), "", http.StatusOK, true, "none"},
		{"authorized api key", apiKeyAuthConfig(), "test-key-a", http.StatusOK, true, "none"},
		{"missing key 401", apiKeyAuthConfig(), "", http.StatusUnauthorized, false, "client_error"},
		{"invalid key 401", apiKeyAuthConfig(), "wrong-key", http.StatusUnauthorized, false, "client_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := observability.NewLoggerTo(&buf)
			fake := &fakeProcessor{resp: contract.ProcessResponse{Result: "r"}}

			handler := newTestHandlerWithLogger(fake, 1<<20, tt.authCfg, 0, log)

			req := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"a","payload_id":"id-1"}`))
			req.Header.Set("Content-Type", "application/json")
			if tt.apiKey != "" {
				req.Header.Set("X-API-Key", tt.apiKey)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			out := buf.String()
			if got := strings.Count(out, `"event":"request_completed"`); got != 1 {
				t.Errorf("expected exactly one completion record, got %d: %q", got, out)
			}
			if !strings.Contains(out, `"route":"POST /process"`) {
				t.Errorf("log missing route=POST /process: %q", out)
			}
			if !strings.Contains(out, `"error_class":"`+tt.wantErrClass+`"`) {
				t.Errorf("log missing error_class=%s: %q", tt.wantErrClass, out)
			}
			hasConsumer := strings.Contains(out, `"consumer_id":`)
			if tt.wantConsumer && !hasConsumer {
				t.Errorf("log missing consumer_id: %q", out)
			}
			if !tt.wantConsumer && hasConsumer {
				t.Errorf("log contains consumer_id for %s: %q", tt.name, out)
			}
		})
	}
}

func TestCompletionLogPanic(t *testing.T) {
	var buf bytes.Buffer
	log := observability.NewLoggerTo(&buf)

	panicProc := &panicProcessor{}
	handler := newTestHandlerWithLogger(panicProc, 1<<20, verifyAuthConfig(), 0, log)

	req := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"a","payload_id":"id-1"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	out := buf.String()
	if got := strings.Count(out, `"event":"request_completed"`); got != 1 {
		t.Errorf("expected exactly one completion record, got %d: %q", got, out)
	}
	if !strings.Contains(out, `"status":500`) {
		t.Errorf("log missing status=500: %q", out)
	}
	if !strings.Contains(out, `"error_class":"panic"`) {
		t.Errorf("log missing error_class=panic: %q", out)
	}
}

// panicProcessor panics when invoked.
type panicProcessor struct{}

func (panicProcessor) Process(context.Context, contract.ProcessRequest) (contract.ProcessResponse, error) {
	panic("boom")
}

// newTestHandler wires the handler through the full production chain: request
// ID, completion logger, recovery, route metadata, auth, processing timeout and
// body limit.
func newTestHandler(p contract.Processor, bodyLimit int64, authCfg config.Config) http.Handler {
	return newTestHandlerWithTimeout(p, bodyLimit, authCfg, 0)
}

func newTestHandlerWithTimeout(p contract.Processor, bodyLimit int64, authCfg config.Config, timeout time.Duration) http.Handler {
	return newTestHandlerWithLogger(p, bodyLimit, authCfg, timeout, observability.NewLoggerTo(io.Discard))
}

func newTestHandlerWithLogger(p contract.Processor, bodyLimit int64, authCfg config.Config, timeout time.Duration, log *observability.Logger) http.Handler {
	handler := NewHandler(p)
	authenticator := auth.New(authCfg)

	var process http.Handler = http.HandlerFunc(handler.Process)
	process = middleware.BodyLimit(bodyLimit, process)
	if timeout > 0 {
		process = middleware.ProcessingTimeout(timeout, process)
	}
	process = authenticator.Middleware(process)
	process = middleware.RouteMetadata(ProcessRoute, process)

	mux := http.NewServeMux()
	mux.Handle(ProcessRoute, process)

	var h http.Handler = mux
	h = middleware.RequestIDMiddleware(h)
	h = middleware.Recover(h)
	h = middleware.CompletionLogger(log, h)
	return h
}

func verifyAuthConfig() config.Config {
	return config.Config{AuthMode: config.AuthModeVerify}
}

func apiKeyAuthConfig() config.Config {
	return config.Config{
		AuthMode: config.AuthModeAPIKey,
		Systems: []config.System{
			{ID: "sys-a", Enabled: true, APIKey: "test-key-a"},
		},
	}
}
