// Package config loads and validates server configuration for the HTTP contour.
// Secrets (API keys) are never stored in source and never logged; they are read
// from a JSON file referenced by an environment variable.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"
)

// RunMode selects the operating mode of the service.
type RunMode string

const (
	// RunModeDev is the development mode. It is the only mode that permits the
	// temporary MockProcessor.
	RunModeDev RunMode = "dev"
	// RunModeVerify is the verification mode used by automated checks.
	RunModeVerify RunMode = "verify"
	// RunModeFinal is the final/production mode.
	RunModeFinal RunMode = "final"
)

// AuthMode selects how the caller is authenticated.
type AuthMode string

const (
	// AuthModeAPIKey is the protected mode: the consumer is identified only by a
	// verified API key from configuration.
	AuthModeAPIKey AuthMode = "api_key"
	// AuthModeVerify is the explicitly enabled verification mode without an API
	// key. It is never enabled by default and uses a fixed internal ConsumerID.
	AuthModeVerify AuthMode = "verify"
)

// ProcessorMode selects which Processor implementation is wired.
type ProcessorMode string

const (
	// ProcessorMock selects the temporary MockProcessor. Allowed only in dev mode.
	ProcessorMock ProcessorMode = "mock"
	// ProcessorReal selects a real Processor. Required for verify/final modes.
	ProcessorReal ProcessorMode = "real"
)

