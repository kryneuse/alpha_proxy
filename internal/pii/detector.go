package pii

import "context"

type Entity struct {
	Kind       PIIKind
	Start      int
	End        int
	Confidence float64
	Source     Source
	Metadata   map[string]string
}

type BackendDetector interface {
	Detect(ctx context.Context, text string, policy Policy) ([]Entity, error)
}
