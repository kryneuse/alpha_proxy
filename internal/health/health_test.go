package health

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestNewReadinessInitiallyNotReady(t *testing.T) {
	r := NewReadiness()
	if r.IsReady() {
		t.Error("IsReady() = true, want false for a new Readiness")
	}
}

func TestReadinessFalseToTrue(t *testing.T) {
	r := NewReadiness()
	r.SetReady(true)
	if !r.IsReady() {
		t.Error("IsReady() = false, want true after SetReady(true)")
	}
}

func TestReadinessTrueToFalse(t *testing.T) {
	r := NewReadiness()
	r.SetReady(true)
	r.SetReady(false)
	if r.IsReady() {
		t.Error("IsReady() = true, want false after SetReady(false)")
	}
}

func TestReadyHandlerNotReady(t *testing.T) {
	r := NewReadiness()
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	if got := rec.Body.String(); got != "not ready\n" {
		t.Errorf("body = %q, want %q", got, "not ready\n")
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", got, "text/plain; charset=utf-8")
	}
}

func TestReadyHandlerReady(t *testing.T) {
	r := NewReadiness()
	r.SetReady(true)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "ready\n" {
		t.Errorf("body = %q, want %q", got, "ready\n")
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", got, "text/plain; charset=utf-8")
	}
}

func TestLivenessHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	LivenessHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "ok\n" {
		t.Errorf("body = %q, want %q", got, "ok\n")
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", got, "text/plain; charset=utf-8")
	}
}

func TestReadinessConcurrentAccess(t *testing.T) {
	r := NewReadiness()

	const workers = 8
	const iterations = 1000

	var wg sync.WaitGroup
	wg.Add(workers * 2)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				r.SetReady(j%2 == 0)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = r.IsReady()
			}
		}()
	}

	wg.Wait()
}
