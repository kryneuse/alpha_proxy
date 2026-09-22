package observability

import (
	"context"
	"errors"
	"time"

	"alpha_proxy/internal/contract"
)

// NewInstrumentedProcessor wraps next so that every Process call is timed and
// recorded by outcome. It returns a contract.Processor that forwards the
// original context, request, response and error unchanged. The outcome is
// derived only from the returned error and the context; no payload, payload_id,
// result, consumer ID or error text is ever used as a metric label.
func NewInstrumentedProcessor(next contract.Processor, metrics *Metrics) (contract.Processor, error) {
	return newInstrumentedProcessor(next, metrics, time.Now)
}

// newInstrumentedProcessor is the internal constructor used by tests to inject
// a deterministic clock. It validates all arguments and never panics.
func newInstrumentedProcessor(next contract.Processor, metrics *Metrics, now func() time.Time) (contract.Processor, error) {
	if next == nil {
		return nil, errors.New("observability: instrumented processor requires a non-nil processor")
	}
	if metrics == nil {
		return nil, errors.New("observability: instrumented processor requires non-nil metrics")
	}
	if now == nil {
		return nil, errors.New("observability: instrumented processor requires a non-nil clock function")
	}
	return &instrumentedProcessor{next: next, metrics: metrics, now: now}, nil
}

// instrumentedProcessor decorates a Processor with metrics observation. It
// contains no masking or data-protection logic and spawns no goroutine.
type instrumentedProcessor struct {
	next    contract.Processor
	metrics *Metrics
	now     func() time.Time
}

// Process forwards the call unchanged and records exactly one observation of
// outcome and duration. The outcome defaults to "error" and is only replaced
// after a normal return, so a panic from the wrapped Processor is recorded as
// "error" while the original panic continues to propagate.
func (o *instrumentedProcessor) Process(ctx context.Context, req contract.ProcessRequest) (resp contract.ProcessResponse, err error) {
	start := o.now()
	outcome := "error"

	defer func() {
		o.metrics.ObserveProcessor(outcome, o.now().Sub(start))
	}()

	resp, err = o.next.Process(ctx, req)
	outcome = classifyOutcome(ctx, err)
	return resp, err
}

// classifyOutcome maps an error to a safe outcome label. Priority: success,
// unavailable, timeout, error.
func classifyOutcome(ctx context.Context, err error) string {
	switch {
	case err == nil:
		return "success"
	case errors.Is(err, contract.ErrUnavailable):
		return "unavailable"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled) && ctx.Err() != nil:
		return "timeout"
	default:
		return "error"
	}
}
