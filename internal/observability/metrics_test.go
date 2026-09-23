package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewMetricsSucceeds(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}
	if m == nil {
		t.Fatal("NewMetrics() returned nil")
	}
}

func TestNewMetricsIndependentInstances(t *testing.T) {
	m1, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() 1 error: %v", err)
	}
	m2, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() 2 error: %v", err)
	}

	// Observing on one instance must not affect the other.
	m1.ObserveHTTPRequest("POST", "POST /process", 200, 10*time.Millisecond)
	if got := testutil.ToFloat64(m2.httpRequestsTotal.WithLabelValues("POST", "POST /process", "200")); got != 0 {
		t.Errorf("instance 2 counter = %v, want 0 (instances share state)", got)
	}
	if got := testutil.ToFloat64(m1.httpRequestsTotal.WithLabelValues("POST", "POST /process", "200")); got != 1 {
		t.Errorf("instance 1 counter = %v, want 1", got)
	}
}

func TestObserveHTTPRequest(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	const duration = 100 * time.Millisecond
	m.ObserveHTTPRequest("POST", "POST /process", 200, duration)

	if got := testutil.ToFloat64(m.httpRequestsTotal.WithLabelValues("POST", "POST /process", "200")); got != 1 {
		t.Errorf("counter = %v, want 1", got)
	}
	count, sum := histogramStats(t, m, "alpha_proxy_http_request_duration_seconds",
		map[string]string{"method": "POST", "route": "POST /process"})
	if count != 1 {
		t.Errorf("histogram sample_count = %d, want 1", count)
	}
	if sum != duration.Seconds() {
		t.Errorf("histogram sample_sum = %v, want %v", sum, duration.Seconds())
	}
}

func TestObserveHTTPRequestNormalizesUnknown(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	m.ObserveHTTPRequest("DELETE", "/unknown", 200, time.Millisecond)

	if got := testutil.ToFloat64(m.httpRequestsTotal.WithLabelValues("OTHER", "unmatched", "200")); got != 1 {
		t.Errorf("normalized counter = %v, want 1", got)
	}
}

func TestObserveHTTPRequestInvalidStatus(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	m.ObserveHTTPRequest("POST", "POST /process", 999, time.Millisecond)

	if got := testutil.ToFloat64(m.httpRequestsTotal.WithLabelValues("POST", "POST /process", "unknown")); got != 1 {
		t.Errorf("invalid status counter = %v, want 1", got)
	}
}

func TestInFlightGauge(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	m.IncHTTPInFlight()
	if got := testutil.ToFloat64(m.httpInFlightRequests); got != 1 {
		t.Errorf("in-flight = %v, want 1", got)
	}
	m.DecHTTPInFlight()
	if got := testutil.ToFloat64(m.httpInFlightRequests); got != 0 {
		t.Errorf("in-flight = %v, want 0", got)
	}
}

func TestIncHTTPTimeout(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	m.IncHTTPTimeout()
	if got := testutil.ToFloat64(m.httpTimeoutsTotal); got != 1 {
		t.Errorf("timeouts = %v, want 1", got)
	}
}

func TestObserveProcessorOutcomes(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	const duration = 50 * time.Millisecond
	for _, outcome := range []string{"success", "unavailable", "timeout", "error"} {
		m.ObserveProcessor(outcome, duration)
		if got := testutil.ToFloat64(m.processorCallsTotal.WithLabelValues(outcome)); got != 1 {
			t.Errorf("outcome %q counter = %v, want 1", outcome, got)
		}
		count, sum := histogramStats(t, m, "alpha_proxy_processor_duration_seconds",
			map[string]string{"outcome": outcome})
		if count != 1 {
			t.Errorf("outcome %q histogram sample_count = %d, want 1", outcome, count)
		}
		if sum != duration.Seconds() {
			t.Errorf("outcome %q histogram sample_sum = %v, want %v", outcome, sum, duration.Seconds())
		}
	}
}

func TestObserveProcessorUnknownOutcome(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	m.ObserveProcessor("weird", time.Millisecond)
	if got := testutil.ToFloat64(m.processorCallsTotal.WithLabelValues("error")); got != 1 {
		t.Errorf("unknown outcome counter = %v, want 1", got)
	}
}