// System is a configured consumer system.
type System struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"api_key"`
}

// Config holds all settings needed to run the HTTP contour.
type Config struct {
	Addr              string
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	BodyLimit         int64
	ProcessingTimeout time.Duration
	ParallelLimit     int
	RunMode           RunMode
	AuthMode          AuthMode
	ProcessorMode     ProcessorMode
	Systems           []System
	// MLAddress is the gRPC target of the Python ML service.
	MLAddress string

	// GlobalRateLimitRPS is the global token bucket rate in requests per second.
	// Zero disables the global limiter.
	GlobalRateLimitRPS float64
	// GlobalRateLimitBurst is the global token bucket burst.
	GlobalRateLimitBurst int
	// ConsumerRateLimitRPS is the per-consumer token bucket rate. Zero disables
	// the per-consumer limiter.
	ConsumerRateLimitRPS float64
	// ConsumerRateLimitBurst is the per-consumer token bucket burst.
	ConsumerRateLimitBurst int
	// OverloadRetryAfter is the Retry-After used when the concurrency limit is
	// full.
	OverloadRetryAfter time.Duration
	// MetricsEnabled enables the Prometheus metrics endpoint.
	MetricsEnabled bool
	// ShutdownTimeout bounds how long the server waits for in-flight requests to
	// drain during a graceful shutdown.
	ShutdownTimeout time.Duration
}

// Load reads configuration from the environment and validates it.
func Load() (Config, error) {
	var firstErr error
	fail := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
	}

	cfg := Config{
		Addr:          envOr("ALPHA_PROXY_ADDR", ":8080"),
		RunMode:       RunMode(envOr("ALPHA_PROXY_RUN_MODE", string(RunModeFinal))),
		AuthMode:      AuthMode(envOr("ALPHA_PROXY_AUTH_MODE", string(AuthModeAPIKey))),
		ProcessorMode: ProcessorMode(envOr("ALPHA_PROXY_PROCESSOR_MODE", string(ProcessorReal))),
		MLAddress:     envOr("ALPHA_PROXY_ML_ADDR", "127.0.0.1:50051"),
	}

	if v, err := durEnv("ALPHA_PROXY_READ_TIMEOUT", 10*time.Second); err != nil {
		fail(err)
	} else {
		cfg.ReadTimeout = v
	}
	if v, err := durEnv("ALPHA_PROXY_READ_HEADER_TIMEOUT", 5*time.Second); err != nil {
		fail(err)
	} else {
		cfg.ReadHeaderTimeout = v
	}
	if v, err := durEnv("ALPHA_PROXY_WRITE_TIMEOUT", 10*time.Second); err != nil {
		fail(err)
	} else {
		cfg.WriteTimeout = v
	}
	if v, err := durEnv("ALPHA_PROXY_IDLE_TIMEOUT", 60*time.Second); err != nil {
		fail(err)
	} else {
		cfg.IdleTimeout = v
	}
	if v, err := durEnv("ALPHA_PROXY_PROCESSING_TIMEOUT", 5*time.Second); err != nil {
		fail(err)
	} else {
		cfg.ProcessingTimeout = v
	}
	if v, err := durEnv("ALPHA_PROXY_OVERLOAD_RETRY_AFTER", time.Second); err != nil {
		fail(err)
	} else {
		cfg.OverloadRetryAfter = v
	}
	if v, err := int64Env("ALPHA_PROXY_BODY_LIMIT", 1<<20); err != nil {
		fail(err)
	} else {
		cfg.BodyLimit = v
	}
	if v, err := intEnv("ALPHA_PROXY_PARALLEL_LIMIT", 512); err != nil {
		fail(err)
	} else {
		cfg.ParallelLimit = v
	}
	if v, err := intEnv("ALPHA_PROXY_GLOBAL_RATE_LIMIT_BURST", 2000); err != nil {
		fail(err)
	} else {
		cfg.GlobalRateLimitBurst = v
	}
	if v, err := intEnv("ALPHA_PROXY_CONSUMER_RATE_LIMIT_BURST", 0); err != nil {
		fail(err)
	} else {
		cfg.ConsumerRateLimitBurst = v
	}
	if v, err := floatEnv("ALPHA_PROXY_GLOBAL_RATE_LIMIT_RPS", 2000); err != nil {
		fail(err)
	} else {
		cfg.GlobalRateLimitRPS = v
	}
	if v, err := floatEnv("ALPHA_PROXY_CONSUMER_RATE_LIMIT_RPS", 0); err != nil {
		fail(err)
	} else {
		cfg.ConsumerRateLimitRPS = v
	}
	if v, err := boolEnv("ALPHA_PROXY_METRICS_ENABLED", true); err != nil {
		fail(err)
	} else {
		cfg.MetricsEnabled = v
	}
	if v, err := durEnv("ALPHA_PROXY_SHUTDOWN_TIMEOUT", 15*time.Second); err != nil {
		fail(err)
	} else {
		cfg.ShutdownTimeout = v
	}

	if file := os.Getenv("ALPHA_PROXY_SYSTEMS_FILE"); file != "" {
		systems, err := loadSystems(file)
		if err != nil {
			fail(err)
		} else {
			cfg.Systems = systems
		}
	}

	if firstErr != nil {
		return Config{}, firstErr
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks the configuration for consistency.
func (c Config) Validate() error {
	if c.Addr == "" {
		return fmt.Errorf("config: addr must not be empty")
	}
	if c.MLAddress == "" {
		return fmt.Errorf("config: ml address must not be empty")
	}
	if c.ReadTimeout <= 0 || c.ReadHeaderTimeout <= 0 || c.WriteTimeout <= 0 || c.IdleTimeout <= 0 {
		return fmt.Errorf("config: http timeouts must be strictly positive")
	}
	if c.BodyLimit <= 0 {
		return fmt.Errorf("config: body limit must be positive")
	}
	if c.ProcessingTimeout <= 0 {
		return fmt.Errorf("config: processing timeout must be positive")
	}
	if c.ParallelLimit <= 0 {
		return fmt.Errorf("config: parallel limit must be positive")
	}
	if c.OverloadRetryAfter <= 0 {
		return fmt.Errorf("config: overload retry after must be positive")
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("config: shutdown timeout must be positive")
	}
	if err := validateRateLimit(c.GlobalRateLimitRPS, c.GlobalRateLimitBurst, "global"); err != nil {
		return err
	}
	if err := validateRateLimit(c.ConsumerRateLimitRPS, c.ConsumerRateLimitBurst, "consumer"); err != nil {
		return err
	}

	if err := validateModes(c.RunMode, c.AuthMode, c.ProcessorMode); err != nil {
		return err
	}

	if c.AuthMode == AuthModeAPIKey {
		if err := validateSystems(c.Systems); err != nil {
			return err
		}
	}

	return nil
}

// validateRateLimit requires a finite, non-negative RPS and a non-negative
// burst. When RPS is set, the burst must be strictly positive.
func validateRateLimit(rps float64, burst int, name string) error {
	if math.IsNaN(rps) || math.IsInf(rps, 0) {
		return fmt.Errorf("config: %s rate limit rps must be finite", name)
	}
	if rps < 0 {
		return fmt.Errorf("config: %s rate limit rps must not be negative", name)
	}
	if burst < 0 {
		return fmt.Errorf("config: %s rate limit burst must not be negative", name)
	}
	if rps > 0 && burst <= 0 {
		return fmt.Errorf("config: %s rate limit burst must be positive when rps is set", name)
	}
	return nil
}

// validateModes enforces the safe combinations of run/auth/processor modes.
func validateModes(run RunMode, auth AuthMode, proc ProcessorMode) error {
	switch run {
	case RunModeDev, RunModeVerify, RunModeFinal:
	default:
		return fmt.Errorf("config: invalid run mode %q", run)
	}
	switch auth {
	case AuthModeAPIKey, AuthModeVerify:
	default:
		return fmt.Errorf("config: invalid auth mode %q", auth)
	}
	switch proc {
	case ProcessorMock, ProcessorReal:
	default:
		return fmt.Errorf("config: invalid processor mode %q", proc)
	}

	switch run {
	case RunModeFinal:
		if auth != AuthModeAPIKey {
			return fmt.Errorf("config: final mode requires auth_mode=api_key")
		}
		if proc != ProcessorReal {
			return fmt.Errorf("config: final mode requires processor_mode=real")
		}
	case RunModeVerify:
		if auth != AuthModeVerify {
			return fmt.Errorf("config: verify mode requires auth_mode=verify")
		}
		if proc != ProcessorReal {
			return fmt.Errorf("config: verify mode requires processor_mode=real")
		}
	case RunModeDev:
		// Dev mode permits the mock processor and either auth mode. The mock
		// processor is a stub and must never be used outside dev mode.
		if proc == ProcessorMock && auth != AuthModeVerify && auth != AuthModeAPIKey {
			return fmt.Errorf("config: invalid auth mode %q", auth)
		}
	}
	return nil
}

func validateSystems(systems []System) error {
	seenID := make(map[string]bool)
	seenKey := make(map[string]bool)
	enabled := 0
	for _, s := range systems {
		if s.ID == "" {
			return fmt.Errorf("config: system id must not be empty")
		}
		if seenID[s.ID] {
			return fmt.Errorf("config: duplicate system id %q", s.ID)
		}
		seenID[s.ID] = true
		if s.APIKey == "" {
			return fmt.Errorf("config: system %q has empty api key", s.ID)
		}
		if seenKey[s.APIKey] {
			return fmt.Errorf("config: duplicate api key across systems")
		}
		seenKey[s.APIKey] = true
		if s.Enabled {
			enabled++
		}
	}
	if enabled == 0 {
		return fmt.Errorf("config: at least one enabled system is required in api_key mode")
	}
	return nil
}

func loadSystems(path string) ([]System, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read systems file: %w", err)
	}
	var systems []System
	if err := json.Unmarshal(data, &systems); err != nil {
		return nil, fmt.Errorf("config: parse systems file: %w", err)
	}
	return systems, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envParseError reports an invalid environment value. Its message names the
// parameter but never includes the value; the underlying parse error is kept
// for errors.Is/As via Unwrap.
type envParseError struct {
	key string
	err error
}

func (e *envParseError) Error() string { return "config: invalid value for " + e.key }
func (e *envParseError) Unwrap() error { return e.err }

func durEnv(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, &envParseError{key: key, err: err}
	}
	return d, nil
}

func intEnv(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, &envParseError{key: key, err: err}
	}
	return n, nil
}

func int64Env(key string, fallback int64) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, &envParseError{key: key, err: err}
	}
	return n, nil
}

func floatEnv(key string, fallback float64) (float64, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, &envParseError{key: key, err: err}
	}
	return f, nil
}

func boolEnv(key string, fallback bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, &envParseError{key: key, err: err}
	}
	return b, nil
}

// IsEnvParseError reports whether err is an environment parse error.
func IsEnvParseError(err error) bool {
	var e *envParseError
	return errors.As(err, &e)
}
