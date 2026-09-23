// Package contract defines the neutral boundary between the HTTP layer and the
// processing layer. It intentionally carries no masking, tokenization, storage
// or state-machine logic so that both sides can depend on it without coupling.
package contract

import (
	"context"
	"errors"
)

// ErrUnavailable reports that the Processor is temporarily unavailable. The
// HTTP layer maps it to 503. Processors may wrap it with %w.
var ErrUnavailable = errors.New("processor unavailable")

// ProcessRequest is the input handed from the HTTP layer to a Processor.
type ProcessRequest struct {
	// Payload is the raw text to be processed.
	Payload string
	// PayloadID identifies the payload so the Processor can keep per-ID state.
	PayloadID string
	// ConsumerID identifies the caller/consumer of the request.
	ConsumerID string
	// MaskKinds optionally overrides the set of PII kinds to mask. When
	// MaskKindsSet is true it narrows the consumer policy AllowedKinds to the
	// listed kinds (an empty list means mask nothing). When MaskKindsSet is
	// false the consumer policy is used unchanged.
	MaskKinds    []string
	MaskKindsSet bool
}

// ProcessResponse is the output produced by a Processor.
type ProcessResponse struct {
	// Result is the processed text returned to the caller.
	Result string
}

// Processor is the contract implemented by the processing layer. The HTTP layer
// only forwards ProcessRequest and returns ProcessResponse; it never decides the
// processing phase based on a call counter.
type Processor interface {
	Process(ctx context.Context, req ProcessRequest) (ProcessResponse, error)
}