func TestObservePIIEntity(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	m.ObservePIIEntity("phone")
	m.ObservePIIEntity("phone")
	m.ObservePIIEntity("email")

	if got := testutil.ToFloat64(m.piiEntitiesTotal.WithLabelValues("phone")); got != 2 {
		t.Errorf("phone counter = %v, want 2", got)
	}
	if got := testutil.ToFloat64(m.piiEntitiesTotal.WithLabelValues("email")); got != 1 {
		t.Errorf("email counter = %v, want 1", got)
	}
}

func TestObserveMLRequest(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	const duration = 30 * time.Millisecond
	m.ObserveMLRequest("success", duration)
	m.ObserveMLRequest("error", duration)

	if got := testutil.ToFloat64(m.mlRequestsTotal.WithLabelValues("success")); got != 1 {
		t.Errorf("success counter = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.mlRequestsTotal.WithLabelValues("error")); got != 1 {
		t.Errorf("error counter = %v, want 1", got)
	}
	count, sum := histogramStats(t, m, "alpha_proxy_ml_request_duration_seconds",
		map[string]string{"outcome": "success"})
	if count != 1 {
		t.Errorf("success histogram sample_count = %d, want 1", count)
	}
	if sum != duration.Seconds() {
		t.Errorf("success histogram sample_sum = %v, want %v", sum, duration.Seconds())
	}
}

func TestObserveMLRequestNormalizesOutcome(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	m.ObserveMLRequest("weird", time.Millisecond)
	if got := testutil.ToFloat64(m.mlRequestsTotal.WithLabelValues("error")); got != 1 {
		t.Errorf("unknown outcome counter = %v, want 1", got)
	}
}

func TestObserveCascadeRoute(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	for _, route := range []string{"safe", "rule", "ml"} {
		m.ObserveCascadeRoute(route)
		if got := testutil.ToFloat64(m.cascadeRoutesTotal.WithLabelValues(route)); got != 1 {
			t.Errorf("route %q counter = %v, want 1", route, got)
		}
	}
}

func TestObserveCascadeRouteNormalizesUnknown(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	m.ObserveCascadeRoute("bogus")
	if got := testutil.ToFloat64(m.cascadeRoutesTotal.WithLabelValues("safe")); got != 1 {
		t.Errorf("unknown route counter = %v, want 1", got)
	}
}

func TestHandlerExpositionFormat(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}
	m.ObserveHTTPRequest("POST", "POST /process", 200, time.Millisecond)
	m.ObserveProcessor("success", time.Millisecond)
	m.ObservePIIEntity("phone")
	m.ObserveMLRequest("success", time.Millisecond)
	m.ObserveCascadeRoute("ml")

	handler := m.Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") && !strings.HasPrefix(ct, "application/openmetrics-text") {
		t.Errorf("Content-Type = %q, want Prometheus/OpenMetrics text exposition media type", ct)
	}
	body := rec.Body.String()
	for _, name := range []string{
		"alpha_proxy_http_requests_total",
		"alpha_proxy_http_request_duration_seconds",
		"alpha_proxy_http_in_flight_requests",
		"alpha_proxy_http_timeouts_total",
		"alpha_proxy_processor_calls_total",
		"alpha_proxy_processor_duration_seconds",
		"alpha_proxy_pii_entities_total",
		"alpha_proxy_ml_requests_total",
		"alpha_proxy_ml_request_duration_seconds",
		"alpha_proxy_cascade_routes_total",
	} {
		if !strings.Contains(body, name) {
			t.Errorf("handler output missing metric %q", name)
		}
	}
}

// histogramStats returns the sample count and sum of a histogram metric family
// matching the given labels, gathered from the instance's registry.
func histogramStats(t *testing.T, m *Metrics, familyName string, wantLabels map[string]string) (uint64, float64) {
	t.Helper()
	families, err := m.Gatherer().Gather()
	if err != nil {
		t.Fatalf("Gather() error: %v", err)
	}
	for _, f := range families {
		if f.GetName() != familyName {
			continue
		}
		for _, metric := range f.GetMetric() {
			match := true
			for _, lp := range metric.GetLabel() {
				if wantVal, ok := wantLabels[lp.GetName()]; ok && lp.GetValue() != wantVal {
					match = false
					break
				}
			}
			if !match {
				continue
			}
			h := metric.GetHistogram()
			return h.GetSampleCount(), h.GetSampleSum()
		}
	}
	t.Fatalf("metric family %q with labels %v not found", familyName, wantLabels)
	return 0, 0
}
