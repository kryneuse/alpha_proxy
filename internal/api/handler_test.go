package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"alpha_proxy/internal/auth"
	"alpha_proxy/internal/config"
	"alpha_proxy/internal/contract"
	"alpha_proxy/internal/middleware"
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
	)

	fake := &fakeProcessor{err: errors.New(secretErr)}
	handler := newTestHandler(fake, 1<<20, apiKeyAuthConfig())

	req := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(
		fmt.Sprintf(`{"payload":%q,"payload_id":%q}`, payload, payloadID)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	for _, leak := range []string{secretErr, payload, payloadID, apiKey} {
		if strings.Contains(body, leak) {
			t.Errorf("response leaked %q: %q", leak, body)
		}
	}
	if body != "internal error\n" {
		t.Errorf("body = %q, want stable message", body)
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

// newTestHandler wires the handler through the mux, the auth middleware and the
// body limit, matching the production chain order.
func newTestHandler(p contract.Processor, bodyLimit int64, authCfg config.Config) http.Handler {
	mux := http.NewServeMux()
	NewHandler(p).Routes(mux)
	authenticator := auth.New(authCfg)
	var h http.Handler = mux
	h = authenticator.Middleware(h)
	h = middleware.BodyLimit(bodyLimit, h)
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
