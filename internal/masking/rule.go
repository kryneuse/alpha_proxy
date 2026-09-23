package masking

import (
	"context"
	"errors"

	"github.com/kryneuse/alpha_proxy/internal/pii"
	"github.com/kryneuse/alpha_proxy/internal/tokenizer"
)

// RuleMasker маскирует персональные данные через rule engine detector.
type RuleMasker struct {
	detector pii.BackendDetector
}

// NewRuleMasker создаёт RuleMasker.
func NewRuleMasker(detector pii.BackendDetector) (*RuleMasker, error) {
	if detector == nil {
		return nil, errors.New("detector must not be nil")
	}
	return &RuleMasker{detector: detector}, nil
}

// Mask находит сущности через detector, строит replacement plan и применяет
// его к тексту.
func (m *RuleMasker) Mask(ctx context.Context, text string, policy pii.Policy) (string, []pii.TokenMapping, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}

	entities, err := m.detector.Detect(ctx, text)
	if err != nil {
		return "", nil, err
	}

	plan, err := tokenizer.BuildReplacementPlan(ctx, text, entities, policy)
	if err != nil {
		return "", nil, err
	}

	masked, mappings, err := tokenizer.ApplyReplacements(ctx, text, plan)
	if err != nil {
		return "", nil, err
	}

	return masked, mappings, nil
}
