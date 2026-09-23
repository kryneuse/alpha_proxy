package cascade

import "context"

// SmallGate decides whether a residual text may still contain personal data
// and therefore needs the big (expensive) model. It returns an explicit
// decision: yes / no / error. Converting a model probability into this
// decision is the responsibility of the model adapter, not the router.
type SmallGate interface {
	// NeedsNER reports whether the residual text should be sent to the big
	// model. An error means the gate could not decide; the caller must then
	// fall back to the big model.
	NeedsNER(ctx context.Context, residual string) (bool, error)
}

// ProbabilityGate adapts a model that returns a probability in [0,1] into a
// SmallGate. The threshold is applied here, in the adapter, so the router
// never applies an arbitrary threshold itself.
type ProbabilityGate struct {
	// Score returns a probability in [0,1] that the residual contains PII.
	Score func(ctx context.Context, residual string) (float64, error)
	// Threshold is the probability at or above which NeedsNER returns true.
	Threshold float64
}

// NeedsNER implements SmallGate.
func (p *ProbabilityGate) NeedsNER(ctx context.Context, residual string) (bool, error) {
	score, err := p.Score(ctx, residual)
	if err != nil {
		return false, err
	}
	return score >= p.Threshold, nil
}