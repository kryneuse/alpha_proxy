// Package engine wires the full pipeline:
// text -> normalization -> recognizers -> context scoring -> resolver -> []Entity.
package engine

import (
	"github.com/alpha-proxy/rule-engine/internal/context"
	"github.com/alpha-proxy/rule-engine/internal/entity"
	"github.com/alpha-proxy/rule-engine/internal/normalize"
	"github.com/alpha-proxy/rule-engine/internal/recognizer"
	"github.com/alpha-proxy/rule-engine/internal/resolver"
)

// Engine is the top-level rule engine.
type Engine struct {
	registry *recognizer.Registry
	scorer   *context.Scorer
	resolver *resolver.Resolver
}

// Options configures the engine.
type Options struct {
	// MinScore is the minimum entity score to emit.
	MinScore float64
}

// New builds an engine with the default recognizer set.
func New(opts Options) *Engine {
	if opts.MinScore == 0 {
		opts.MinScore = 0.5
	}
	reg := recognizer.NewRegistry(
		recognizer.NewEmailRecognizer(),
		recognizer.NewPhoneRecognizer(),
		recognizer.NewInnRecognizer(),
		recognizer.NewCardNumberRecognizer(),
		recognizer.NewCvvRecognizer(),
		recognizer.NewPinRecognizer(),
		recognizer.NewCardholderRecognizer(),
		recognizer.NewPassportRecognizer(),
		recognizer.NewDepartmentCodeRecognizer(),
		recognizer.NewDriverLicenseRecognizer(),
		recognizer.NewPassportIssuerRecognizer(),
		recognizer.NewDateRecognizer(),
		recognizer.NewCitizenshipRecognizer(nil),
		recognizer.NewBirthPlaceRecognizer(),
		recognizer.NewFullNameRecognizer(nil, nil, nil),
		recognizer.NewAddressRecognizer(nil),
	)
	return &Engine{
		registry: reg,
		scorer:   context.NewScorer(),
		resolver: resolver.New(opts.MinScore),
	}
}

// NewWithRegistry builds an engine with a custom recognizer registry.
func NewWithRegistry(reg *recognizer.Registry, opts Options) *Engine {
	if opts.MinScore == 0 {
		opts.MinScore = 0.5
	}
	return &Engine{
		registry: reg,
		scorer:   context.NewScorer(),
		resolver: resolver.New(opts.MinScore),
	}
}

// Analyze runs the full pipeline and returns detected entities.
func (e *Engine) Analyze(text string) []entity.Entity {
	norm := normalize.New(text)
	spans := e.registry.Recognize(norm)

	// Context scoring.
	scored := make([]entity.CandidateSpan, 0, len(spans))
	for _, s := range spans {
		adjusted, contextual := e.scorer.Score(norm, s)
		adjusted.Contextual = contextual
		scored = append(scored, adjusted)
	}

	return e.resolver.Resolve(scored)
}

// Registry returns the underlying recognizer registry.
func (e *Engine) Registry() *recognizer.Registry {
	return e.registry
}
