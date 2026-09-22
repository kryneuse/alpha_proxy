// Package app wires the HTTP contour together: config, middleware, auth, rate
// limiting, bounded concurrency, API handlers, metrics, health and the
// Processor.
package app

import (
	"fmt"
	"net/http"

	"alpha_proxy/internal/api"
	"alpha_proxy/internal/auth"
	"alpha_proxy/internal/config"
	"alpha_proxy/internal/contract"
	"alpha_proxy/internal/health"
	"alpha_proxy/internal/middleware"
	"alpha_proxy/internal/observability"
	"alpha_proxy/internal/ratelimit"
)

// Runtime is the fully-wired HTTP contour together with its readiness state.
type Runtime struct {
	Handler   http.Handler
	Readiness *health.Readiness
}

// NewRuntime builds the fully-wired http.Handler for the contour.
//
// Execution order (outside-in): CompletionLogger → Metrics → Recover → RequestID
// → ServeMux. The process route keeps its internal order: Route metadata →
// global rate limit → Auth → per-consumer rate limit → bounded concurrency →
// ProcessingTimeout → BodyLimit → handler. Management routes (health, readiness,
// metrics) are registered method-aware and bypass auth, rate limiting, the
// concurrency semaphore, the processing timeout and the body limit.
func NewRuntime(cfg config.Config, log *observability.Logger, processor contract.Processor) (*Runtime, error) {
	if log == nil {
		return nil, fmt.Errorf("app: logger must not be nil")
	}
	if processor == nil {
		return nil, fmt.Errorf("app: processor must not be nil")
	}

	readiness := health.NewReadiness()

	var metrics *observability.Metrics
	if cfg.MetricsEnabled {
		var err error
		metrics, err = observability.NewMetrics()
		if err != nil {
			return nil, fmt.Errorf("app: create metrics: %w", err)
		}
		processor, err = observability.NewInstrumentedProcessor(processor, metrics)
		if err != nil {
			return nil, fmt.Errorf("app: instrument processor: %w", err)
		}
	}

	handler := api.NewHandler(processor)
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
	mux.Handle("GET /healthz", middleware.RouteMetadata("GET /healthz", health.LivenessHandler()))
	mux.Handle("GET /readyz", middleware.RouteMetadata("GET /readyz", readiness.Handler()))
	if metrics != nil {
		mux.Handle("GET /metrics", middleware.RouteMetadata("GET /metrics", metrics.Handler()))
	}

	var h http.Handler = mux
	h = middleware.RequestIDMiddleware(h)
	h = middleware.Recover(h)
	h = middleware.Metrics(metrics, h)
	h = middleware.CompletionLogger(log, h)

	readiness.SetReady(true)

	return &Runtime{Handler: h, Readiness: readiness}, nil
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
