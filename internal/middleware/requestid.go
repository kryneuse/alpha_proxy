package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/kryneuse/alpha_proxy/internal/requestmeta"
)

type requestIDKey struct{}

// RequestID returns the request ID from the context, or "" if absent.
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey{}).(string); ok {
		return v
	}
	return ""
}

// RequestIDMiddleware wraps h, ensuring every request carries a request ID. A
// valid incoming X-Request-ID is preserved; an empty or invalid one is replaced
// with a freshly generated 128-bit hex ID. The final ID is written to the
// response header, the context and the request-scoped logging metadata. If the
// entropy source fails, a safe 500 is returned and the next handler is not
// called.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return requestIDMiddlewareWithReader(rand.Reader, next)
}

// requestIDMiddlewareWithReader is the internal variant with an injectable
// entropy reader, used by tests.
func requestIDMiddlewareWithReader(reader io.Reader, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !validRequestID(id) {
			var err error
			id, err = newRequestID(reader)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		if meta := requestmeta.From(r.Context()); meta != nil {
			meta.RequestID = id
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// validRequestID reports whether id is 1-64 bytes of ASCII letters, digits,
// ".", "_" or "-".
func validRequestID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '.' || c == '_' || c == '-':
		default:
			return false
		}
	}
	return true
}

// newRequestID generates a 128-bit random ID encoded as hex. It returns an error
// if the entropy source cannot be read; no fixed or zero ID is ever returned.
func newRequestID(reader io.Reader) (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(reader, b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
