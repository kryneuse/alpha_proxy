// Package observability provides structured logging and Prometheus metrics for
// the HTTP contour. Metrics use an isolated registry per instance; no global
// registry or global mutable state is used.
package observability

import (
	"fmt"
	"net/http"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/pii"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// httpDurationBuckets returns a fresh slice of buckets covering HTTP latency
// from milliseconds to a few seconds.
func httpDurationBuckets() []float64 {
	return []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}
}

// Metrics owns an isolated Prometheus registry and the service metrics.
type Metrics struct {
	registry *prometheus.Registry

	httpRequestsTotal        *prometheus.CounterVec
	httpRequestDuration      *prometheus.HistogramVec
	httpInFlightRequests     prometheus.Gauge
	httpTimeoutsTotal        prometheus.Counter
	processorCallsTotal      *prometheus.CounterVec
	processorDurationSeconds *prometheus.HistogramVec

	piiEntitiesTotal   *prometheus.CounterVec
	mlRequestsTotal    *prometheus.CounterVec
	mlRequestDuration  *prometheus.HistogramVec
	cascadeRoutesTotal *prometheus.CounterVec
}

// NewMetrics creates a Metrics instance with its own registry and registers the
// service metrics. It never touches the default registry and never panics.
func NewMetrics() (*Metrics, error) {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		httpRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "alpha_proxy_http_requests_total",
			Help: "Total number of HTTP requests processed by alpha_proxy.",
		}, []string{"method", "route", "status"}),
		httpRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "alpha_proxy_http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds.",
			Buckets: httpDurationBuckets(),
		}, []string{"method", "route"}),
		httpInFlightRequests: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "alpha_proxy_http_in_flight_requests",
			Help: "Number of HTTP requests currently in flight.",
		}),
		httpTimeoutsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "alpha_proxy_http_timeouts_total",
			Help: "Total number of HTTP requests that timed out.",
		}),
		processorCallsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "alpha_proxy_processor_calls_total",
			Help: "Total number of Processor calls by outcome.",
		}, []string{"outcome"}),
		processorDurationSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "alpha_proxy_processor_duration_seconds",
			Help:    "Duration of Processor calls in seconds by outcome.",
			Buckets: httpDurationBuckets(),
		}, []string{"outcome"}),
		piiEntitiesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "alpha_proxy_pii_entities_total",
			Help: "Total number of PII entities found by kind.",
		}, []string{"kind"}),
		mlRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "alpha_proxy_ml_requests_total",
			Help: "Total number of ML requests by outcome.",
		}, []string{"outcome"}),
		mlRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "alpha_proxy_ml_request_duration_seconds",
			Help:    "Duration of ML requests in seconds by outcome.",
			Buckets: httpDurationBuckets(),
		}, []string{"outcome"}),
		cascadeRoutesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "alpha_proxy_cascade_routes_total",
			Help: "Total number of cascade routing decisions by route.",
		}, []string{"route"}),
	}

	collectors := []prometheus.Collector{
		m.httpRequestsTotal,
		m.httpRequestDuration,
		m.httpInFlightRequests,
		m.httpTimeoutsTotal,
		m.processorCallsTotal,
		m.processorDurationSeconds,
		m.piiEntitiesTotal,
		m.mlRequestsTotal,
		m.mlRequestDuration,
		m.cascadeRoutesTotal,
	}
	for _, c := range collectors {
		if err := m.registry.Register(c); err != nil {
			return nil, fmt.Errorf("observability: register metric: %w", err)
		}
	}
	return m, nil
}

// ObserveHTTPRequest records an HTTP request with normalized method, route and
// status.
func (m *Metrics) ObserveHTTPRequest(method string, route string, status int, duration time.Duration) {
	method = normalizeMethod(method)
	route = normalizeRoute(route)
	statusStr := normalizeStatus(status)
	m.httpRequestsTotal.WithLabelValues(method, route, statusStr).Inc()
	m.httpRequestDuration.WithLabelValues(method, route).Observe(duration.Seconds())
}

// IncHTTPInFlight increments the in-flight gauge.
func (m *Metrics) IncHTTPInFlight() {
	m.httpInFlightRequests.Inc()
}

