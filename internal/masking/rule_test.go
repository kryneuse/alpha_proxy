package masking

import (
	"context"
	"errors"
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

type fakeDetector struct {
	entities []pii.Entity
	err      error
}

func (f *fakeDetector) Detect(_ context.Context, _ string) ([]pii.Entity, error) {
	return f.entities, f.err
}

func testPolicy() pii.Policy {
	return pii.Policy{
		AllowedKinds: map[pii.PIIKind]bool{
			pii.PIIKindPhone: true,
		},
		MinConfidence: 0.5,
	}
}

func TestRuleMaskerPhone(t *testing.T) {
	d := &fakeDetector{entities: []pii.Entity{
		{Kind: pii.PIIKindPhone, Start: 5, End: 16, Confidence: 0.9, Source: pii.SourceReg},
	}}
	m, err := NewRuleMasker(d)
	if err != nil {
		t.Fatalf("NewRuleMasker returned error: %v", err)
	}

	masked, mappings, err := m.Mask(context.Background(), "call 79123456789", testPolicy())
	if err != nil {
		t.Fatalf("Mask returned error: %v", err)
	}
	if masked != "call <PHONE_1>" {
		t.Fatalf("unexpected masked text: %q", masked)
	}
	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(mappings))
	}
}

func TestRuleMaskerForbiddenKind(t *testing.T) {
	d := &fakeDetector{entities: []pii.Entity{
		{Kind: pii.PIIKindINN, Start: 5, End: 16, Confidence: 0.9, Source: pii.SourceReg},
	}}
	m, err := NewRuleMasker(d)
	if err != nil {
		t.Fatalf("NewRuleMasker returned error: %v", err)
	}

	masked, mappings, err := m.Mask(context.Background(), "call 79123456789", testPolicy())
	if err != nil {
		t.Fatalf("Mask returned error: %v", err)
	}
	if masked != "call 79123456789" {
		t.Fatalf("expected unchanged text, got %q", masked)
	}
	if len(mappings) != 0 {
		t.Fatalf("expected 0 mappings, got %d", len(mappings))
	}
}

func TestRuleMaskerDetectorError(t *testing.T) {
	d := &fakeDetector{err: errors.New("detector failed")}
	m, err := NewRuleMasker(d)
	if err != nil {
		t.Fatalf("NewRuleMasker returned error: %v", err)
	}

	_, _, err = m.Mask(context.Background(), "call 79123456789", testPolicy())
	if err == nil || err.Error() != "detector failed" {
		t.Fatalf("expected detector error, got %v", err)
	}
}

func TestRuleMaskerCancelledContext(t *testing.T) {
	d := &fakeDetector{}
	m, err := NewRuleMasker(d)
	if err != nil {
		t.Fatalf("NewRuleMasker returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err = m.Mask(ctx, "call 79123456789", testPolicy())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRuleMaskerEmptyEntities(t *testing.T) {
	d := &fakeDetector{}
	m, err := NewRuleMasker(d)
	if err != nil {
		t.Fatalf("NewRuleMasker returned error: %v", err)
	}

	masked, mappings, err := m.Mask(context.Background(), "call 79123456789", testPolicy())
	if err != nil {
		t.Fatalf("Mask returned error: %v", err)
	}
	if masked != "call 79123456789" {
		t.Fatalf("expected unchanged text, got %q", masked)
	}
	if len(mappings) != 0 {
		t.Fatalf("expected 0 mappings, got %d", len(mappings))
	}
}

func TestNewRuleMaskerNilDetector(t *testing.T) {
	if _, err := NewRuleMasker(nil); err == nil {
		t.Fatal("expected error for nil detector")
	}
}
