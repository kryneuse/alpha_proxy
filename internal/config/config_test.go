package config

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "systems.json")
	if err := os.WriteFile(path, []byte(`[{"id":"sys-a","enabled":true,"api_key":"secret-a"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALPHA_PROXY_SYSTEMS_FILE", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", cfg.Addr)
	}
	if cfg.RunMode != RunModeFinal {
		t.Errorf("RunMode = %q, want final", cfg.RunMode)
	}
	if cfg.AuthMode != AuthModeAPIKey {
		t.Errorf("AuthMode = %q, want api_key", cfg.AuthMode)
	}
	if cfg.ProcessorMode != ProcessorReal {
		t.Errorf("ProcessorMode = %q, want real", cfg.ProcessorMode)
	}
	if cfg.MLAddress != "127.0.0.1:50051" {
		t.Errorf("MLAddress = %q, want default 127.0.0.1:50051", cfg.MLAddress)
	}
	if cfg.ReadHeaderTimeout <= 0 {
		t.Errorf("ReadHeaderTimeout must be positive, got %v", cfg.ReadHeaderTimeout)
	}
	if cfg.BodyLimit <= 0 || cfg.ProcessingTimeout <= 0 || cfg.ParallelLimit <= 0 {
		t.Errorf("limits/timeouts must be positive: %+v", cfg)
	}
	if cfg.ParallelLimit != 512 {
		t.Errorf("ParallelLimit = %d, want default 512", cfg.ParallelLimit)
	}
	if !cfg.MetricsEnabled {
		t.Errorf("MetricsEnabled = false, want default true")
	}
}

func TestLoadDefaultsFailWithoutEnabledSystem(t *testing.T) {
	clearEnv(t)
	// Defaults use api_key mode but no systems file is set, so validation fails.
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error without any enabled system in api_key mode")
	}
}

func TestValidateModeCombinations(t *testing.T) {
	tests := []struct {
		name    string
		run     RunMode
		auth    AuthMode
		proc    ProcessorMode
		wantErr bool
	}{
		// final must use api_key + real
		{"final api_key real", RunModeFinal, AuthModeAPIKey, ProcessorReal, false},
		{"final verify real", RunModeFinal, AuthModeVerify, ProcessorReal, true},
		{"final api_key mock", RunModeFinal, AuthModeAPIKey, ProcessorMock, true},
		{"final verify mock", RunModeFinal, AuthModeVerify, ProcessorMock, true},
		// verify must use verify + real
		{"verify verify real", RunModeVerify, AuthModeVerify, ProcessorReal, false},
		{"verify api_key real", RunModeVerify, AuthModeAPIKey, ProcessorReal, true},
		{"verify verify mock", RunModeVerify, AuthModeVerify, ProcessorMock, true},
		{"verify api_key mock", RunModeVerify, AuthModeAPIKey, ProcessorMock, true},
		// dev permits mock and either auth mode
		{"dev api_key real", RunModeDev, AuthModeAPIKey, ProcessorReal, false},
		{"dev verify real", RunModeDev, AuthModeVerify, ProcessorReal, false},
		{"dev api_key mock", RunModeDev, AuthModeAPIKey, ProcessorMock, false},
		{"dev verify mock", RunModeDev, AuthModeVerify, ProcessorMock, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.RunMode = tt.run
			cfg.AuthMode = tt.auth
			cfg.ProcessorMode = tt.proc
			err := cfg.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("Validate() expected error for %s/%s/%s", tt.run, tt.auth, tt.proc)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() unexpected error for %s/%s/%s: %v", tt.run, tt.auth, tt.proc, err)
			}
		})
	}
}

func TestValidateDisabledSystem(t *testing.T) {
	base := validConfig()
	base.Systems = []System{{ID: "s1", Enabled: false, APIKey: "k1"}}
	if err := base.Validate(); err == nil {
		t.Fatal("expected error: no enabled system")
	}
}

func TestValidateDuplicateKeys(t *testing.T) {
	base := validConfig()
	base.Systems = []System{
		{ID: "s1", Enabled: true, APIKey: "k1"},
		{ID: "s2", Enabled: true, APIKey: "k1"},
	}
	if err := base.Validate(); err == nil {
		t.Fatal("expected error: duplicate api key")
	}
}

func TestValidateRateLimit(t *testing.T) {
	base := validConfig()
	base.GlobalRateLimitRPS = -1
	if err := base.Validate(); err == nil {
		t.Fatal("expected error: negative global rps")
	}

	base = validConfig()
	base.GlobalRateLimitRPS = math.NaN()
	if err := base.Validate(); err == nil {
		t.Fatal("expected error: NaN global rps")
	}

	base = validConfig()
	base.GlobalRateLimitRPS = math.Inf(1)
	if err := base.Validate(); err == nil {
		t.Fatal("expected error: Inf global rps")
	}

	base = validConfig()
	base.GlobalRateLimitRPS = 10
	base.GlobalRateLimitBurst = 0
	if err := base.Validate(); err == nil {
		t.Fatal("expected error: rps set but burst not positive")
	}

	base = validConfig()
	base.ConsumerRateLimitRPS = 5
	base.ConsumerRateLimitBurst = 0
	if err := base.Validate(); err == nil {
		t.Fatal("expected error: consumer rps set but burst not positive")
	}

	base = validConfig()
	base.GlobalRateLimitBurst = -1
	if err := base.Validate(); err == nil {
		t.Fatal("expected error: negative global burst")
	}

	base = validConfig()
	base.ConsumerRateLimitRPS = 0
	base.ConsumerRateLimitBurst = -1
	if err := base.Validate(); err == nil {
		t.Fatal("expected error: negative consumer burst at rps=0")
	}

	base = validConfig()
	base.GlobalRateLimitRPS = 2000
	base.GlobalRateLimitBurst = 2000
	if err := base.Validate(); err != nil {
		t.Fatalf("valid rate limit rejected: %v", err)
	}
}

func TestLoadInvalidFloatEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("ALPHA_PROXY_GLOBAL_RATE_LIMIT_RPS", "not-a-float")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid float")
	}
	if !IsEnvParseError(err) {
		t.Fatalf("expected env parse error, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "ALPHA_PROXY_GLOBAL_RATE_LIMIT_RPS") {
		t.Errorf("error must name the parameter, got: %v", err)
	}
	if strings.Contains(err.Error(), "not-a-float") {
		t.Errorf("error must not include the value, got: %v", err)
	}
}

func TestLoadMetricsEnabledTrue(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "systems.json")
	if err := os.WriteFile(path, []byte(`[{"id":"sys-a","enabled":true,"api_key":"secret-a"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALPHA_PROXY_SYSTEMS_FILE", path)
	t.Setenv("ALPHA_PROXY_METRICS_ENABLED", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !cfg.MetricsEnabled {
		t.Errorf("MetricsEnabled = false, want true")
	}
}

func TestLoadMetricsEnabledFalse(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "systems.json")
	if err := os.WriteFile(path, []byte(`[{"id":"sys-a","enabled":true,"api_key":"secret-a"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALPHA_PROXY_SYSTEMS_FILE", path)
	t.Setenv("ALPHA_PROXY_METRICS_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.MetricsEnabled {
		t.Errorf("MetricsEnabled = true, want false")
	}
}

func TestLoadMetricsEnabledInvalid(t *testing.T) {
	clearEnv(t)
	t.Setenv("ALPHA_PROXY_METRICS_ENABLED", "not-a-bool")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid bool")
	}
	if !IsEnvParseError(err) {
		t.Fatalf("expected env parse error, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "ALPHA_PROXY_METRICS_ENABLED") {
		t.Errorf("error must name the parameter, got: %v", err)
	}
	if strings.Contains(err.Error(), "not-a-bool") {
		t.Errorf("error must not include the value, got: %v", err)
	}
}

func TestLoadInvalidDurationEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("ALPHA_PROXY_READ_TIMEOUT", "not-a-duration")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid duration")
	}
	if !IsEnvParseError(err) {
		t.Fatalf("expected env parse error, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "ALPHA_PROXY_READ_TIMEOUT") {
		t.Errorf("error must name the parameter, got: %v", err)
	}
	if strings.Contains(err.Error(), "not-a-duration") {
		t.Errorf("error must not include the value, got: %v", err)
	}
}

func TestLoadInvalidBodyLimitEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("ALPHA_PROXY_BODY_LIMIT", "abc")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid body limit")
	}
	if !IsEnvParseError(err) {
		t.Fatalf("expected env parse error, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "ALPHA_PROXY_BODY_LIMIT") {
		t.Errorf("error must name the parameter, got: %v", err)
	}
	if strings.Contains(err.Error(), "abc") {
		t.Errorf("error must not include the value, got: %v", err)
	}
}

func TestLoadInvalidParallelLimitEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("ALPHA_PROXY_PARALLEL_LIMIT", "xyz")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid parallel limit")
	}
	if !IsEnvParseError(err) {
		t.Fatalf("expected env parse error, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "ALPHA_PROXY_PARALLEL_LIMIT") {
		t.Errorf("error must name the parameter, got: %v", err)
	}
}

func TestLoadReadHeaderTimeoutEnv(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "systems.json")
	if err := os.WriteFile(path, []byte(`[{"id":"sys-a","enabled":true,"api_key":"secret-a"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALPHA_PROXY_SYSTEMS_FILE", path)
	t.Setenv("ALPHA_PROXY_READ_HEADER_TIMEOUT", "3s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.ReadHeaderTimeout != 3*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v, want 3s", cfg.ReadHeaderTimeout)
	}
}

func TestValidateZeroTimeouts(t *testing.T) {
	base := validConfig()
	base.ReadTimeout = 0
	if err := base.Validate(); err == nil {
		t.Fatal("expected error: zero read timeout")
	}
	base = validConfig()
	base.ReadHeaderTimeout = 0
	if err := base.Validate(); err == nil {
		t.Fatal("expected error: zero read header timeout")
	}
	base = validConfig()
	base.WriteTimeout = 0
	if err := base.Validate(); err == nil {
		t.Fatal("expected error: zero write timeout")
	}
	base = validConfig()
	base.IdleTimeout = 0
	if err := base.Validate(); err == nil {
		t.Fatal("expected error: zero idle timeout")
	}
}

func TestLoadSystemsFromFile(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "systems.json")
	content := `[{"id":"sys-a","enabled":true,"api_key":"secret-a"}]`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALPHA_PROXY_SYSTEMS_FILE", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(cfg.Systems) != 1 || cfg.Systems[0].ID != "sys-a" {
		t.Fatalf("unexpected systems: %+v", cfg.Systems)
	}
}

func TestLoadShutdownTimeoutDefault(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "systems.json")
	if err := os.WriteFile(path, []byte(`[{"id":"sys-a","enabled":true,"api_key":"secret-a"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALPHA_PROXY_SYSTEMS_FILE", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.ShutdownTimeout != 15*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 15s", cfg.ShutdownTimeout)
	}
}

func TestLoadShutdownTimeoutEnv(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "systems.json")
	if err := os.WriteFile(path, []byte(`[{"id":"sys-a","enabled":true,"api_key":"secret-a"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALPHA_PROXY_SYSTEMS_FILE", path)
	t.Setenv("ALPHA_PROXY_SHUTDOWN_TIMEOUT", "30s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 30s", cfg.ShutdownTimeout)
	}
}

func TestLoadShutdownTimeoutInvalid(t *testing.T) {
	clearEnv(t)
	t.Setenv("ALPHA_PROXY_SHUTDOWN_TIMEOUT", "not-a-duration")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid shutdown timeout")
	}
	if !IsEnvParseError(err) {
		t.Fatalf("expected env parse error, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "ALPHA_PROXY_SHUTDOWN_TIMEOUT") {
		t.Errorf("error must name the parameter, got: %v", err)
	}
	if strings.Contains(err.Error(), "not-a-duration") {
		t.Errorf("error must not include the value, got: %v", err)
	}
}

func TestValidateShutdownTimeoutZero(t *testing.T) {
	base := validConfig()
	base.ShutdownTimeout = 0
	if err := base.Validate(); err == nil {
		t.Fatal("expected error: zero shutdown timeout")
	}
}

func TestValidateShutdownTimeoutNegative(t *testing.T) {
	base := validConfig()
	base.ShutdownTimeout = -time.Second
	if err := base.Validate(); err == nil {
		t.Fatal("expected error: negative shutdown timeout")
	}
}

func TestValidateShutdownTimeoutPositive(t *testing.T) {
	base := validConfig()
	base.ShutdownTimeout = 5 * time.Second
	if err := base.Validate(); err != nil {
		t.Fatalf("valid shutdown timeout rejected: %v", err)
	}
}

func validConfig() Config {
	return Config{
		Addr:                   ":8080",
		ReadTimeout:            10 * time.Second,
		ReadHeaderTimeout:      5 * time.Second,
		WriteTimeout:           10 * time.Second,
		IdleTimeout:            60 * time.Second,
		BodyLimit:              1 << 20,
		ProcessingTimeout:      5 * time.Second,
		ParallelLimit:          16,
		OverloadRetryAfter:     time.Second,
		GlobalRateLimitRPS:     2000,
		GlobalRateLimitBurst:   2000,
		ConsumerRateLimitRPS:   0,
		ConsumerRateLimitBurst: 0,
		ShutdownTimeout:        15 * time.Second,
		RunMode:                RunModeFinal,
		AuthMode:               AuthModeAPIKey,
		ProcessorMode:          ProcessorReal,
		Systems:                []System{{ID: "sys-a", Enabled: true, APIKey: "secret-a"}},
		MLAddress:              "127.0.0.1:50051",
	}
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"ALPHA_PROXY_ADDR",
		"ALPHA_PROXY_READ_TIMEOUT",
		"ALPHA_PROXY_READ_HEADER_TIMEOUT",
		"ALPHA_PROXY_WRITE_TIMEOUT",
		"ALPHA_PROXY_IDLE_TIMEOUT",
		"ALPHA_PROXY_BODY_LIMIT",
		"ALPHA_PROXY_PROCESSING_TIMEOUT",
		"ALPHA_PROXY_PARALLEL_LIMIT",
		"ALPHA_PROXY_OVERLOAD_RETRY_AFTER",
		"ALPHA_PROXY_GLOBAL_RATE_LIMIT_RPS",
		"ALPHA_PROXY_GLOBAL_RATE_LIMIT_BURST",
		"ALPHA_PROXY_CONSUMER_RATE_LIMIT_RPS",
		"ALPHA_PROXY_CONSUMER_RATE_LIMIT_BURST",
		"ALPHA_PROXY_METRICS_ENABLED",
		"ALPHA_PROXY_SHUTDOWN_TIMEOUT",
		"ALPHA_PROXY_RUN_MODE",
		"ALPHA_PROXY_AUTH_MODE",
		"ALPHA_PROXY_PROCESSOR_MODE",
		"ALPHA_PROXY_SYSTEMS_FILE",
		"ALPHA_PROXY_ML_ADDR",
	} {
		t.Setenv(k, "")
	}
}

func TestValidateEmptyMLAddress(t *testing.T) {
	cfg := Config{
		Addr:               ":8080",
		ReadTimeout:        time.Second,
		ReadHeaderTimeout:  time.Second,
		WriteTimeout:       time.Second,
		IdleTimeout:        time.Second,
		BodyLimit:          1024,
		ProcessingTimeout:  time.Second,
		ParallelLimit:      1,
		OverloadRetryAfter: time.Second,
		ShutdownTimeout:    time.Second,
		RunMode:            RunModeVerify,
		AuthMode:           AuthModeVerify,
		ProcessorMode:      ProcessorReal,
		MLAddress:          "",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty ml address")
	}
}