// DecHTTPInFlight decrements the in-flight gauge.
func (m *Metrics) DecHTTPInFlight() {
	m.httpInFlightRequests.Dec()
}

// IncHTTPTimeout increments the timeout counter.
func (m *Metrics) IncHTTPTimeout() {
	m.httpTimeoutsTotal.Inc()
}

// ObserveProcessor records a Processor call with a normalized outcome.
func (m *Metrics) ObserveProcessor(outcome string, duration time.Duration) {
	outcome = normalizeOutcome(outcome)
	m.processorCallsTotal.WithLabelValues(outcome).Inc()
	m.processorDurationSeconds.WithLabelValues(outcome).Observe(duration.Seconds())
}

// ObservePIIEntity records one found PII entity of the given kind. The kind is
// normalized to a safe label; unknown kinds fall back to "unknown".
func (m *Metrics) ObservePIIEntity(kind string) {
	m.piiEntitiesTotal.WithLabelValues(normalizeKind(kind)).Inc()
}

// ObserveMLRequest records an ML request with a normalized outcome and duration.
func (m *Metrics) ObserveMLRequest(outcome string, duration time.Duration) {
	outcome = normalizeMLOutcome(outcome)
	m.mlRequestsTotal.WithLabelValues(outcome).Inc()
	m.mlRequestDuration.WithLabelValues(outcome).Observe(duration.Seconds())
}

// ObserveCascadeRoute records a cascade routing decision. The route is
// normalized to one of "safe", "rule" or "ml".
func (m *Metrics) ObserveCascadeRoute(route string) {
	m.cascadeRoutesTotal.WithLabelValues(normalizeCascadeRoute(route)).Inc()
}

// Handler returns an HTTP handler serving the metrics in Prometheus exposition
// format from this instance's isolated registry.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Gatherer returns the isolated registry as a read-only gatherer.
func (m *Metrics) Gatherer() prometheus.Gatherer {
	return m.registry
}

func normalizeMethod(method string) string {
	switch method {
	case "GET", "POST":
		return method
	default:
		return "OTHER"
	}
}

func normalizeRoute(route string) string {
	switch route {
	case "POST /process", "GET /metrics", "GET /healthz", "GET /readyz":
		return route
	default:
		return "unmatched"
	}
}

func normalizeStatus(status int) string {
	if status >= 100 && status <= 599 {
		return fmt.Sprintf("%d", status)
	}
	return "unknown"
}

func normalizeOutcome(outcome string) string {
	switch outcome {
	case "success", "unavailable", "timeout":
		return outcome
	default:
		return "error"
	}
}

// normalizeMLOutcome maps an ML request outcome to a safe label. Only "success"
// and "error" are allowed.
func normalizeMLOutcome(outcome string) string {
	if outcome == "success" {
		return "success"
	}
	return "error"
}

// normalizeKind maps a PII kind to a safe label. Only known PIIKind string
// values are allowed; anything else falls back to "unknown" so no raw value
// ever becomes a metric label.
func normalizeKind(kind string) string {
	switch pii.PIIKind(kind) {
	case pii.PIIKindFullName, pii.PIIKindFirstName, pii.PIIKindLastName,
		pii.PIIKindMiddleName, pii.PIIKindAddress, pii.PIIKindCity,
		pii.PIIKindStreet, pii.PIIKindHouse, pii.PIIKindApartment,
		pii.PIIKindBirthPlace, pii.PIIKindCitizenship, pii.PIIKindPassportIssuer,
		pii.PIIKindCardHolderName, pii.PIIKindEmail, pii.PIIKindPhone,
		pii.PIIKindINN, pii.PIIKindBankCard, pii.PIIKindPassport,
		pii.PIIKindPassportDivision, pii.PIIKindDate, pii.PIIKindDriverLicense,
		pii.PIIKindCVV, pii.PIIKindPIN, pii.PIIKindPostalCode:
		return kind
	default:
		return "unknown"
	}
}

// normalizeCascadeRoute maps a cascade routing decision to one of "safe",
// "rule" or "ml". Unknown values fall back to "safe".
func normalizeCascadeRoute(route string) string {
	switch route {
	case "safe", "rule", "ml":
		return route
	default:
		return "safe"
	}
}
