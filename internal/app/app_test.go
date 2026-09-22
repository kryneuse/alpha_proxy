package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"alpha_proxy/internal/config"
	"alpha_proxy/internal/contract"
	"alpha_proxy/internal/observability"
)

// fakeProcessor records calls.
type fakeProcessor struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeProcessor) Process(context.Context, contract.ProcessRequest) (contract.ProcessResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return contract.ProcessResponse{Result: "ok"}, nil
}

func (f *fakeProcessor) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestGlobalLimiterBeforeAuth(t *testing.T) {
	cfg := baseConfig()
	cfg.GlobalRateLimitRPS = 1
	cfg.GlobalRateLimitBurst = 1
	cfg.ConsumerRateLimitRPS = 0

	fake := &fakeProcessor{}
	handler := New(cfg, observability.NewLoggerTo(discard{}), fake)

	// Unauthenticated request consumes the single global token, then auth fails.
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"a","payload_id":"id-1"}`))
	req1.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusUnauthorized {
		t.Fatalf("first status = %d, want 401", rec1.Code)
	}

	// Authorized request is rejected by the exhausted global bucket.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"a","payload_id":"id-1"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-API-Key", "secret-a")
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429 (global limiter ran before auth)", rec2.Code)
	}
	if fake.count() != 0 {
		t.Fatalf("processor called %d times, want 0", fake.count())
	}
}

func TestConsumerLimiterAfterAuth(t *testing.T) {
	cfg := baseConfig()
	cfg.GlobalRateLimitRPS = 0
	cfg.ConsumerRateLimitRPS = 1
	cfg.ConsumerRateLimitBurst = 1

	fake := &fakeProcessor{}
	handler := New(cfg, observability.NewLoggerTo(discard{}), fake)

	// First authorized request consumes the consumer token.
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"a","payload_id":"id-1"}`))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-API-Key", "secret-a")
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", rec1.Code)
	}

	// Second authorized request is rejected by the per-consumer bucket.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"a","payload_id":"id-1"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-API-Key", "secret-a")
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429 (per-consumer limiter after auth)", rec2.Code)
	}
}

func TestConsumerLimiterIndependentBuckets(t *testing.T) {
	cfg := baseConfig()
	cfg.GlobalRateLimitRPS = 0
	cfg.ConsumerRateLimitRPS = 1
	cfg.ConsumerRateLimitBurst = 1
	cfg.Systems = []config.System{
		{ID: "sys-a", Enabled: true, APIKey: "secret-a"},
		{ID: "sys-b", Enabled: true, APIKey: "secret-b"},
	}

	fake := &fakeProcessor{}
	handler := New(cfg, observability.NewLoggerTo(discard{}), fake)

	// Consumer A exhausts its bucket.
	reqA1 := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"a","payload_id":"id-1"}`))
	reqA1.Header.Set("Content-Type", "application/json")
	reqA1.Header.Set("X-API-Key", "secret-a")
	recA1 := httptest.NewRecorder()
	handler.ServeHTTP(recA1, reqA1)
	if recA1.Code != http.StatusOK {
		t.Fatalf("A first status = %d, want 200", recA1.Code)
	}

	reqA2 := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"a","payload_id":"id-1"}`))
	reqA2.Header.Set("Content-Type", "application/json")
	reqA2.Header.Set("X-API-Key", "secret-a")
	recA2 := httptest.NewRecorder()
	handler.ServeHTTP(recA2, reqA2)
	if recA2.Code != http.StatusTooManyRequests {
		t.Fatalf("A second status = %d, want 429", recA2.Code)
	}

	// Consumer B has an independent bucket and is still allowed.
	reqB := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"a","payload_id":"id-1"}`))
	reqB.Header.Set("Content-Type", "application/json")
	reqB.Header.Set("X-API-Key", "secret-b")
	recB := httptest.NewRecorder()
	handler.ServeHTTP(recB, reqB)
	if recB.Code != http.StatusOK {
		t.Fatalf("B status = %d, want 200 (independent bucket)", recB.Code)
	}
}

func TestInvalidKeyNoConsumerBucket(t *testing.T) {
	cfg := baseConfig()
	cfg.GlobalRateLimitRPS = 0
	cfg.ConsumerRateLimitRPS = 1
	cfg.ConsumerRateLimitBurst = 1

	fake := &fakeProcessor{}
	handler := New(cfg, observability.NewLoggerTo(discard{}), fake)

	// Invalid key -> 401, does not reach the per-consumer limiter.
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"a","payload_id":"id-1"}`))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-API-Key", "wrong-key")
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusUnauthorized {
		t.Fatalf("invalid key status = %d, want 401", rec1.Code)
	}

	// Valid key is still allowed: the invalid key created no consumer bucket.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"a","payload_id":"id-1"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-API-Key", "secret-a")
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("valid key status = %d, want 200", rec2.Code)
	}
	if fake.count() != 1 {
		t.Fatalf("processor called %d times, want 1", fake.count())
	}
}

func TestUnknownRouteNotRateLimited(t *testing.T) {
	cfg := baseConfig()
	cfg.GlobalRateLimitRPS = 1
	cfg.GlobalRateLimitBurst = 1
	cfg.ConsumerRateLimitRPS = 0

	fake := &fakeProcessor{}
	handler := New(cfg, observability.NewLoggerTo(discard{}), fake)

	// Exhaust the global bucket via POST /process.
	req1 := httptest.NewRequest(http.MethodPost, "/process", strings.NewReader(`{"payload":"a","payload_id":"id-1"}`))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-API-Key", "secret-a")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("process status = %d, want 200", rec1.Code)
	}

	// Unknown route is not rate limited and returns 404.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("unknown route status = %d, want 404", rec2.Code)
	}
}

func baseConfig() config.Config {
	return config.Config{
		Addr:                   ":8080",
		ReadTimeout:            10 * time.Second,
		ReadHeaderTimeout:      5 * time.Second,
		WriteTimeout:           10 * time.Second,
		IdleTimeout:            60 * time.Second,
		BodyLimit:              1 << 20,
		ProcessingTimeout:      5 * time.Second,
		ParallelLimit:          512,
		OverloadRetryAfter:     time.Second,
		GlobalRateLimitRPS:     2000,
		GlobalRateLimitBurst:   2000,
		ConsumerRateLimitRPS:   0,
		ConsumerRateLimitBurst: 0,
		RunMode:                config.RunModeFinal,
		AuthMode:               config.AuthModeAPIKey,
		ProcessorMode:          config.ProcessorReal,
		Systems:                []config.System{{ID: "sys-a", Enabled: true, APIKey: "secret-a"}},
	}
}

// discard is an io.Writer that discards output.
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
