package tokenizer

import (
	"context"
	"fmt"
	"sort"
	"unicode/utf8"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

func BuildReplacementPlan(ctx context.Context, text string, entities []pii.Entity, policy pii.Policy) ([]pii.Replacement, error) {
	// Backward-compatible entry point: compute confirmed kinds from the given
	// entities. Callers that need payload-level conditions (entities split
	// across ml/backend or chunks) should use BuildReplacementPlanWithConfirmed.
	confirmed := policy.ConfirmedKinds(entities)
	return BuildReplacementPlanWithConfirmed(ctx, text, entities, policy, confirmed)
}

// BuildReplacementPlanWithConfirmed builds a replacement plan applying
// conditional masking. `confirmed` is the payload-level set of confirmed PII
// kinds used to evaluate MaskConditions; it must be computed from ALL entities
// of the payload (not just the ones passed here) so that supporting kinds from
// other chunks/sources satisfy the condition.
func BuildReplacementPlanWithConfirmed(ctx context.Context, text string, entities []pii.Entity, policy pii.Policy, confirmed map[pii.PIIKind]bool) ([]pii.Replacement, error) {
	sorted := make([]pii.Entity, len(entities))
	copy(sorted, entities)

	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Start != sorted[j].Start {
			return sorted[i].Start < sorted[j].Start
		}
		return sorted[i].End > sorted[j].End
	})

	allocator := NewAllocator(text)
	plan := make([]pii.Replacement, 0, len(sorted))

	for _, entity := range sorted {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if err := validateSpan(text, entity.Start, entity.End); err != nil {
			return nil, err
		}

		if !policy.AllowedKinds[entity.Kind] {
			continue
		}
		if entity.Confidence < policy.MinConfidence {
			continue
		}
		if !policy.MaskAllowed(entity.Kind, confirmed) {
			continue
		}

		original := text[entity.Start:entity.End]
		token := allocator.Allocate(entity.Kind, original)

		plan = append(plan, pii.Replacement{
			Start:      entity.Start,
			End:        entity.End,
			Token:      token,
			Original:   original,
			Kind:       entity.Kind,
			Source:     entity.Source,
			Confidence: entity.Confidence,
		})
	}

	return plan, nil
}

func validateSpan(text string, start, end int) error {
	if start < 0 || end > len(text) || start >= end {
		return fmt.Errorf("invalid span [%d:%d]: %w", start, end, pii.ErrInvalidSpan)
	}
	if !utf8.ValidString(text[start:end]) {
		return fmt.Errorf("span [%d:%d] cuts a UTF-8 character: %w", start, end, pii.ErrInvalidSpan)
	}
	return nil
}
