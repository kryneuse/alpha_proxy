package pii

import "context"

type Policy struct {
	AllowedKinds          map[PIIKind]bool
	DetokenizationAllowed bool
	MinConfidence         float64
	MaskingStrategy       string
	AllowPartialResult    bool
}

type PolicyProvider interface {
	Get(ctx context.Context, consumerID string) (Policy, error)
}
