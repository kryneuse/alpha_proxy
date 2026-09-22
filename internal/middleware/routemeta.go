package middleware

import (
	"net/http"

	"alpha_proxy/internal/requestmeta"
)

// RouteMetadata records a known route pattern in the request-scoped logging
// metadata. It runs before auth so that 401/403 responses still carry the route.
// It never reads r.URL.Path.
func RouteMetadata(route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if meta := requestmeta.From(r.Context()); meta != nil {
			meta.Route = route
		}
		next.ServeHTTP(w, r)
	})
}
