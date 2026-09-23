package observability

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/cascade"
	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/gate"
	"github.com/kryneuse/alpha_proxy/internal/ml"
	"github.com/kryneuse/alpha_proxy/internal/pii"
	"github.com/kryneuse/alpha_proxy/internal/requestmeta"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestInstrumentedMaskerRecordsEntitiesAndMeta(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	meta := &requestmeta.Meta{}
	ctx := requestmeta.With(context.Background(), meta)

	stub := &stubMasker{mappings: []pii.TokenMapping{
		{Kind: pii.PIIKindPhone},
		{Kind: pii.PIIKindPhone},
		{Kind: pii.PIIKindEmail},
	}}
	masker := NewInstrumentedMasker(stub, m)

	masked, mappings, err := masker.Mask(ctx, "raw", pii.Policy{})
	if err != nil {
		t.Fatalf("Mask() error: %v", err)
	}
	if masked != "masked" {
		t.Errorf("masked = %q, want masked", masked)
	}
	if len(mappings) != 3 {
		t.Errorf("mappings = %d, want 3", len(mappings))
	}

	if got := testutil.ToFloat64(m.piiEntitiesTotal.WithLabelValues("phone")); got != 2 {
		t.Errorf("phone counter = %v, want 2", got)
	}
	if got := testutil.ToFloat64(m.piiEntitiesTotal.WithLabelValues("email")); got != 1 {
		t.Errorf("email counter = %v, want 1", got)
	}

	if meta.PIICount != 3 {
		t.Errorf("PIICount = %d, want 3", meta.PIICount)
	}
	if len(meta.PIITypes) != 2 {
		t.Fatalf("PIITypes = %v, want 2 unique types", meta.PIITypes)
	}
	joined := strings.Join(meta.PIITypes, ",")
	if !strings.Contains(joined, "phone") || !strings.Contains(joined, "email") {
		t.Errorf("PIITypes = %v, want phone and email", meta.PIITypes)
	}
}

func TestInstrumentedMaskerNilMetricsPassthrough(t *testing.T) {
	stub := &stubMasker{mappings: []pii.TokenMapping{{Kind: pii.PIIKindPhone}}}
	masker := NewInstrumentedMasker(stub, nil)
	if masker != stub {
		t.Fatal("NewInstrumentedMasker with nil metrics must return next unchanged")
	}
}

func TestInstrumentedMLClientRecordsOutcome(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	stub := &stubMLClient{resp: ml.BatchResponse{BatchID: "b1"}}
	client := NewInstrumentedMLClient(stub, m)

	_, err = client.ProcessBatch(context.Background(), ml.BatchRequest{BatchID: "b1"})
	if err != nil {
		t.Fatalf("ProcessBatch() error: %v", err)
	}
	if got := testutil.ToFloat64(m.mlRequestsTotal.WithLabelValues("success")); got != 1 {
		t.Errorf("success counter = %v, want 1", got)
	}
	count, _ := histogramStats(t, m, "alpha_proxy_ml_request_duration_seconds",
		map[string]string{"outcome": "success"})
	if count != 1 {
		t.Errorf("success histogram sample_count = %d, want 1", count)
	}
}

func TestInstrumentedMLClientRecordsError(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	stub := &stubMLClient{err: errors.New("ml down")}
	client := NewInstrumentedMLClient(stub, m)

	_, err = client.ProcessBatch(context.Background(), ml.BatchRequest{})
	if err == nil {
		t.Fatal("ProcessBatch() error = nil, want error")
	}
	if got := counterValue(t, m, "alpha_proxy_ml_requests_total", "error"); got != 1 {
		t.Errorf("error counter = %v, want 1", got)
	}
}

func TestInstrumentedCascadeRecordsRoutes(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	meta := &requestmeta.Meta{}
	ctx := requestmeta.With(context.Background(), meta)

	stub := &stubCascade{results: []cascade.Result{
		{Route: gate.SAFE},
		{Route: gate.LIKELY_PII, ExpensiveInvoked: true},
		{Route: gate.SAFE, Entities: []entity.Entity{{Type: entity.PHONE}}},
	}}
	runner := NewInstrumentedCascade(stub, m)

	for range stub.results {
		if _, err := runner.Run(ctx, "text"); err != nil {
			t.Fatalf("Run() error: %v", err)
		}
	}

	if got := testutil.ToFloat64(m.cascadeRoutesTotal.WithLabelValues("safe")); got != 1 {
		t.Errorf("safe counter = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.cascadeRoutesTotal.WithLabelValues("ml")); got != 1 {
		t.Errorf("ml counter = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.cascadeRoutesTotal.WithLabelValues("rule")); got != 1 {
		t.Errorf("rule counter = %v, want 1", got)
	}
	if !meta.MLInvoked() {
		t.Error("MLInvoked = false, want true")
	}
}

func TestInstrumentedCascadeNilMetricsPassthrough(t *testing.T) {
	stub := &stubCascade{}
	runner := NewInstrumentedCascade(stub, nil)
	if runner != stub {
		t.Fatal("NewInstrumentedCascade with nil metrics must return next unchanged")
	}
}

func TestInstrumentedCascadeMarksMLInvokedOnError(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	meta := &requestmeta.Meta{}
	ctx := requestmeta.With(context.Background(), meta)

	// ML was actually invoked (ExpensiveInvoked) but the runner returned an error.
	stub := &stubCascade{
		results: []cascade.Result{{Route: gate.LIKELY_PII, ExpensiveInvoked: true}},
		errs:    []error{errors.New("ml failed")},
	}
	runner := NewInstrumentedCascade(stub, m)

	if _, err := runner.Run(ctx, "text"); err == nil {
		t.Fatal("Run() error = nil, want error")
	}

	if got := testutil.ToFloat64(m.cascadeRoutesTotal.WithLabelValues("ml")); got != 1 {
		t.Errorf("ml counter = %v, want 1", got)
	}
	if !meta.MLInvoked() {
		t.Error("MLInvoked = false, want true even when Run returned an error")
	}
}

// stubMasker returns a fixed masked text and mappings.
type stubMasker struct {
	mappings []pii.TokenMapping
}

func (s *stubMasker) Mask(_ context.Context, _ string, _ pii.Policy) (string, []pii.TokenMapping, error) {
	return "masked", s.mappings, nil
}

// stubMLClient returns a fixed response or error.
type stubMLClient struct {
	resp ml.BatchResponse
	err  error
}

func (s *stubMLClient) ProcessBatch(_ context.Context, _ ml.BatchRequest) (ml.BatchResponse, error) {
	return s.resp, s.err
}

// stubCascade returns a fixed sequence of results and optional errors.
type stubCascade struct {
	results []cascade.Result
	errs    []error
	idx     int
}

func (s *stubCascade) Run(_ context.Context, _ string) (cascade.Result, error) {
	if s.idx >= len(s.results) {
		return cascade.Result{}, nil
	}
	res := s.results[s.idx]
	var err error
	if s.idx < len(s.errs) {
		err = s.errs[s.idx]
	}
	s.idx++
	return res, err
}
