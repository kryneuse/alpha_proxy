package observability

import (
	"context"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/cascade"
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

// ProcessBatch forwards the call unchanged and records outcome and duration.
func (c *InstrumentedMLClient) ProcessBatch(ctx context.Context, req ml.BatchRequest) (ml.BatchResponse, error) {
	start := time.Now()
	resp, err := c.next.ProcessBatch(ctx, req)
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	c.metrics.ObserveMLRequest(outcome, time.Since(start))
	return resp, err
}

// InstrumentedCascade wraps a cascade runner to record routing decisions and to
// mark when the ML service was invoked. It never reads the chunk text.
type InstrumentedCascade struct {
	next    cascadeRunner
	metrics *Metrics
}

// cascadeRunner is the subset of the cascade interface used by the masker.
type cascadeRunner interface {
	Run(ctx context.Context, text string) (cascade.Result, error)
}

// NewInstrumentedCascade wraps next with metrics observation. A nil metrics
// disables observation and returns next unchanged.
func NewInstrumentedCascade(next cascadeRunner, metrics *Metrics) cascadeRunner {
	if metrics == nil {
		return next
	}
	return &InstrumentedCascade{next: next, metrics: metrics}
}

// Run forwards the call unchanged and records the routing decision. The route
// and ml_invoked flag are recorded even when the underlying runner returns an
// error, as long as the ML service was actually invoked.
func (c *InstrumentedCascade) Run(ctx context.Context, text string) (cascade.Result, error) {
	res, err := c.next.Run(ctx, text)
	c.metrics.ObserveCascadeRoute(classifyRoute(res))
	if res.ExpensiveInvoked {
		if meta := requestmeta.From(ctx); meta != nil {
			meta.SetMLInvoked()
		}
	}
	return res, err
}

// classifyRoute maps a cascade result to one of "safe", "rule" or "ml".
func classifyRoute(res cascade.Result) string {
	if res.ExpensiveInvoked {
		return "ml"
	}
	if len(res.Entities) > 0 {
		return "rule"
	}
	return "safe"
}
