package processor_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/contract"
	"github.com/kryneuse/alpha_proxy/internal/engine"
	"github.com/kryneuse/alpha_proxy/internal/masking"
	"github.com/kryneuse/alpha_proxy/internal/pii"
	"github.com/kryneuse/alpha_proxy/internal/policy"
	"github.com/kryneuse/alpha_proxy/internal/processor"
	"github.com/kryneuse/alpha_proxy/internal/store"
)

// conditionalPolicy returns a policy with the default business rule:
// PIN requires CARD_NUMBER.
func conditionalPolicy() pii.Policy {
	return pii.Policy{
		AllowedKinds: map[pii.PIIKind]bool{
			pii.PIIKindPIN:      true,
			pii.PIIKindBankCard: true,
		},
		DetokenizationAllowed: true,
		MinConfidence:         0.5,
		MaskConditions: map[pii.PIIKind]pii.MaskCondition{
			pii.PIIKindPIN: {RequiresAll: []pii.PIIKind{pii.PIIKindBankCard}},
		},
	}
}

// TestConditionalMaskingIntegration exercises the real Rule Engine + real
// masking path (RuleMasker -> processor) with the conditional PIN rule.
func TestConditionalMaskingIntegration(t *testing.T) {
	eng := engine.New(engine.Options{})
	masker, err := masking.NewRuleMasker(eng)
	if err != nil {
		t.Fatalf("NewRuleMasker returned error: %v", err)
	}

	st := store.NewMemoryStore(100)
	pp := policy.NewStaticProvider(map[string]pii.Policy{"consumer-1": conditionalPolicy()})

	p, err := processor.New(st, pp, masker, time.Hour)
	if err != nil {
		t.Fatalf("processor.New returned error: %v", err)
	}

	ctx := context.Background()

	// CASE 1: PIN only -> not masked.
	resp1, err := p.Process(ctx, contract.ProcessRequest{
		Payload:    "Пин-код 7305",
		PayloadID:  "id-pin-only",
		ConsumerID: "consumer-1",
	})
	if err != nil {
		t.Fatalf("PIN-only Process returned error: %v", err)
	}
	if resp1.Result != "Пин-код 7305" {
		t.Fatalf("expected PIN not masked, got %q", resp1.Result)
	}

	// CASE 2: PIN + CARD_NUMBER -> both masked.
	resp2, err := p.Process(ctx, contract.ProcessRequest{
		Payload:    "Номер карты 4111 1111 1111 1111, пин-код 7305",
		PayloadID:  "id-pin-card",
		ConsumerID: "consumer-1",
	})
	if err != nil {
		t.Fatalf("PIN+card Process returned error: %v", err)
	}
	if strings.Contains(resp2.Result, "4111 1111 1111 1111") {
		t.Fatalf("expected card masked, got %q", resp2.Result)
	}
	if strings.Contains(resp2.Result, "7305") {
		t.Fatalf("expected PIN masked, got %q", resp2.Result)
	}

	// CASE 3: CARD_NUMBER only -> masked as before.
	resp3, err := p.Process(ctx, contract.ProcessRequest{
		Payload:    "Номер карты 4111 1111 1111 1111",
		PayloadID:  "id-card-only",
		ConsumerID: "consumer-1",
	})
	if err != nil {
		t.Fatalf("card-only Process returned error: %v", err)
	}
	if strings.Contains(resp3.Result, "4111 1111 1111 1111") {
		t.Fatalf("expected card masked, got %q", resp3.Result)
	}

	// Retry/detokenization: PIN-only produced no mapping, so no token to restore.
	sess1, err := st.Get(ctx, "id-pin-only")
	if err != nil {
		t.Fatalf("Get pin-only session returned error: %v", err)
	}
	if len(sess1.Mappings) != 0 {
		t.Fatalf("expected 0 mappings for unmasked PIN, got %d", len(sess1.Mappings))
	}

	// PIN+card produced mappings; detokenization restores them.
	sess2, err := st.Get(ctx, "id-pin-card")
	if err != nil {
		t.Fatalf("Get pin-card session returned error: %v", err)
	}
	if len(sess2.Mappings) != 2 {
		t.Fatalf("expected 2 mappings for masked PIN+card, got %d", len(sess2.Mappings))
	}
	detok, err := p.Process(ctx, contract.ProcessRequest{
		Payload:    resp2.Result,
		PayloadID:  "id-pin-card",
		ConsumerID: "consumer-1",
	})
	if err != nil {
		t.Fatalf("detokenize Process returned error: %v", err)
	}
	if detok.Result != "Номер карты 4111 1111 1111 1111, пин-код 7305" {
		t.Fatalf("expected restored text, got %q", detok.Result)
	}
}
