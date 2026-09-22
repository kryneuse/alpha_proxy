// Package requestmeta carries request-scoped metadata used only for logging. It
// is separate from authentication state: it cannot override auth.ConsumerID,
// which is what the Processor receives.
package requestmeta

import "context"

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
