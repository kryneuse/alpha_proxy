// Package engine wires the full pipeline:
// text -> normalization -> recognizers -> context scoring -> resolver -> []Entity.
package engine

import (
	"unicode/utf8"

	"github.com/kryneuse/alpha_proxy/internal/context"
	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/normalize"
	"github.com/kryneuse/alpha_proxy/internal/recognizer"
	"github.com/kryneuse/alpha_proxy/internal/resolver"
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

	// Second pass: a nearby CARD_NUMBER is a strong signal for CVV/PIN, but
	// only when the code already has its own positive context.
	scored = boostNearCard(text, scored)

	return e.resolver.Resolve(scored)
}

// boostNearCard raises the score of CVV/PIN candidates that are close to a
// detected CARD_NUMBER. A real card number nearby is strong evidence that a
// 3/4-digit number is a CVV/PIN, but only after the candidate already has its
// own positive context. Distances are measured in Unicode characters, not
// byte offsets.
func boostNearCard(text string, spans []entity.CandidateSpan) []entity.CandidateSpan {
	// Collect card number spans.
	var cards []entity.CandidateSpan
	for _, s := range spans {
		if s.Type == entity.CARD_NUMBER && s.Score >= 0.5 {
			cards = append(cards, s)
		}
	}
	if len(cards) == 0 {
		return spans
	}
	out := make([]entity.CandidateSpan, len(spans))
	copy(out, spans)
	for i, s := range out {
		if s.Type != entity.CVV && s.Type != entity.PIN {
			continue
		}
		// Only boost a code that already has its own positive context.
		if !s.Contextual {
			continue
		}
		for _, c := range cards {
			if spansCloseRunes(text, s, c, 60) {
				// Strong boost: a card number nearby.
				out[i].Score += 0.4
				if out[i].Score > 1 {
					out[i].Score = 1
				}
				out[i].Sources = append(out[i].Sources, entity.SourceContext)
				break
			}
		}
	}
	return out
}

// spansCloseRunes reports whether two spans are within maxGap Unicode
// characters of each other.
func spansCloseRunes(text string, a, b entity.CandidateSpan, maxGap int) bool {
	if a.End < b.Start {
		return utf8.RuneCountInString(text[a.End:b.Start]) <= maxGap
	}
	if b.End < a.Start {
		return utf8.RuneCountInString(text[b.End:a.Start]) <= maxGap
	}
	return true
}

// Registry returns the underlying recognizer registry.
func (e *Engine) Registry() *recognizer.Registry {
	return e.registry
}
