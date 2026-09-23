package pii

import "context"

// MaskCondition is a conditional-masking rule for a PII kind. A target entity
// of that kind is masked only when the condition is satisfied by the set of
// confirmed PII kinds present in the whole payload.
//
//   - RequiresAll: every listed kind must be present (confirmed).
//   - RequiresAny: at least one listed kind must be present.
//
// An empty condition (no RequiresAll and no RequiresAny) is treated as
// "always satisfied" (legacy behavior: the kind can be masked on its own).
type MaskCondition struct {
	RequiresAny []PIIKind
	RequiresAll []PIIKind
}

// Satisfied reports whether the condition holds given the confirmed kinds.
func (c MaskCondition) Satisfied(confirmed map[PIIKind]bool) bool {
	for _, k := range c.RequiresAll {
		if !confirmed[k] {
			return false
		}
	}
	if len(c.RequiresAny) == 0 {
		return true
	}
	for _, k := range c.RequiresAny {
		if confirmed[k] {
			return true
		}
	}
	return false
}

type Policy struct {
	AllowedKinds          map[PIIKind]bool
	DetokenizationAllowed bool
	MinConfidence         float64
	MaskingStrategy       string
	AllowPartialResult    bool
	// MaskConditions maps a target PII kind to the condition that must be
	// satisfied (by confirmed kinds across the whole payload) for entities of
	// that kind to be masked. A kind absent from this map keeps the legacy
	// behavior: it can be masked on its own.
	MaskConditions map[PIIKind]MaskCondition
}

// ConfirmedKinds returns the set of PII kinds that are present in entities and
// pass AllowedKinds + MinConfidence. This is the payload-level set of
// unambiguously identified PII kinds used to evaluate MaskConditions.
func (p Policy) ConfirmedKinds(entities []Entity) map[PIIKind]bool {
	confirmed := make(map[PIIKind]bool)
	for _, e := range entities {
		if !p.AllowedKinds[e.Kind] {
			continue
		}
		if e.Confidence < p.MinConfidence {
			continue
		}
		confirmed[e.Kind] = true
	}
	return confirmed
}

// MaskAllowed reports whether an entity of the given kind may be masked given
// the confirmed kinds present in the payload. A kind with no MaskCondition is
// always allowed (legacy behavior).
func (p Policy) MaskAllowed(kind PIIKind, confirmed map[PIIKind]bool) bool {
	cond, ok := p.MaskConditions[kind]
	if !ok {
		return true
	}
	return cond.Satisfied(confirmed)
}

type PolicyProvider interface {
	Get(ctx context.Context, consumerID string) (Policy, error)
}
