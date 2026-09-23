package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/observability"
	"github.com/kryneuse/alpha_proxy/internal/ratelimit"
)

// fakeClock is a deterministic clock for rate limit tests.
type fakeClock struct {
	t time.Time
}

func (c *fakeClock) Now() time.Time { return c.t }

func TestGlobalRateLimitExhausted(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	bucket := ratelimit.NewTokenBucket(1, 1, clock)

	var buf bytes.Buffer
	log := observability.NewLoggerTo(&buf)

	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := CompletionLogger(log, GlobalRateLimit(bucket, inner))

	// First request consumes the single token.
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodPost, "/process", nil)
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", rec1.Code)
	}

	// Second request is rejected.
	called = false
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/process", nil)
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429", rec2.Code)
	}
	if called {
		t.Error("downstream handler called on rejected request")
	}

	// Retry-After is an integer >= 1.
	ra := rec2.Header().Get("Retry-After")
	n, err := strconv.Atoi(ra)
	if err != nil || n < 1 {
		t.Errorf("Retry-After = %q, want integer >= 1", ra)
	}

	// Completion log has status=429 and error_class=rate_limited.
	out := buf.String()
	if !strings.Contains(out, `"status":429`) {
		t.Errorf("log missing status=429: %q", out)
	}
	if !strings.Contains(out, `"error_class":"rate_limited"`) {
		t.Errorf("log missing error_class=rate_limited: %q", out)
	}
}

func TestGlobalRateLimitNilPassesThrough(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := GlobalRateLimit(nil, inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !called {
		t.Error("downstream handler not called with nil limiter")
	}
}

func TestParallelLimitRejectsWhenFull(t *testing.T) {
	limit := 1
	retryAfter := 2 * time.Second

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	var startedOnce sync.Once

	var buf bytes.Buffer
	log := observability.NewLoggerTo(&buf)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() { close(started) })
		<-release
		w.WriteHeader(http.StatusOK)
	})
	handler := CompletionLogger(log, ParallelLimit(limit, retryAfter, inner))

	// First request takes the slot and blocks.
	go func() {
		defer close(done)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/process", nil)
		handler.ServeHTTP(rec, req)
	}()
	<-started

	// Second request is rejected immediately.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/process", nil)
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429", rec2.Code)
	}
	ra := rec2.Header().Get("Retry-After")
	n, err := strconv.Atoi(ra)
	if err != nil || n < 1 {
		t.Errorf("Retry-After = %q, want integer >= 1", ra)
	}
	out := buf.String()
	if !strings.Contains(out, `"error_class":"concurrency_limited"`) {
		t.Errorf("log missing error_class=concurrency_limited: %q", out)
	}

	// Release the first request; it must complete.
	close(release)
	<-done

	// Next request gets the slot.
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "/process", nil)
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("third status = %d, want 200", rec3.Code)
	}
}

func TestParallelLimitReleasesSlot(t *testing.T) {
	tests := []struct {
		name string
		run  func(http.Handler) http.Handler
	}{
		{
			name: "success",
			run: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				})
			},
		},
		{
			name: "error status",
			run: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "bad", http.StatusBadRequest)
				})
			},
		},
		{
			name: "panic",
			run: func(next http.Handler) http.Handler {
				return Recover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					panic("boom")
				}))
			},
		},
		{
			name: "timeout",
			run: func(next http.Handler) http.Handler {
				return ProcessingTimeout(20*time.Millisecond, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					<-r.Context().Done()
					w.WriteHeader(http.StatusOK)
				}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := tt.run(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
			handler := ParallelLimit(1, time.Second, inner)

			// First request occupies the slot.
			rec1 := httptest.NewRecorder()
			req1 := httptest.NewRequest(http.MethodPost, "/process", nil)
			handler.ServeHTTP(rec1, req1)

			// After it returns, the slot must be free.
			rec2 := httptest.NewRecorder()
			req2 := httptest.NewRequest(http.MethodPost, "/process", nil)
			handler.ServeHTTP(rec2, req2)
			if rec2.Code == http.StatusTooManyRequests {
				t.Fatalf("slot not released after %s", tt.name)
			}
		})
	}
}
