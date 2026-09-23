package tokenizer

import (
	"context"
	"fmt"
	"sort"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

func CombinePlans(ctx context.Context, text string, mlPlan, backendPlan []pii.Replacement, policy pii.Policy) ([]pii.Replacement, error) {
	combined := make([]pii.Replacement, 0, len(mlPlan)+len(backendPlan))
	combined = append(combined, mlPlan...)
	combined = append(combined, backendPlan...)

	validated := make([]pii.Replacement, 0, len(combined))
	for _, r := range combined {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if err := validateSpan(text, r.Start, r.End); err != nil {
			return nil, err
		}
		if text[r.Start:r.End] != r.Original {
			return nil, fmt.Errorf("original mismatch at [%d:%d]: %w", r.Start, r.End, pii.ErrInvalidSpan)
		}

		if !policy.AllowedKinds[r.Kind] {
			continue
		}
		if r.Confidence < policy.MinConfidence {
			continue
		}

		validated = append(validated, r)
	}

	merged := dedupe(validated)
	selected, err := resolveOverlaps(ctx, text, merged)
	if err != nil {
		return nil, err
	}

	sort.Slice(selected, func(i, j int) bool {
		if selected[i].Start != selected[j].Start {
			return selected[i].Start < selected[j].Start
		}
		if selected[i].End != selected[j].End {
			return selected[i].End > selected[j].End
		}
		return selected[i].Kind < selected[j].Kind
	})

	return normalizeTokens(text, selected), nil
}

func normalizeTokens(text string, replacements []pii.Replacement) []pii.Replacement {
	allocator := NewAllocator(text)
	normalized := make([]pii.Replacement, len(replacements))
	for i, r := range replacements {
		normalized[i] = r
		normalized[i].Token = allocator.Allocate(r.Kind, r.Original)
	}
	return normalized
}

func dedupe(replacements []pii.Replacement) []pii.Replacement {
	byKey := make(map[string]pii.Replacement)
	order := make([]string, 0, len(replacements))

	for _, r := range replacements {
		key := fmt.Sprintf("%d:%d:%s:%s", r.Start, r.End, r.Kind, r.Original)
		existing, ok := byKey[key]
		if !ok {
			byKey[key] = r
			order = append(order, key)
			continue
		}

		winner := pickWinner(existing, r)
		byKey[key] = winner
	}

	result := make([]pii.Replacement, 0, len(order))
	for _, key := range order {
		result = append(result, byKey[key])
	}
	return result
}

func pickWinner(a, b pii.Replacement) pii.Replacement {
	primaryA := primarySource(a.Kind)
	primaryB := primarySource(b.Kind)

	if a.Source == primaryA && b.Source != primaryB {
		return a
	}
	if b.Source == primaryB && a.Source != primaryA {
		return b
	}

	if a.Confidence != b.Confidence {
		if a.Confidence > b.Confidence {
			return a
		}
		return b
	}

	if a.Source != b.Source {
		if a.Source < b.Source {
			return a
		}
		return b
	}

	if a.Token < b.Token {
		return a
	}
	return b
}

// resolveOverlaps выбирает winner по приоритету, но не теряет непокрытые
// части проигравших spans. Например, spans [0,10) и [5,15) закрывают весь
// [0,15), а не оставляют хвост [10,15). Покрытие итогового плана равно
// объединению всех принятых интервалов.
func resolveOverlaps(ctx context.Context, text string, replacements []pii.Replacement) ([]pii.Replacement, error) {
	candidates := make([]pii.Replacement, len(replacements))
	copy(candidates, replacements)

	sort.Slice(candidates, func(i, j int) bool {
		return candidateLess(candidates[i], candidates[j])
	})

	var selected []pii.Replacement
	for _, c := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for _, u := range uncoveredSpans(c, selected) {
			r := c
			r.Start = u[0]
			r.End = u[1]
			r.Original = text[u[0]:u[1]]
			selected = append(selected, r)
		}
	}
	return selected, nil
}

// uncoveredSpans возвращает интервалы внутри [c.Start, c.End), не покрытые ни
// одним span из selected.
func uncoveredSpans(c pii.Replacement, selected []pii.Replacement) [][2]int {
	sorted := make([]pii.Replacement, len(selected))
	copy(sorted, selected)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Start != sorted[j].Start {
			return sorted[i].Start < sorted[j].Start
		}
		return sorted[i].End < sorted[j].End
	})

	var out [][2]int
	cursor := c.Start
	for _, s := range sorted {
		if s.End <= cursor {
			continue
		}
		if s.Start >= c.End {
			break
		}
		if s.Start > cursor {
			out = append(out, [2]int{cursor, min(s.Start, c.End)})
		}
		if s.End > cursor {
			cursor = max(cursor, s.End)
		}
		if cursor >= c.End {
			break
		}
	}
	if cursor < c.End {
		out = append(out, [2]int{cursor, c.End})
	}
	return out
}

func candidateLess(a, b pii.Replacement) bool {
	pa := typePriority(a.Kind)
	pb := typePriority(b.Kind)
	if pa != pb {
		return pa > pb
	}

	primaryA := primarySource(a.Kind)
	primaryB := primarySource(b.Kind)
	aPrimary := a.Source == primaryA
	bPrimary := b.Source == primaryB
	if aPrimary != bPrimary {
		return aPrimary
	}

	if a.Confidence != b.Confidence {
		return a.Confidence > b.Confidence
	}

	aLen := a.End - a.Start
	bLen := b.End - b.Start
	if aLen != bLen {
		return aLen > bLen
	}

	if a.Start != b.Start {
		return a.Start < b.Start
	}
	if a.End != b.End {
		return a.End < b.End
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	return a.Token < b.Token
}

func spansOverlap(a, b pii.Replacement) bool {
	return a.Start < b.End && b.Start < a.End
}

func typePriority(kind pii.PIIKind) int {
	switch kind {
	case pii.PIIKindPhone, pii.PIIKindEmail, pii.PIIKindINN, pii.PIIKindBankCard,
		pii.PIIKindPassport, pii.PIIKindPassportDivision, pii.PIIKindDate,
		pii.PIIKindDriverLicense, pii.PIIKindCVV, pii.PIIKindPIN, pii.PIIKindPostalCode,
		pii.PIIKindFullName, pii.PIIKindAddress:
		return 2
	case pii.PIIKindFirstName, pii.PIIKindLastName, pii.PIIKindMiddleName,
		pii.PIIKindCity, pii.PIIKindStreet, pii.PIIKindHouse, pii.PIIKindApartment,
		pii.PIIKindBirthPlace, pii.PIIKindCitizenship, pii.PIIKindPassportIssuer,
		pii.PIIKindCardHolderName:
		return 1
	default:
		return 0
	}
}

func primarySource(kind pii.PIIKind) pii.Source {
	switch kind {
	case pii.PIIKindPhone, pii.PIIKindEmail, pii.PIIKindINN, pii.PIIKindBankCard,
		pii.PIIKindPassport, pii.PIIKindPassportDivision, pii.PIIKindDate,
		pii.PIIKindDriverLicense, pii.PIIKindCVV, pii.PIIKindPIN, pii.PIIKindPostalCode:
		return pii.SourceReg
	default:
		return pii.SourceML
	}
}
