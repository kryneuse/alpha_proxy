// Package app wires the HTTP contour together: config, middleware, auth, rate
// limiting, bounded concurrency, API handlers and the Processor.
package app

import (
	"net/http"

	"alpha_proxy/internal/api"
	"alpha_proxy/internal/auth"
	"alpha_proxy/internal/config"
	"alpha_proxy/internal/contract"
	"alpha_proxy/internal/middleware"
	"alpha_proxy/internal/observability"
	"alpha_proxy/internal/ratelimit"
)

// New builds the fully-wired http.Handler for the contour.
//
// Execution order: CompletionLogger → Recover → RequestID → ServeMux → Route
// metadata → global rate limit → Auth → per-consumer rate limit → bounded
// concurrency → ProcessingTimeout → BodyLimit → handler.
func New(cfg config.Config, log *observability.Logger, p contract.Processor) http.Handler {
	handler := api.NewHandler(p)
	authenticator := auth.New(cfg)

	var process http.Handler = http.HandlerFunc(handler.Process)
	process = middleware.BodyLimit(cfg.BodyLimit, process)
	process = middleware.ProcessingTimeout(cfg.ProcessingTimeout, process)
	process = middleware.ParallelLimit(cfg.ParallelLimit, cfg.OverloadRetryAfter, process)
	process = middleware.ConsumerRateLimit(buildConsumerLimiter(cfg), process)
	process = authenticator.Middleware(process)
	process = middleware.GlobalRateLimit(buildGlobalLimiter(cfg), process)
	process = middleware.RouteMetadata(api.ProcessRoute, process)

	mux := http.NewServeMux()
	mux.Handle(api.ProcessRoute, process)

	var h http.Handler = mux
	h = middleware.RequestIDMiddleware(h)
	h = middleware.Recover(h)
	h = middleware.CompletionLogger(log, h)
	return h
}

// buildGlobalLimiter returns the global token bucket, or nil if disabled.
func buildGlobalLimiter(cfg config.Config) *ratelimit.TokenBucket {
	if cfg.GlobalRateLimitRPS <= 0 {
		return nil
	}
	return ratelimit.NewTokenBucket(cfg.GlobalRateLimitRPS, cfg.GlobalRateLimitBurst, ratelimit.RealClock{})
}

// buildConsumerLimiter returns the per-consumer limiter, or nil if disabled. The
// set of consumers is limited to enabled systems and the fixed verify consumer.
func buildConsumerLimiter(cfg config.Config) *ratelimit.ConsumerLimiter {
	if cfg.ConsumerRateLimitRPS <= 0 {
		return nil
	}
	var consumers []string
	for _, s := range cfg.Systems {
		if s.Enabled {
			consumers = append(consumers, s.ID)
		}
	}
	if cfg.AuthMode == config.AuthModeVerify {
		consumers = append(consumers, auth.VerifyConsumerID)
	}
	return ratelimit.NewConsumerLimiter(cfg.ConsumerRateLimitRPS, cfg.ConsumerRateLimitBurst, consumers, ratelimit.RealClock{})
}
