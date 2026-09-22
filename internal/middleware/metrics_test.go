package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/observability"
	"github.com/kryneuse/alpha_proxy/internal/requestmeta"
)

func TestMetricsSuccessfulRequest(t *testing.T) {
	m, err := observability.NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	start := time.Unix(100, 0)
	finish := time.Unix(100, 500_000_000) // 500ms later
	clock := sequenceClock(start, finish)

	meta := &requestmeta.Meta{Route: "POST /process"}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := withMeta(meta, metricsWithClock(m, inner, clock))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	handler.ServeHTTP(rec, req)

	if got := httpRequestsCounter(t, m, "POST", "POST /process", "200"); got != 1 {
		t.Errorf("requests counter = %v, want 1", got)
	}
	count, sum := httpDurationHistogram(t, m, "POST", "POST /process")
	if count != 1 {
		t.Errorf("histogram sample_count = %d, want 1", count)
	}
	if want := finish.Sub(start).Seconds(); sum != want {
		t.Errorf("histogram sample_sum = %v, want %v", sum, want)
	}
}

func TestMetricsFinalStatuses(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			m, err := observability.NewMetrics()
			if err != nil {
				t.Fatalf("NewMetrics() error: %v", err)
			}

			meta := &requestmeta.Meta{Route: "POST /process"}
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			})
			handler := withMeta(meta, metricsWithClock(m, inner, sequenceClock(time.Unix(100, 0), time.Unix(100, 100_000_000))))

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/process", nil)
			handler.ServeHTTP(rec, req)

			if got := httpRequestsCounter(t, m, "POST", "POST /process", strconv.Itoa(status)); got != 1 {
				t.Errorf("requests counter = %v, want 1", got)
			}
		})
	}
}

func TestMetricsMissingRouteIsUnmatched(t *testing.T) {
	m, err := observability.NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	// No request metadata is set up, so the route must fall back to "unmatched".
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := metricsWithClock(m, inner, sequenceClock(time.Unix(100, 0), time.Unix(100, 100_000_000)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/some/secret/path", nil)
	handler.ServeHTTP(rec, req)

	if got := httpRequestsCounter(t, m, "GET", "unmatched", "200"); got != 1 {
		t.Errorf("requests counter = %v, want 1", got)
	}
}

func TestMetricsInFlightGauge(t *testing.T) {
	m, err := observability.NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	done := make(chan struct{})
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := inFlightGauge(t, m); got != 1 {
			t.Errorf("in-flight during request = %v, want 1", got)
		}
		close(done)
		w.WriteHeader(http.StatusOK)
	})
	handler := metricsWithClock(m, inner, sequenceClock(time.Unix(100, 0), time.Unix(100, 100_000_000)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	handler.ServeHTTP(rec, req)
	<-done

	if got := inFlightGauge(t, m); got != 0 {
		t.Errorf("in-flight after request = %v, want 0", got)
	}
}

func TestMetricsNilIsNoop(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	})
	handler := Metrics(nil, inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("downstream handler was not called")
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rec.Code)
	}
}

func TestMetricsTimeoutErrorClass(t *testing.T) {
	m, err := observability.NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	meta := &requestmeta.Meta{Route: "POST /process", ErrorClass: "timeout"}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	handler := withMeta(meta, metricsWithClock(m, inner, sequenceClock(time.Unix(100, 0), time.Unix(100, 100_000_000))))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	handler.ServeHTTP(rec, req)

	if got := timeoutsCounter(t, m); got != 1 {
		t.Errorf("timeouts counter = %v, want 1", got)
	}
}

func TestMetricsOtherErrorClassNoTimeout(t *testing.T) {
	m, err := observability.NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	for _, class := range []string{"panic", "rate_limited", "none"} {
		t.Run(class, func(t *testing.T) {
			meta := &requestmeta.Meta{Route: "POST /process", ErrorClass: class}
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			handler := withMeta(meta, metricsWithClock(m, inner, sequenceClock(time.Unix(100, 0), time.Unix(100, 100_000_000))))

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/process", nil)
			handler.ServeHTTP(rec, req)

			if got := timeoutsCounter(t, m); got != 0 {
				t.Errorf("timeouts counter = %v, want 0", got)
			}
		})
	}
}

func TestMetricsWithRecoverOnPanic(t *testing.T) {
	m, err := observability.NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	meta := &requestmeta.Meta{Route: "POST /process"}
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})
	handler := withMeta(meta, metricsWithClock(m, Recover(panicHandler), sequenceClock(time.Unix(100, 0), time.Unix(100, 100_000_000))))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if got := httpRequestsCounter(t, m, "POST", "POST /process", "500"); got != 1 {
		t.Errorf("requests counter = %v, want 1", got)
	}
	if got := inFlightGauge(t, m); got != 0 {
		t.Errorf("in-flight after request = %v, want 0", got)
	}
}

