package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/config"
	"github.com/kryneuse/alpha_proxy/internal/contract"
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

func TestBuildProcessorReal(t *testing.T) {
	cfg := config.Config{
		RunMode:       config.RunModeVerify,
		AuthMode:      config.AuthModeVerify,
		ProcessorMode: config.ProcessorReal,
		MLAddress:     "127.0.0.1:50051",
	}
	proc, cleanup, err := buildProcessor(cfg)
	if err != nil {
		t.Fatalf("buildProcessor returned error: %v", err)
	}
	if proc == nil {
		t.Fatal("expected non-nil processor")
	}
	cleanup()
	cleanup() // повторный вызов не должен паниковать
}

func TestBuildProcessorMock(t *testing.T) {
	cfg := config.Config{
		RunMode:       config.RunModeDev,
		AuthMode:      config.AuthModeVerify,
		ProcessorMode: config.ProcessorMock,
	}
	proc, cleanup, err := buildProcessor(cfg)
	if err != nil {
		t.Fatalf("buildProcessor returned error: %v", err)
	}
	if _, ok := proc.(contract.MockProcessor); !ok {
		t.Fatalf("expected MockProcessor, got %T", proc)
	}
	cleanup()
	cleanup()
}

func TestBuildProcessorMockNotInDev(t *testing.T) {
	cfg := config.Config{
		RunMode:       config.RunModeVerify,
		AuthMode:      config.AuthModeVerify,
		ProcessorMode: config.ProcessorMock,
	}
	if _, _, err := buildProcessor(cfg); err == nil {
		t.Fatal("expected error for mock processor outside dev mode")
	}
}
