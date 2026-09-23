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

// pinCondition returns the default PIN condition (requires CARD_NUMBER).
func pinCondition() pii.MaskCondition {
	return pii.MaskCondition{RequiresAll: []pii.PIIKind{pii.PIIKindBankCard}}
}

func TestBuildPoliciesDefaultForVerify(t *testing.T) {
	cfg := config.Config{}
	policies := buildPolicies(cfg)
	pol, ok := policies[auth.VerifyConsumerID]
	if !ok {
		t.Fatal("expected verify consumer policy")
	}
	cond, ok := pol.MaskConditions[pii.PIIKindPIN]
	if !ok {
		t.Fatal("expected default PIN condition")
	}
	if len(cond.RequiresAll) != 1 || cond.RequiresAll[0] != pii.PIIKindBankCard {
		t.Fatalf("expected default PIN requires CARD_NUMBER, got %+v", cond)
	}
}

// Case 1: system without masking_conditions -> gets default PIN requires CARD.
func TestBuildPoliciesSystemWithoutConditionsGetsDefault(t *testing.T) {
	cfg := config.Config{Systems: []config.System{
		{ID: "sys-a", Enabled: true, APIKey: "k"},
	}}
	policies := buildPolicies(cfg)
	pol, ok := policies["sys-a"]
	if !ok {
		t.Fatal("expected sys-a policy")
	}
	cond, ok := pol.MaskConditions[pii.PIIKindPIN]
	if !ok {
		t.Fatal("expected default PIN condition for system without masking_conditions")
	}
	if len(cond.RequiresAll) != 1 || cond.RequiresAll[0] != pii.PIIKindBankCard {
		t.Fatalf("expected default PIN requires CARD_NUMBER, got %+v", cond)
	}
}

// Case 2: system adds only CVV rule -> PIN default preserved.
func TestBuildPoliciesSystemAddsCvvKeepsPinDefault(t *testing.T) {
	cfg := config.Config{Systems: []config.System{
		{
			ID: "sys-a", Enabled: true, APIKey: "k",
			MaskingConditions: map[string]config.MaskConditionConfig{
				"cvv": {RequiresAll: []string{"card"}},
			},
		},
	}}
	policies := buildPolicies(cfg)
	pol := policies["sys-a"]

	// PIN default preserved.
	pinCond, ok := pol.MaskConditions[pii.PIIKindPIN]
	if !ok {
		t.Fatal("expected PIN default preserved")
	}
	if len(pinCond.RequiresAll) != 1 || pinCond.RequiresAll[0] != pii.PIIKindBankCard {
		t.Fatalf("expected PIN requires CARD_NUMBER preserved, got %+v", pinCond)
	}

	// CVV rule added.
	cvvCond, ok := pol.MaskConditions[pii.PIIKindCVV]
	if !ok {
		t.Fatal("expected CVV condition added")
	}
	if len(cvvCond.RequiresAll) != 1 || cvvCond.RequiresAll[0] != pii.PIIKindBankCard {
		t.Fatalf("expected CVV requires CARD_NUMBER, got %+v", cvvCond)
	}
}

// Case 3: system sets "pin": {} -> default PIN dependency explicitly disabled.
func TestBuildPoliciesSystemDisablesPinDefault(t *testing.T) {
	cfg := config.Config{Systems: []config.System{
		{
			ID: "sys-a", Enabled: true, APIKey: "k",
			MaskingConditions: map[string]config.MaskConditionConfig{
				"pin": {},
			},
		},
	}}
	policies := buildPolicies(cfg)
	pol := policies["sys-a"]

	cond, ok := pol.MaskConditions[pii.PIIKindPIN]
	if !ok {
		t.Fatal("expected PIN condition present (empty = always satisfied)")
	}
	if len(cond.RequiresAll) != 0 || len(cond.RequiresAny) != 0 {
		t.Fatalf("expected empty PIN condition (always satisfied), got %+v", cond)
	}
	if !cond.Satisfied(map[pii.PIIKind]bool{}) {
		t.Fatal("empty condition must be always satisfied")
	}
}

// Case 4: system overrides PIN with another condition -> override applied.
func TestBuildPoliciesSystemOverridesPin(t *testing.T) {
	cfg := config.Config{Systems: []config.System{
		{
			ID: "sys-a", Enabled: true, APIKey: "k",
			MaskingConditions: map[string]config.MaskConditionConfig{
				"pin": {RequiresAny: []string{"card", "cvv"}},
			},
		},
	}}
	policies := buildPolicies(cfg)
	pol := policies["sys-a"]

	cond, ok := pol.MaskConditions[pii.PIIKindPIN]
	if !ok {
		t.Fatal("expected PIN condition")
	}
	if len(cond.RequiresAll) != 0 || len(cond.RequiresAny) != 2 {
		t.Fatalf("expected override RequiresAny=[card,cvv], got %+v", cond)
	}
	if cond.RequiresAny[0] != pii.PIIKindBankCard || cond.RequiresAny[1] != pii.PIIKindCVV {
		t.Fatalf("unexpected override RequiresAny: %v", cond.RequiresAny)
	}
}

// Case 5: unknown kind -> validation error is covered by
// config.TestValidateMaskingConditionsUnknownTarget (config.Validate rejects
// unknown masking_conditions kinds before buildPolicies is reached).
