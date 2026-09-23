package policy

import (
	"context"
	"errors"
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

func newTestPolicies() map[string]pii.Policy {
	return map[string]pii.Policy{
		"consumer-1": {
			AllowedKinds: map[pii.PIIKind]bool{
				pii.PIIKindEmail: true,
				pii.PIIKindPhone: true,
			},
			DetokenizationAllowed: true,
			MinConfidence:         0.8,
			MaskingStrategy:       "token",
			AllowPartialResult:    true,
		},
	}
}

func TestGetExistingPolicy(t *testing.T) {
	p := NewStaticProvider(newTestPolicies())

	pol, err := p.Get(context.Background(), "consumer-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if !pol.DetokenizationAllowed {
		t.Fatal("expected DetokenizationAllowed to be true")
	}
	if pol.MinConfidence != 0.8 {
		t.Fatalf("expected MinConfidence 0.8, got %v", pol.MinConfidence)
	}
	if pol.MaskingStrategy != "token" {
		t.Fatalf("expected MaskingStrategy token, got %q", pol.MaskingStrategy)
	}
	if !pol.AllowPartialResult {
		t.Fatal("expected AllowPartialResult to be true")
	}
	if !pol.AllowedKinds[pii.PIIKindEmail] || !pol.AllowedKinds[pii.PIIKindPhone] {
		t.Fatalf("unexpected AllowedKinds: %+v", pol.AllowedKinds)
	}
}

func TestGetUnknownConsumerRejected(t *testing.T) {
	p := NewStaticProvider(newTestPolicies())

	_, err := p.Get(context.Background(), "unknown")
	if !errors.Is(err, pii.ErrPolicyRejected) {
		t.Fatalf("expected ErrPolicyRejected, got %v", err)
	}
}

func TestGetCancelledContext(t *testing.T) {
	p := NewStaticProvider(newTestPolicies())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Get(ctx, "consumer-1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestDefensiveCopyOnCreate(t *testing.T) {
	policies := newTestPolicies()
	p := NewStaticProvider(policies)

	// Mutate the input map and nested AllowedKinds after creation.
	pol := policies["consumer-1"]
	pol.AllowedKinds[pii.PIIKindEmail] = false
	pol.MinConfidence = 0.1
	pol.MaskingStrategy = "mutated"
	policies["consumer-1"] = pol
	policies["consumer-2"] = pii.Policy{}

	pol, err := p.Get(context.Background(), "consumer-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if !pol.AllowedKinds[pii.PIIKindEmail] {
		t.Fatal("stored AllowedKinds was mutated via input map")
	}
	if pol.MinConfidence != 0.8 {
		t.Fatalf("stored MinConfidence was mutated, got %v", pol.MinConfidence)
	}
	if pol.MaskingStrategy != "token" {
		t.Fatalf("stored MaskingStrategy was mutated, got %q", pol.MaskingStrategy)
	}

	if _, err := p.Get(context.Background(), "consumer-2"); !errors.Is(err, pii.ErrPolicyRejected) {
		t.Fatalf("expected consumer-2 to be absent, got %v", err)
	}
}

func TestDefensiveCopyOnGet(t *testing.T) {
	p := NewStaticProvider(newTestPolicies())

	pol, err := p.Get(context.Background(), "consumer-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	// Mutate the returned policy and its AllowedKinds.
	pol.AllowedKinds[pii.PIIKindEmail] = false
	pol.AllowedKinds[pii.PIIKindPhone] = false
	pol.MinConfidence = 0.1
	pol.MaskingStrategy = "mutated"

	again, err := p.Get(context.Background(), "consumer-1")
	if err != nil {
		t.Fatalf("second Get returned error: %v", err)
	}
	if !again.AllowedKinds[pii.PIIKindEmail] || !again.AllowedKinds[pii.PIIKindPhone] {
		t.Fatal("stored AllowedKinds was mutated via Get result")
	}
	if again.MinConfidence != 0.8 {
		t.Fatalf("stored MinConfidence was mutated via Get result, got %v", again.MinConfidence)
	}
	if again.MaskingStrategy != "token" {
		t.Fatalf("stored MaskingStrategy was mutated via Get result, got %q", again.MaskingStrategy)
	}
}

func TestDefensiveCopyMaskConditions(t *testing.T) {
	policies := map[string]pii.Policy{
		"consumer-1": {
			AllowedKinds:  map[pii.PIIKind]bool{pii.PIIKindPIN: true, pii.PIIKindBankCard: true},
			MinConfidence: 0.5,
			MaskConditions: map[pii.PIIKind]pii.MaskCondition{
				pii.PIIKindPIN: {
					RequiresAll: []pii.PIIKind{pii.PIIKindBankCard},
					RequiresAny: []pii.PIIKind{pii.PIIKindBankCard, pii.PIIKindCVV},
				},
			},
		},
	}
	p := NewStaticProvider(policies)

	// Mutate the input map's nested MaskConditions after creation.
	pol := policies["consumer-1"]
	cond := pol.MaskConditions[pii.PIIKindPIN]
	cond.RequiresAll[0] = pii.PIIKindCVV
	cond.RequiresAny = append(cond.RequiresAny, pii.PIIKindPhone)
	pol.MaskConditions[pii.PIIKindPIN] = cond
	policies["consumer-1"] = pol

	got, err := p.Get(context.Background(), "consumer-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	gotCond := got.MaskConditions[pii.PIIKindPIN]
	if len(gotCond.RequiresAll) != 1 || gotCond.RequiresAll[0] != pii.PIIKindBankCard {
		t.Fatalf("stored RequiresAll was mutated via input map: %v", gotCond.RequiresAll)
	}
	if len(gotCond.RequiresAny) != 2 {
		t.Fatalf("stored RequiresAny was mutated via input map: %v", gotCond.RequiresAny)
	}

	// Mutate the returned policy's nested MaskConditions.
	gotCond.RequiresAll[0] = pii.PIIKindPhone
	gotCond.RequiresAny = nil
	got.MaskConditions[pii.PIIKindPIN] = gotCond

	again, err := p.Get(context.Background(), "consumer-1")
	if err != nil {
		t.Fatalf("second Get returned error: %v", err)
	}
	cond2 := again.MaskConditions[pii.PIIKindPIN]
	if len(cond2.RequiresAll) != 1 || cond2.RequiresAll[0] != pii.PIIKindBankCard {
		t.Fatalf("stored RequiresAll was mutated via Get result: %v", cond2.RequiresAll)
	}
	if len(cond2.RequiresAny) != 2 {
		t.Fatalf("stored RequiresAny was mutated via Get result: %v", cond2.RequiresAny)
	}
}
