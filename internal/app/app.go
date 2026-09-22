// Package app wires the HTTP contour together: config, middleware, auth, API
// handlers and the Processor. Rate limiting is intentionally not applied here;
// it is a separate task.
package app

import (
	"net/http"

	"alpha_proxy/internal/api"
	"alpha_proxy/internal/auth"
	"alpha_proxy/internal/config"
	"alpha_proxy/internal/contract"
	"alpha_proxy/internal/middleware"
	"alpha_proxy/internal/observability"
)

// New builds the fully-wired http.Handler for the contour.
//
// Execution order: CompletionLogger → Recover → RequestID → ServeMux → Route
// metadata → Auth → ProcessingTimeout → BodyLimit → handler.
func New(cfg config.Config, log *observability.Logger, p contract.Processor) http.Handler {
	handler := api.NewHandler(p)
	authenticator := auth.New(cfg)

	processHandler := middleware.RouteMetadata(api.ProcessRoute,
		authenticator.Middleware(
			middleware.ProcessingTimeout(cfg.ProcessingTimeout,
				middleware.BodyLimit(cfg.BodyLimit, http.HandlerFunc(handler.Process)))))

	mux := http.NewServeMux()
	mux.Handle(api.ProcessRoute, processHandler)

	var h http.Handler = mux
	h = middleware.RequestIDMiddleware(h)
	h = middleware.Recover(h)
	h = middleware.CompletionLogger(log, h)
	return h
}
