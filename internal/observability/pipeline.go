package observability

import (
	"context"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/ml"
	"github.com/kryneuse/alpha_proxy/internal/pii"
	"github.com/kryneuse/alpha_proxy/internal/processor"
	"github.com/kryneuse/alpha_proxy/internal/requestmeta"
)

// InstrumentedMasker wraps a processor.Masker to record PII entity counts per
// kind and to populate request-scoped log metadata. It contains no masking or
// data-protection logic and never reads the payload text.
type InstrumentedMasker struct {
	next    processor.Masker
	metrics *Metrics
}

// NewInstrumentedMasker wraps next with metrics observation. A nil metrics
// disables observation and returns next unchanged.
func NewInstrumentedMasker(next processor.Masker, metrics *Metrics) processor.Masker {
	if metrics == nil {
		return next
	}
	return &InstrumentedMasker{next: next, metrics: metrics}
}

// Mask forwards the call unchanged and records entity counts and log metadata.
func (m *InstrumentedMasker) Mask(ctx context.Context, text string, policy pii.Policy) (string, []pii.TokenMapping, error) {
	masked, mappings, err := m.next.Mask(ctx, text, policy)

	types := make([]string, 0, len(mappings))
	seen := make(map[pii.PIIKind]bool, len(mappings))
	for _, mp := range mappings {
		m.metrics.ObservePIIEntity(string(mp.Kind))
		if !seen[mp.Kind] {
			seen[mp.Kind] = true
			types = append(types, string(mp.Kind))
		}
	}

	if meta := requestmeta.From(ctx); meta != nil {
		meta.PIICount = len(mappings)
		meta.PIITypes = types
	}

	return masked, mappings, err
}

// InstrumentedMLClient wraps an ml.Client to record ML request counts and
// durations. It never reads the request text or response entities.
type InstrumentedMLClient struct {
	next    ml.Client
	metrics *Metrics
}

// NewInstrumentedMLClient wraps next with metrics observation. A nil metrics
// disables observation and returns next unchanged.
func NewInstrumentedMLClient(next ml.Client, metrics *Metrics) ml.Client {
	if metrics == nil {
		return next
	}
	return &InstrumentedMLClient{next: next, metrics: metrics}
}

// ProcessBatch preserves the legacy transport for callers that still use V1.
func (c *InstrumentedMLClient) ProcessBatch(ctx context.Context, req ml.BatchRequest) (ml.BatchResponse, error) {
	return c.observe(ctx, req, c.next.ProcessBatch)
}

// ProcessBatchV2 preserves original/gate text pairs in the adaptive transport.
func (c *InstrumentedMLClient) ProcessBatchV2(ctx context.Context, req ml.BatchRequest) (ml.BatchResponse, error) {
	return c.observe(ctx, req, c.next.ProcessBatchV2)
}

func (c *InstrumentedMLClient) observe(ctx context.Context, req ml.BatchRequest, call func(context.Context, ml.BatchRequest) (ml.BatchResponse, error)) (ml.BatchResponse, error) {
	start := time.Now()
	resp, err := call(ctx, req)
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	c.metrics.ObserveMLRequest(outcome, time.Since(start))
	return resp, err
}

// InstrumentedCascade observes calls to the Python ML service. Actual
// quality/spaCy admission is measured by the Python replica's metrics.
type InstrumentedCascade struct {
	next    cascadeRunner
	metrics *Metrics
}

type cascadeRunner interface {
	AnalyzeRules(text string) ([]entity.Entity, string)
	DetectChunk(ctx context.Context, original, gate string) ([]entity.Entity, error)
}

func NewInstrumentedCascade(next cascadeRunner, metrics *Metrics) cascadeRunner {
	if metrics == nil {
		return next
	}
	return &InstrumentedCascade{next: next, metrics: metrics}
}

func (c *InstrumentedCascade) AnalyzeRules(text string) ([]entity.Entity, string) {
	return c.next.AnalyzeRules(text)
}

func (c *InstrumentedCascade) DetectChunk(ctx context.Context, original, gate string) ([]entity.Entity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.metrics.ObserveCascadeRoute("ml")
	if meta := requestmeta.From(ctx); meta != nil {
		meta.SetMLInvoked()
	}
	return c.next.DetectChunk(ctx, original, gate)
}
