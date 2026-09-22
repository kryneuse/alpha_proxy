// Package auth authenticates callers for the HTTP contour. In the protected
// mode the consumer is identified only by a verified API key from configuration;
// in the explicitly enabled verification mode a fixed internal ConsumerID is
// used and no API key is required.
//
// Raw API keys are never stored or compared directly. Configured keys are
// reduced to SHA-256 fingerprints at construction time, and incoming keys are
// hashed before matching. Fingerprints are never logged.
package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"alpha_proxy/internal/config"
)

type ctxKey struct{}

// VerifyConsumerID is the fixed internal consumer used in verification mode.
const VerifyConsumerID = "verify"

// Authenticator resolves a request to a ConsumerID.
type Authenticator struct {
	mode    config.AuthMode
	byFp    map[string]string // key fingerprint -> systemID
	enabled map[string]bool   // systemID -> enabled
}

// New builds an Authenticator from configuration. Configured API keys are
// reduced to SHA-256 fingerprints; the raw keys are not retained.
func New(cfg config.Config) *Authenticator {
	a := &Authenticator{
		mode:    cfg.AuthMode,
		byFp:    make(map[string]string),
		enabled: make(map[string]bool),
	}
	for _, s := range cfg.Systems {
		a.byFp[fingerprint(s.APIKey)] = s.ID
		a.enabled[s.ID] = s.Enabled
	}
	return a
}

// ConsumerID returns the authenticated consumer ID from the context, or "".
func ConsumerID(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKey{}).(string); ok {
		return v
	}
	return ""
}

// Middleware authenticates the caller and stores the ConsumerID in the context.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		consumer, status := a.authenticate(r)
		if status != 0 {
			http.Error(w, http.StatusText(status), status)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKey{}, consumer)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// authenticate returns the ConsumerID and an HTTP status, or status 0 on success.
func (a *Authenticator) authenticate(r *http.Request) (string, int) {
	if a.mode == config.AuthModeVerify {
		return VerifyConsumerID, 0
	}

	key := r.Header.Get("X-API-Key")
	if key == "" {
		return "", http.StatusUnauthorized
	}
	systemID, ok := a.byFp[fingerprint(key)]
	if !ok {
		return "", http.StatusUnauthorized
	}
	if !a.enabled[systemID] {
		return "", http.StatusForbidden
	}
	return systemID, 0
}

// fingerprint returns the SHA-256 fingerprint of a key. The raw key is never
// retained or logged.
func fingerprint(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}