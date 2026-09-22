package tokenizer

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

func ApplyReplacements(ctx context.Context, text string, replacements []pii.Replacement) (string, []pii.TokenMapping, error) {
	sorted := make([]pii.Replacement, len(replacements))
	copy(sorted, replacements)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Start < sorted[j].Start
	})

	if err := validateReplacements(ctx, text, sorted); err != nil {
		return "", nil, err
	}

	mappings, err := buildMappings(ctx, sorted)
	if err != nil {
		return "", nil, err
	}

	masked, err := buildMaskedText(ctx, text, sorted)
	if err != nil {
		return "", nil, err
	}

	return masked, mappings, nil
}

func validateReplacements(ctx context.Context, text string, replacements []pii.Replacement) error {
	for i, r := range replacements {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := validateSpan(text, r.Start, r.End); err != nil {
			return err
		}
		if text[r.Start:r.End] != r.Original {
			return fmt.Errorf("original mismatch at [%d:%d]: %w", r.Start, r.End, pii.ErrInvalidSpan)
		}
		if r.Token == "" {
			return fmt.Errorf("empty token at [%d:%d]: %w", r.Start, r.End, pii.ErrInvalidSpan)
		}
		if i > 0 && spansOverlap(replacements[i-1], r) {
			return fmt.Errorf("overlapping replacements at [%d:%d] and [%d:%d]: %w",
				replacements[i-1].Start, replacements[i-1].End, r.Start, r.End, pii.ErrInvalidSpan)
		}
	}
	return nil
}

func buildMappings(ctx context.Context, replacements []pii.Replacement) ([]pii.TokenMapping, error) {
	tokenOriginals := make(map[string]pii.TokenMapping)
	mappings := make([]pii.TokenMapping, 0, len(replacements))

	for _, r := range replacements {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if existing, ok := tokenOriginals[r.Token]; ok {
			if existing.Original != r.Original || existing.Kind != r.Kind {
				return nil, fmt.Errorf("token %q maps to conflicting originals: %w", r.Token, pii.ErrInvalidSpan)
			}
		} else {
			tokenOriginals[r.Token] = pii.TokenMapping{
				Token:    r.Token,
				Original: r.Original,
				Kind:     r.Kind,
				Source:   r.Source,
			}
		}

		mappings = append(mappings, pii.TokenMapping{
			Token:    r.Token,
			Original: r.Original,
			Kind:     r.Kind,
			Source:   r.Source,
			Start:    r.Start,
			End:      r.End,
		})
	}

	return mappings, nil
}

func buildMaskedText(ctx context.Context, text string, replacements []pii.Replacement) (string, error) {
	parts := make([]string, 0, len(replacements)*2+1)
	prev := len(text)

	for i := len(replacements) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		r := replacements[i]
		parts = append(parts, text[r.End:prev])
		parts = append(parts, r.Token)
		prev = r.Start
	}
	parts = append(parts, text[:prev])

	var sb strings.Builder
	for i := len(parts) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		sb.WriteString(parts[i])
	}

	return sb.String(), nil
}
