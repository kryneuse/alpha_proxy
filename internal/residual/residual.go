// Package residual builds a residual text from an original string and a set
// of rule-engine spans. Found spans are replaced with spaces while preserving
// the exact byte length of the original string, so byte offsets remain
// compatible with the original chunk.
package residual

import (
	"strings"

	"github.com/kryneuse/alpha_proxy/internal/entity"
)

// Build returns a residual text where the given spans are replaced with
// spaces. The residual has the same byte length as the original string.
//
// Spans are expected to be byte offsets into the original string. Overlapping
// or out-of-range spans are handled safely.
func Build(original string, spans []entity.Entity) string {
	if len(spans) == 0 {
		return original
	}
	// Convert to bytes so we can replace byte ranges with spaces.
	b := []byte(original)
	for _, s := range spans {
		start := s.Start
		end := s.End
		if start < 0 {
			start = 0
		}
		if end > len(b) {
			end = len(b)
		}
		if start >= end {
			continue
		}
		for i := start; i < end; i++ {
			b[i] = ' '
		}
	}
	return string(b)
}

// BuildFromCandidates is like Build but accepts candidate spans.
func BuildFromCandidates(original string, spans []entity.CandidateSpan) string {
	entities := make([]entity.Entity, 0, len(spans))
	for _, s := range spans {
		entities = append(entities, entity.Entity{
			Type:  s.Type,
			Text:  s.Text,
			Start: s.Start,
			End:   s.End,
		})
	}
	return Build(original, entities)
}

// Masked reports whether the residual differs from the original (i.e. at
// least one span was masked).
func Masked(original, residual string) bool {
	return original != residual
}

// CountMaskedBytes returns the number of bytes replaced by spaces.
func CountMaskedBytes(original, residual string) int {
	if len(original) != len(residual) {
		return 0
	}
	count := 0
	for i := 0; i < len(original); i++ {
		if original[i] != residual[i] {
			count++
		}
	}
	return count
}

// TrimSpace returns a copy of the residual with leading/trailing whitespace
// removed. This is useful for display but note that byte offsets into the
// trimmed string differ from the original.
func TrimSpace(s string) string {
	return strings.TrimSpace(s)
}
