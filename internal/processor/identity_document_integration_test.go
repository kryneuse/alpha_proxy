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

// identityDocPolicy allows the identity_document kind (and PIN/CARD for the
// conditional test) with detokenization.
func identityDocPolicy() pii.Policy {
	return pii.Policy{
		AllowedKinds: map[pii.PIIKind]bool{
			pii.PIIKindIdentityDocument: true,
		},
		DetokenizationAllowed: true,
		MinConfidence:         0.5,
	}
}

// TestIdentityDocumentMaskingIntegration exercises the real Rule Engine + real
// masking path for each identity document subtype: detection -> pii.Entity ->
// policy -> replacement plan -> masked payload -> TokenMapping -> detokenization.
func TestIdentityDocumentMaskingIntegration(t *testing.T) {
	eng := engine.New(engine.Options{})
	masker, err := masking.NewRuleMasker(eng)
	if err != nil {
		t.Fatalf("NewRuleMasker returned error: %v", err)
	}

	st := store.NewMemoryStore(100)
	pp := policy.NewStaticProvider(map[string]pii.Policy{"consumer-1": identityDocPolicy()})

	p, err := processor.New(st, pp, masker, time.Hour)
	if err != nil {
		t.Fatalf("processor.New returned error: %v", err)
	}

	ctx := context.Background()

	cases := []struct {
		name    string
		payload string
		secret  string
	}{
		{"foreign_passport", "Загранпаспорт 62 № 1234567", "62 № 1234567"},
		{"birth_certificate", "свидетельство о рождении II-МЮ № 123456", "II-МЮ № 123456"},
		{"military_id", "Военный билет ГД № 1234567", "ГД № 1234567"},
		{"temporary_id", "Временное удостоверение личности № 770041160025", "770041160025"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payloadID := "id-" + tc.name

			// 1. Mask.
			resp, err := p.Process(ctx, contract.ProcessRequest{
				Payload:    tc.payload,
				PayloadID:  payloadID,
				ConsumerID: "consumer-1",
			})
			if err != nil {
				t.Fatalf("Process returned error: %v", err)
			}
			if strings.Contains(resp.Result, tc.secret) {
				t.Fatalf("expected secret %q masked, got %q", tc.secret, resp.Result)
			}
			if !strings.Contains(resp.Result, "<IDENTITY_DOCUMENT_") {
				t.Fatalf("expected IDENTITY_DOCUMENT token in result, got %q", resp.Result)
			}

			// 2. TokenMapping present with correct kind.
			sess, err := st.Get(ctx, payloadID)
			if err != nil {
				t.Fatalf("Get session returned error: %v", err)
			}
			if len(sess.Mappings) != 1 {
				t.Fatalf("expected 1 mapping, got %d", len(sess.Mappings))
			}
			m := sess.Mappings[0]
			if m.Kind != pii.PIIKindIdentityDocument {
				t.Fatalf("expected kind identity_document, got %q", m.Kind)
			}
			if m.Original != tc.secret {
				t.Fatalf("expected original %q, got %q", tc.secret, m.Original)
			}

			// 3. Detokenize restores the original value.
			detok, err := p.Process(ctx, contract.ProcessRequest{
				Payload:    resp.Result,
				PayloadID:  payloadID,
				ConsumerID: "consumer-1",
			})
			if err != nil {
				t.Fatalf("detokenize Process returned error: %v", err)
			}
			if detok.Result != tc.payload {
				t.Fatalf("expected restored payload %q, got %q", tc.payload, detok.Result)
			}
		})
	}
}

// TestIdentityDocumentNotMaskedWithoutContext verifies that an ambiguous number
// without document context is not masked as an identity document.
func TestIdentityDocumentNotMaskedWithoutContext(t *testing.T) {
	eng := engine.New(engine.Options{})
	masker, err := masking.NewRuleMasker(eng)
	if err != nil {
		t.Fatalf("NewRuleMasker returned error: %v", err)
	}
	st := store.NewMemoryStore(100)
	pp := policy.NewStaticProvider(map[string]pii.Policy{"consumer-1": identityDocPolicy()})
	p, err := processor.New(st, pp, masker, time.Hour)
	if err != nil {
		t.Fatalf("processor.New returned error: %v", err)
	}

	ctx := context.Background()
	resp, err := p.Process(ctx, contract.ProcessRequest{
		Payload:    "Заказ 621234567",
		PayloadID:  "id-order",
		ConsumerID: "consumer-1",
	})
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if resp.Result != "Заказ 621234567" {
		t.Fatalf("expected order number not masked, got %q", resp.Result)
	}
}
