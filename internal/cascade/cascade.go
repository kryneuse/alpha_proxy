// Package cascade implements the routing cascade:
//
//	chunk -> Rule Engine -> residual -> Heuristic Gate -> routing
//
// The gate routes to SAFE / UNCERTAIN / LIKELY_PII. UNCERTAIN consults a cheap
// ML classifier; LIKELY_PII goes straight to an expensive ML extractor. The
// interfaces for the ML components are defined here so real models can be
// plugged in later without changing the cascade.
package cascade

import (
	"context"

	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/gate"
	"github.com/kryneuse/alpha_proxy/internal/residual"
)

// CheapClassifier is a cheap ML classifier that decides whether a residual
// text may contain PII. It returns a score in [0,1].
type CheapClassifier interface {
	HasPII(ctx context.Context, text string) (score float64, err error)
}

// ExpensiveExtractor is an expensive ML extractor that detects PII entities
// in a residual text.
type ExpensiveExtractor interface {
	Detect(ctx context.Context, text string) ([]entity.Entity, error)
}

// RuleEngine is the deterministic rule engine used as the first stage.
type RuleEngine interface {
	Analyze(text string) []entity.Entity
}

// Result is the final output of the cascade.
type Result struct {
	// Entities is the merged set of rule + ML entities.
	Entities []entity.Entity
	// Residual is the residual text after rule spans were masked.
	Residual string
	// Route is the gate routing decision.
	Route gate.Route
	// GateScore is the heuristic gate score.
	GateScore float64
	// CheapInvoked reports whether the cheap classifier was called.
	CheapInvoked bool
	// ExpensiveInvoked reports whether the expensive extractor was called.
	ExpensiveInvoked bool
}

// Cascade runs the full routing pipeline.
type Cascade struct {
	engine    RuleEngine
	gate      *gate.Gate
	cheap     CheapClassifier
	expensive ExpensiveExtractor
}

// New builds a cascade. cheap and expensive may be nil; if a route requires
// them and they are nil, the cascade fails closed (returns an error).
func New(engine RuleEngine, g *gate.Gate, cheap CheapClassifier, expensive ExpensiveExtractor) *Cascade {
	return &Cascade{
		engine:    engine,
		gate:      g,
		cheap:     cheap,
		expensive: expensive,
	}
}

// Run executes the cascade on a chunk and returns the merged result.
func (c *Cascade) Run(ctx context.Context, chunk string) (Result, error) {
	// 1. Run the rule engine.
	ruleEntities := c.engine.Analyze(chunk)

	// 2. Build the residual text (rule spans masked with spaces).
	residualText := residual.Build(chunk, ruleEntities)

	// 3. Run the heuristic gate.
	decision := c.gate.Evaluate(residualText)

	res := Result{
		Entities:  ruleEntities,
		Residual:  residualText,
		Route:     decision.Route,
		GateScore: decision.Score,
	}

	switch decision.Route {
	case gate.SAFE:
		// No further analysis.
		return res, nil

	case gate.UNCERTAIN:
		// Consult the cheap classifier.
		if c.cheap == nil {
			// Fail closed: we cannot confirm the residual is clean.
			return res, errFailClosed("cheap classifier unavailable for UNCERTAIN route")
		}
		res.CheapInvoked = true
		score, err := c.cheap.HasPII(ctx, residualText)
		if err != nil {
			// Fail closed: route to the expensive extractor.
			return c.runExpensive(ctx, res)
		}
		if score < 0.5 {
			// Cheap negative: residual considered clean.
			return res, nil
		}
		// Cheap positive: run the expensive extractor.
		return c.runExpensive(ctx, res)

	case gate.LIKELY_PII:
		// Skip the cheap classifier, run the expensive extractor directly.
		return c.runExpensive(ctx, res)

	default:
		return res, errFailClosed("unknown gate route: " + string(decision.Route))
	}
}

// runExpensive runs the expensive extractor and merges its entities with the
// rule entities.
func (c *Cascade) runExpensive(ctx context.Context, res Result) (Result, error) {
	if c.expensive == nil {
		return res, errFailClosed("expensive extractor unavailable")
	}
	res.ExpensiveInvoked = true
	mlEntities, err := c.expensive.Detect(ctx, res.Residual)
	if err != nil {
		return res, err
	}
	// Merge rule + ML entities. The resolver/deduplication is applied by the
	// caller or a shared resolver; here we concatenate and let the caller
	// resolve overlaps. For now, append ML entities.
	res.Entities = append(res.Entities, mlEntities...)
	return res, nil
}

// errFailClosed is a sentinel error for fail-closed situations.
type errFailClosed string

func (e errFailClosed) Error() string { return string(e) }
