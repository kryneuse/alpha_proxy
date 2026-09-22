// Package config loads and validates server configuration for the HTTP contour.
// Secrets (API keys) are never stored in source and never logged; they are read
// from a JSON file referenced by an environment variable.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
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

	// Rate limit settings are reserved for the separate rate-limiting task and
	// are not applied to the working path yet.
	RateLimitPerMin int
	RateLimitWindow time.Duration
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
	if v, err := durEnv("ALPHA_PROXY_RATE_LIMIT_WINDOW", time.Minute); err != nil {
		fail(err)
	} else {
		cfg.RateLimitWindow = v
	}
	if v, err := int64Env("ALPHA_PROXY_BODY_LIMIT", 1<<20); err != nil {
		fail(err)
	} else {
		cfg.BodyLimit = v
	}
	if v, err := intEnv("ALPHA_PROXY_PARALLEL_LIMIT", 16); err != nil {
		fail(err)
	} else {
		cfg.ParallelLimit = v
	}
	if v, err := intEnv("ALPHA_PROXY_RATE_LIMIT_PER_MIN", 0); err != nil {
		fail(err)
	} else {
		cfg.RateLimitPerMin = v
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
	if c.RateLimitPerMin < 0 {
		return fmt.Errorf("config: rate limit per minute must not be negative")
	}
	if c.RateLimitPerMin > 0 && c.RateLimitWindow <= 0 {
		return fmt.Errorf("config: rate limit window must be positive when rate limit is set")
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

// IsEnvParseError reports whether err is an environment parse error.
func IsEnvParseError(err error) bool {
	var e *envParseError
	return errors.As(err, &e)
}