func TestMetricsImplicitStatus200(t *testing.T) {
	m, err := observability.NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	meta := &requestmeta.Meta{Route: "POST /process"}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	handler := withMeta(meta, metricsWithClock(m, inner, sequenceClock(time.Unix(100, 0), time.Unix(100, 100_000_000))))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	handler.ServeHTTP(rec, req)

	if got := httpRequestsCounter(t, m, "POST", "POST /process", "200"); got != 1 {
		t.Errorf("requests counter = %v, want 1", got)
	}
}

func TestMetricsDoesNotLeakSensitiveData(t *testing.T) {
	m, err := observability.NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	const (
		path       = "UNIQUE_PATH_1a2b"
		query      = "UNIQUE_QUERY_3c4d"
		requestID  = "UNIQUE_REQUEST_ID_5e6f"
		consumerID = "UNIQUE_CONSUMER_7a8b"
		apiKey     = "UNIQUE_API_KEY_9c0d"
		bodyText   = "UNIQUE_BODY_1e2f"
	)

	meta := &requestmeta.Meta{Route: "POST /process", ConsumerID: consumerID, RequestID: requestID}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := withMeta(meta, metricsWithClock(m, inner, sequenceClock(time.Unix(100, 0), time.Unix(100, 100_000_000))))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/"+path+"?"+query, strings.NewReader(bodyText))
	req.Header.Set("X-API-Key", apiKey)
	handler.ServeHTTP(rec, req)

	metricsRec := httptest.NewRecorder()
	m.Handler().ServeHTTP(metricsRec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	body := metricsRec.Body.String()
	for _, leak := range []string{path, query, requestID, consumerID, apiKey, bodyText} {
		if strings.Contains(body, leak) {
			t.Errorf("metrics exposition leaked %q: %q", leak, body)
		}
	}
}

// withMeta puts meta into the request context before delegating to h, mirroring
// how the completion logger sets up request-scoped metadata in production.
func withMeta(meta *requestmeta.Meta, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(requestmeta.With(r.Context(), meta)))
	})
}

// sequenceClock returns a clock function that yields the given times in sequence,
// holding the last value for any further calls.
func sequenceClock(times ...time.Time) func() time.Time {
	i := 0
	return func() time.Time {
		if i < len(times)-1 {
			t := times[i]
			i++
			return t
		}
		return times[len(times)-1]
	}
}

// httpRequestsCounter returns the value of the HTTP requests counter matching
// the given method, route and status labels, or 0 when absent.
func httpRequestsCounter(t *testing.T, m *observability.Metrics, method, route, status string) float64 {
	t.Helper()
	families, err := m.Gatherer().Gather()
	if err != nil {
		t.Fatalf("Gather() error: %v", err)
	}
	for _, f := range families {
		if f.GetName() != "alpha_proxy_http_requests_total" {
			continue
		}
		for _, metric := range f.GetMetric() {
			if metricHasLabels(metric.GetLabel(), map[string]string{
				"method": method, "route": route, "status": status,
			}) {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}

// httpDurationHistogram returns the sample count and sum of the HTTP duration
// histogram matching the given method and route labels.
func httpDurationHistogram(t *testing.T, m *observability.Metrics, method, route string) (uint64, float64) {
	t.Helper()
	families, err := m.Gatherer().Gather()
	if err != nil {
		t.Fatalf("Gather() error: %v", err)
	}
	for _, f := range families {
		if f.GetName() != "alpha_proxy_http_request_duration_seconds" {
			continue
		}
		for _, metric := range f.GetMetric() {
			if metricHasLabels(metric.GetLabel(), map[string]string{
				"method": method, "route": route,
			}) {
				h := metric.GetHistogram()
				return h.GetSampleCount(), h.GetSampleSum()
			}
		}
	}
	t.Fatalf("histogram %q with method=%s route=%s not found", "alpha_proxy_http_request_duration_seconds", method, route)
	return 0, 0
}

// inFlightGauge returns the current value of the in-flight gauge.
func inFlightGauge(t *testing.T, m *observability.Metrics) float64 {
	t.Helper()
	families, err := m.Gatherer().Gather()
	if err != nil {
		t.Fatalf("Gather() error: %v", err)
	}
	for _, f := range families {
		if f.GetName() != "alpha_proxy_http_in_flight_requests" {
			continue
		}
		for _, metric := range f.GetMetric() {
			return metric.GetGauge().GetValue()
		}
	}
	return 0
}

// timeoutsCounter returns the value of the HTTP timeouts counter.
func timeoutsCounter(t *testing.T, m *observability.Metrics) float64 {
	t.Helper()
	families, err := m.Gatherer().Gather()
	if err != nil {
		t.Fatalf("Gather() error: %v", err)
	}
	for _, f := range families {
		if f.GetName() != "alpha_proxy_http_timeouts_total" {
			continue
		}
		for _, metric := range f.GetMetric() {
			return metric.GetCounter().GetValue()
		}
	}
	return 0
}

// metricHasLabels reports whether the label pairs contain all the wanted pairs.
func metricHasLabels[T interface {
	GetName() string
	GetValue() string
}](pairs []T, want map[string]string) bool {
	for k, v := range want {
		found := false
		for _, lp := range pairs {
			if lp.GetName() == k && lp.GetValue() == v {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
