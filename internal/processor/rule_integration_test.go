package processor_test

import (
	"context"
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

func TestRuleIntegration(t *testing.T) {
	eng := engine.New(engine.Options{})
	masker, err := masking.NewRuleMasker(eng)
	if err != nil {
		t.Fatalf("NewRuleMasker returned error: %v", err)
	}

	st := store.NewMemoryStore(100)
	pol := pii.Policy{
		AllowedKinds:          map[pii.PIIKind]bool{pii.PIIKindPhone: true},
		DetokenizationAllowed: true,
		MinConfidence:         0.5,
	}
	pp := policy.NewStaticProvider(map[string]pii.Policy{"consumer-1": pol})

	p, err := processor.New(st, pp, masker, time.Hour)
	if err != nil {
		t.Fatalf("processor.New returned error: %v", err)
	}

	ctx := context.Background()
	payloadID := "id-1"

	// 1. Первый вызов — новый payload, маскирование.
	resp1, err := p.Process(ctx, contract.ProcessRequest{
		Payload:    "call 79123456789",
		PayloadID:  payloadID,
		ConsumerID: "consumer-1",
	})
	if err != nil {
		t.Fatalf("first Process returned error: %v", err)
	}
	if resp1.Result != "call <PHONE_1>" {
		t.Fatalf("expected masked result, got %q", resp1.Result)
	}

	// 2. Retry исходного текста — тот же замаскированный результат.
	resp2, err := p.Process(ctx, contract.ProcessRequest{
		Payload:    "call 79123456789",
		PayloadID:  payloadID,
		ConsumerID: "consumer-1",
	})
	if err != nil {
		t.Fatalf("retry Process returned error: %v", err)
	}
	if resp2.Result != "call <PHONE_1>" {
		t.Fatalf("expected same masked result on retry, got %q", resp2.Result)
	}

	// 3. Детокенизация ответа.
	resp3, err := p.Process(ctx, contract.ProcessRequest{
		Payload:    "result <PHONE_1>",
		PayloadID:  payloadID,
		ConsumerID: "consumer-1",
	})
	if err != nil {
		t.Fatalf("detokenize Process returned error: %v", err)
	}
	if resp3.Result != "result 79123456789" {
		t.Fatalf("expected restored result, got %q", resp3.Result)
	}

	// Проверка session: одна mapping с полными полями.
	sess, err := st.Get(ctx, payloadID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if len(sess.Mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(sess.Mappings))
	}
	m := sess.Mappings[0]
	if m.Token != "<PHONE_1>" {
		t.Fatalf("unexpected token: %q", m.Token)
	}
	if m.Original != "79123456789" {
		t.Fatalf("unexpected original: %q", m.Original)
	}
	if m.Kind != pii.PIIKindPhone {
		t.Fatalf("unexpected kind: %q", m.Kind)
	}
	if m.Source != pii.SourceReg {
		t.Fatalf("unexpected source: %q", m.Source)
	}
	if m.Start != 5 || m.End != 16 {
		t.Fatalf("unexpected span: [%d:%d]", m.Start, m.End)
	}
}
