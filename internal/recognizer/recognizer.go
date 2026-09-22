// Package recognizer defines the recognizer interface and the registry that
// drives the deterministic recognition layer.
package recognizer

import (
	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/normalize"
)

// Recognizer is the single interface every deterministic recognizer implements.
// It returns candidate spans with offsets relative to the ORIGINAL text.
type Recognizer interface {
	// Type returns the entity type this recognizer produces.
	Type() entity.Type
	// Recognize scans the normalized text and returns candidate spans.
	// The normalized text carries an offset map back to the original text.
	Recognize(norm *normalize.Text) []entity.CandidateSpan
}

// Registry holds all recognizers and dispatches recognition.
type Registry struct {
	recognizers []Recognizer
}

// NewRegistry builds a registry from the given recognizers.
func NewRegistry(recognizers ...Recognizer) *Registry {
	return &Registry{recognizers: recognizers}
}

// Add appends a recognizer to the registry.
func (r *Registry) Add(rec Recognizer) {
	r.recognizers = append(r.recognizers, rec)
}

// Recognizers returns the registered recognizers.
func (r *Registry) Recognizers() []Recognizer {
	return r.recognizers
}

// Recognize runs every recognizer and returns all candidate spans.
func (r *Registry) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan
	for _, rec := range r.recognizers {
		spans = append(spans, rec.Recognize(norm)...)
	}
	return spans
}
