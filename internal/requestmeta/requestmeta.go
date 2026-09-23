// Package requestmeta carries request-scoped metadata used only for logging. It
// is separate from authentication state: it cannot override auth.ConsumerID,
// which is what the Processor receives.
package requestmeta

import (
	"context"
	"sync/atomic"
)

type ctxKey struct{}

// Meta holds request-scoped values that influence logging only.
type Meta struct {
	// RequestID is the final request ID used for the completion log.
	RequestID string
	// ConsumerID is set by the auth middleware after successful authentication.
	ConsumerID string
	// Route is the registered route pattern matched for the request.
	Route string
	// ErrorClass is a safe error classification, e.g. "panic".
	ErrorClass string

	// Operation is the masking operation performed, e.g. "mask" or "detokenize".
	// It is populated by the observability layer and never contains raw data.
	Operation string
	// PIICount is the number of PII entities found for the request.
	PIICount int
	// PIITypes is the set of PII type names found, without any original values.
	PIITypes []string
	// mlInvoked reports whether the ML service was called for the request. It is
	// set from parallel chunk goroutines, so it is an atomic flag.
	mlInvoked atomic.Bool
}

// SetMLInvoked marks that the ML service was invoked for this request.
func (m *Meta) SetMLInvoked() {
	m.mlInvoked.Store(true)
}

// MLInvoked reports whether the ML service was invoked for this request.
func (m *Meta) MLInvoked() bool {
	return m.mlInvoked.Load()
}

// With returns a context carrying m.
func With(ctx context.Context, m *Meta) context.Context {
	return context.WithValue(ctx, ctxKey{}, m)
}

// From returns the request-scoped Meta from ctx, or nil if absent.
func From(ctx context.Context) *Meta {
	m, _ := ctx.Value(ctxKey{}).(*Meta)
	return m
}
