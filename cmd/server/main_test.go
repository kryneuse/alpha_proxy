package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunRejectsNilContext(t *testing.T) {
	if err := run(nil); err == nil {
		t.Fatal("run(nil) error = nil, want error")
	}
}

func TestRunStopsOnCancelledContext(t *testing.T) {
	setDevEnv(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := make(chan error, 1)
	go func() {
		result <- run(ctx)
	}()

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("run() error = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not stop after context cancellation")
	}
}

func TestRunInvalidConfig(t *testing.T) {
	setDevEnv(t)
	t.Setenv("ALPHA_PROXY_SHUTDOWN_TIMEOUT", "invalid-test-value")

	err := run(context.Background())
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "load config") {
		t.Errorf("error must contain 'load config' prefix, got: %v", err)
	}
	if !strings.Contains(err.Error(), "ALPHA_PROXY_SHUTDOWN_TIMEOUT") {
		t.Errorf("error must name the parameter, got: %v", err)
	}
	if strings.Contains(err.Error(), "invalid-test-value") {
		t.Errorf("error must not include the value, got: %v", err)
	}
}

func TestRunListenError(t *testing.T) {
	setDevEnv(t)
	t.Setenv("ALPHA_PROXY_ADDR", "invalid-address-without-port")

	err := run(context.Background())
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "listen") {
		t.Errorf("error must contain 'listen' prefix, got: %v", err)
	}
}

// setDevEnv sets a minimal valid environment for dev/verify/mock mode. It
// explicitly sets every variable that can affect config.Load.
func setDevEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ALPHA_PROXY_ADDR", "127.0.0.1:0")
	t.Setenv("ALPHA_PROXY_RUN_MODE", "dev")
	t.Setenv("ALPHA_PROXY_AUTH_MODE", "verify")
	t.Setenv("ALPHA_PROXY_PROCESSOR_MODE", "mock")
	t.Setenv("ALPHA_PROXY_METRICS_ENABLED", "false")
	t.Setenv("ALPHA_PROXY_GLOBAL_RATE_LIMIT_RPS", "0")
	t.Setenv("ALPHA_PROXY_GLOBAL_RATE_LIMIT_BURST", "0")
	t.Setenv("ALPHA_PROXY_CONSUMER_RATE_LIMIT_RPS", "0")
	t.Setenv("ALPHA_PROXY_CONSUMER_RATE_LIMIT_BURST", "0")
	t.Setenv("ALPHA_PROXY_SHUTDOWN_TIMEOUT", "1s")
	t.Setenv("ALPHA_PROXY_SYSTEMS_FILE", "")
}
