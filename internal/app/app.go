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
func New(cfg config.Config, log *observability.Logger, p contract.Processor) http.Handler {
	handler := api.NewHandler(p)

	mux := http.NewServeMux()
	handler.Routes(mux)

	authenticator := auth.New(cfg)

	var h http.Handler = mux
	h = middleware.BodyLimit(cfg.BodyLimit, h)
	h = middleware.ProcessingTimeout(cfg.ProcessingTimeout, h)
	h = authenticator.Middleware(h)
	h = middleware.Logging(log, h)
	h = middleware.Recover(log, h)
	return h
}