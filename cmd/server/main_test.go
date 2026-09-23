package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/auth"
	"github.com/kryneuse/alpha_proxy/internal/config"
	"github.com/kryneuse/alpha_proxy/internal/contract"
	"github.com/kryneuse/alpha_proxy/internal/pii"
)

func TestRunRejectsNilContext(t *testing.T) {
	//nolint:staticcheck // SA1012: this test intentionally verifies rejection of a nil context.
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

func boolPtr(b bool) *bool { return &b }

func strSlicePtr(s []string) *[]string { return &s }

func TestBuildPoliciesDifferentAllowedKinds(t *testing.T) {
	cfg := config.Config{
		Systems: []config.System{
			{ID: "support", Enabled: true, APIKey: "k1", MaskKinds: strSlicePtr([]string{"phone"})},
			{ID: "analytics", Enabled: true, APIKey: "k2", MaskKinds: strSlicePtr([]string{"email"})},
		},
	}
	policies, err := buildPolicies(cfg)
	if err != nil {
		t.Fatalf("buildPolicies error: %v", err)
	}
	support := policies["support"]
	analytics := policies["analytics"]
	if support.AllowedKinds[pii.PIIKindPhone] && !support.AllowedKinds[pii.PIIKindEmail] {
		// ok
	} else {
		t.Errorf("support AllowedKinds = %v, want only phone", support.AllowedKinds)
	}
	if analytics.AllowedKinds[pii.PIIKindEmail] && !analytics.AllowedKinds[pii.PIIKindPhone] {
		// ok
	} else {
		t.Errorf("analytics AllowedKinds = %v, want only email", analytics.AllowedKinds)
	}
}

func TestBuildPoliciesDetokenizationPerSystem(t *testing.T) {
	cfg := config.Config{
		Systems: []config.System{
			{ID: "support", Enabled: true, APIKey: "k1", DetokenizationAllowed: boolPtr(true)},
			{ID: "analytics", Enabled: true, APIKey: "k2", DetokenizationAllowed: boolPtr(false)},
		},
	}
	policies, err := buildPolicies(cfg)
	if err != nil {
		t.Fatalf("buildPolicies error: %v", err)
	}
	if !policies["support"].DetokenizationAllowed {
		t.Errorf("support DetokenizationAllowed = false, want true")
	}
	if policies["analytics"].DetokenizationAllowed {
		t.Errorf("analytics DetokenizationAllowed = true, want false")
	}
}

func TestBuildPoliciesEmptyMaskKinds(t *testing.T) {
	cfg := config.Config{
		Systems: []config.System{
			{ID: "sys-a", Enabled: true, APIKey: "k1", MaskKinds: strSlicePtr([]string{})},
		},
	}
	policies, err := buildPolicies(cfg)
	if err != nil {
		t.Fatalf("buildPolicies error: %v", err)
	}
	if len(policies["sys-a"].AllowedKinds) != 0 {
		t.Errorf("AllowedKinds = %v, want empty", policies["sys-a"].AllowedKinds)
	}
}

func TestBuildPoliciesAbsentMaskKindsAllowsAll(t *testing.T) {
	cfg := config.Config{
		Systems: []config.System{
			{ID: "sys-a", Enabled: true, APIKey: "k1"},
		},
	}
	policies, err := buildPolicies(cfg)
	if err != nil {
		t.Fatalf("buildPolicies error: %v", err)
	}
	if len(policies["sys-a"].AllowedKinds) != len(allKinds()) {
		t.Errorf("AllowedKinds = %v, want all kinds", policies["sys-a"].AllowedKinds)
	}
}

func TestBuildPoliciesUnknownKindError(t *testing.T) {
	cfg := config.Config{
		Systems: []config.System{
			{ID: "sys-a", Enabled: true, APIKey: "k1", MaskKinds: strSlicePtr([]string{"bogus"})},
		},
	}
	_, err := buildPolicies(cfg)
	if err == nil {
		t.Fatal("expected error for unknown mask kind")
	}
	if !strings.Contains(err.Error(), "sys-a") {
		t.Errorf("error must name the system, got: %v", err)
	}
	if strings.Contains(err.Error(), "k1") {
		t.Errorf("error must not include the api key, got: %v", err)
	}
}

func TestBuildPoliciesMutationIsolated(t *testing.T) {
	cfg := config.Config{
		Systems: []config.System{
			{ID: "support", Enabled: true, APIKey: "k1", MaskKinds: strSlicePtr([]string{"phone"})},
			{ID: "analytics", Enabled: true, APIKey: "k2", MaskKinds: strSlicePtr([]string{"email"})},
		},
	}
	policies, err := buildPolicies(cfg)
	if err != nil {
		t.Fatalf("buildPolicies error: %v", err)
	}
	// Mutate one system's policy; the other must be unaffected.
	policies["support"].AllowedKinds[pii.PIIKindINN] = true
	if policies["analytics"].AllowedKinds[pii.PIIKindINN] {
		t.Errorf("analytics AllowedKinds mutated via support, got %v", policies["analytics"].AllowedKinds)
	}
	if policies[auth.VerifyConsumerID].AllowedKinds[pii.PIIKindINN] != true {
		t.Errorf("verify AllowedKinds must allow inn, got %v", policies[auth.VerifyConsumerID].AllowedKinds)
	}
}
