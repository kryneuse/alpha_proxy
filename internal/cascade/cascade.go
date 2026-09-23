// Package cascade implements the routing cascade:
//
//	payload -> Rule Engine (whole text) -> rule_entities + residual
//	       -> chunk original -> (original_text, gate_text) pairs -> ML
//
// The gate and NER both run inside the Python ML service in a single RPC. Go
// does not run a separate gate: it sends each (original_text, gate_text) pair
// and Python decides whether NER is needed. A positive gate means "the residual
// may still contain PII"; it returns no entities. On a negative gate the
// rule-found PII stays in the result.
package cascade

import (
	"context"

	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/residual"
)

// ExpensiveExtractor is the ML extractor. It receives the original text of a
// chunk and the byte-preserving masked residual (gate_text) of the same chunk.
// The gate and NER run inside the ML service; the extractor returns entities
// with offsets relative to original.
type ExpensiveExtractor interface {
	Detect(ctx context.Context, original, gate string) ([]entity.Entity, error)
}

// RuleEngine is the deterministic rule engine used as the first stage.
type RuleEngine interface {
	Analyze(text string) []entity.Entity
}

// Cascade runs the rule engine on the whole text and the ML extractor on
// (original_text, gate_text) chunk pairs.
type Cascade struct {
	engine    RuleEngine
	expensive ExpensiveExtractor
}

// New builds a cascade. expensive may be nil; if a chunk requires it and it is
// nil, the cascade fails closed.
func New(engine RuleEngine, expensive ExpensiveExtractor) *Cascade {
	return &Cascade{
		engine:    engine,
		expensive: expensive,
	}
}

// AnalyzeRules runs the rule engine on the whole text and returns the rule
// entities and the residual text (byte-preserving masked).
func (c *Cascade) AnalyzeRules(text string) ([]entity.Entity, string) {
	ruleEntities := c.engine.Analyze(text)
	residualText := residual.Build(text, ruleEntities)
	return ruleEntities, residualText
}

// DetectChunk runs the ML extractor on a (original_text, gate_text) pair and
// returns ML entities with offsets relative to original.
func (c *Cascade) DetectChunk(ctx context.Context, original, gate string) ([]entity.Entity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.expensive == nil {
		return nil, errFailClosed("expensive extractor unavailable")
	}
	return c.expensive.Detect(ctx, original, gate)
}

// errFailClosed is a sentinel error for fail-closed situations.
type errFailClosed string

func (e errFailClosed) Error() string { return string(e) }