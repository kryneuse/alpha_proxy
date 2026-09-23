package observability

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/entity"
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

func TestInstrumentedCascadePreservesOriginalAndGate(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatal(err)
	}
	meta := &requestmeta.Meta{}
	ctx := requestmeta.With(context.Background(), meta)
	stub := &stubCascade{}
	runner := NewInstrumentedCascade(stub, m)
	rules, residual := runner.AnalyzeRules("original")
	if len(rules) != 1 || residual != "residual" {
		t.Fatal("rule results changed")
	}
	if _, err := runner.DetectChunk(ctx, "original", "residual"); err != nil {
		t.Fatal(err)
	}
	if stub.original != "original" || stub.gate != "residual" {
		t.Fatal("chunk contexts changed")
	}
	if got := testutil.ToFloat64(m.cascadeRoutesTotal.WithLabelValues("ml")); got != 1 {
		t.Fatalf("ml count=%v", got)
	}
	if !meta.MLInvoked() {
		t.Fatal("ML invocation was not recorded")
	}
}

func TestInstrumentedCascadeNilMetricsPassthrough(t *testing.T) {
	stub := &stubCascade{}
	if NewInstrumentedCascade(stub, nil) != stub {
		t.Fatal("nil metrics should pass through")
	}
}

func TestInstrumentedCascadeMarksMLInvokedOnError(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatal(err)
	}
	meta := &requestmeta.Meta{}
	ctx := requestmeta.With(context.Background(), meta)
	stub := &stubCascade{err: errors.New("ML failed")}
	runner := NewInstrumentedCascade(stub, m)
	if _, err := runner.DetectChunk(ctx, "original", "residual"); err == nil {
		t.Fatal("expected error")
	}
	if !meta.MLInvoked() {
		t.Fatal("failed ML call was not recorded")
	}
}

func TestInstrumentedMLClientPreservesV2(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatal(err)
	}
	stub := &stubMLClient{}
	client := NewInstrumentedMLClient(stub, m)
	req := ml.BatchRequest{BatchID: "b2", Items: []ml.RequestItem{{ChunkID: "c", Text: "original", GateText: "residual"}}}
	if _, err := client.ProcessBatchV2(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if stub.v2 == nil || stub.v2.Items[0].Text != "original" || stub.v2.Items[0].GateText != "residual" {
		t.Fatal("V2 context lost in metrics wrapper")
	}
	if got := testutil.ToFloat64(m.mlRequestsTotal.WithLabelValues("success")); got != 1 {
		t.Fatalf("request count=%v", got)
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
	v2   *ml.BatchRequest
	resp ml.BatchResponse
	err  error
}

func (s *stubMLClient) ProcessBatch(_ context.Context, _ ml.BatchRequest) (ml.BatchResponse, error) {
	return s.resp, s.err
}

func (s *stubMLClient) ProcessBatchV2(_ context.Context, req ml.BatchRequest) (ml.BatchResponse, error) {
	s.v2 = &req
	return s.resp, s.err
}

type stubCascade struct {
	original, gate string
	err            error
}

func (s *stubCascade) AnalyzeRules(_ string) ([]entity.Entity, string) {
	return []entity.Entity{{Type: entity.EMAIL}}, "residual"
}
func (s *stubCascade) DetectChunk(_ context.Context, original, gate string) ([]entity.Entity, error) {
	s.original, s.gate = original, gate
	return nil, s.err
}
