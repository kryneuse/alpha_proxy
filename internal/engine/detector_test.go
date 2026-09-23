package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/normalize"
	"github.com/kryneuse/alpha_proxy/internal/pii"
	"github.com/kryneuse/alpha_proxy/internal/recognizer"
)

func TestMapKindAllTypes(t *testing.T) {
	cases := []struct {
		entityType entity.Type
		want       pii.PIIKind
	}{
		{entity.FULL_NAME, pii.PIIKindFullName},
		{entity.BIRTH_DATE, pii.PIIKindDate},
		{entity.PASSPORT_ISSUE_DATE, pii.PIIKindDate},
		{entity.BIRTH_PLACE, pii.PIIKindBirthPlace},
		{entity.PASSPORT, pii.PIIKindPassport},
		{entity.CITIZENSHIP, pii.PIIKindCitizenship},
		{entity.PASSPORT_ISSUER, pii.PIIKindPassportIssuer},
		{entity.DEPARTMENT_CODE, pii.PIIKindPassportDivision},
		{entity.DRIVER_LICENSE, pii.PIIKindDriverLicense},
		{entity.ADDRESS, pii.PIIKindAddress},
		{entity.EMAIL, pii.PIIKindEmail},
		{entity.PHONE, pii.PIIKindPhone},
		{entity.INN, pii.PIIKindINN},
		{entity.CARD_NUMBER, pii.PIIKindBankCard},
		{entity.CVV, pii.PIIKindCVV},
		{entity.PIN, pii.PIIKindPIN},
		{entity.CARDHOLDER_NAME, pii.PIIKindCardHolderName},
	}
	for _, c := range cases {
		got, ok := mapKind(c.entityType)
		if !ok {
			t.Errorf("mapKind(%s) not found", c.entityType)
			continue
		}
		if got != c.want {
			t.Errorf("mapKind(%s) = %s, want %s", c.entityType, got, c.want)
		}
	}
}

func TestMapKindUnknown(t *testing.T) {
	if _, ok := mapKind(entity.Type("UNKNOWN")); ok {
		t.Fatal("expected unknown type to not map")
	}
}

func TestDetectEmail(t *testing.T) {
	e := New(Options{})
	text := "contact a@example.com"
	entities, err := e.Detect(context.Background(), text)
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("expected at least one entity")
	}
	found := false
	for _, en := range entities {
		if en.Kind == pii.PIIKindEmail {
			found = true
			if en.Source != pii.SourceReg {
				t.Fatalf("expected SourceReg, got %q", en.Source)
			}
			if en.Start < 0 || en.End > len(text) || en.Start >= en.End {
				t.Fatalf("invalid span: %+v", en)
			}
		}
	}
	if !found {
		t.Fatal("expected email entity")
	}
}

func TestDetectPhone(t *testing.T) {
	e := New(Options{})
	text := "call 79123456789"
	entities, err := e.Detect(context.Background(), text)
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	found := false
	for _, en := range entities {
		if en.Kind == pii.PIIKindPhone {
			found = true
			if en.Source != pii.SourceReg {
				t.Fatalf("expected SourceReg, got %q", en.Source)
			}
		}
	}
	if !found {
		t.Fatal("expected phone entity")
	}
}

func TestDetectCyrillic(t *testing.T) {
	e := New(Options{})
	text := "почта a@example.com"
	entities, err := e.Detect(context.Background(), text)
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	for _, en := range entities {
		if en.Kind == pii.PIIKindEmail {
			if text[en.Start:en.End] != "a@example.com" {
				t.Fatalf("unexpected slice: %q", text[en.Start:en.End])
			}
		}
	}
}

func TestDetectCancelledContext(t *testing.T) {
	e := New(Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := e.Detect(ctx, "call 79123456789")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// fakeRecognizer returns a fixed set of candidate spans.
type fakeRecognizer struct {
	typ   entity.Type
	spans []entity.CandidateSpan
}

func (f *fakeRecognizer) Type() entity.Type { return f.typ }
func (f *fakeRecognizer) Recognize(_ *normalize.Text) []entity.CandidateSpan {
	return f.spans
}

func TestDetectInvalidSpan(t *testing.T) {
	reg := recognizer.NewRegistry(&fakeRecognizer{
		typ: entity.EMAIL,
		spans: []entity.CandidateSpan{
			{Type: entity.EMAIL, Text: "a@example.com", Start: -1, End: 13, Score: 0.9},
		},
	})
	e := NewWithRegistry(reg, Options{MinScore: 0.5})
	_, err := e.Detect(context.Background(), "a@example.com")
	if !errors.Is(err, pii.ErrInvalidSpan) {
		t.Fatalf("expected ErrInvalidSpan, got %v", err)
	}
}

func TestDetectUnknownType(t *testing.T) {
	reg := recognizer.NewRegistry(&fakeRecognizer{
		typ: entity.Type("UNKNOWN"),
		spans: []entity.CandidateSpan{
			{Type: entity.Type("UNKNOWN"), Text: "x", Start: 0, End: 1, Score: 0.9},
		},
	})
	e := NewWithRegistry(reg, Options{MinScore: 0.5})
	_, err := e.Detect(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestDetectTextMismatch(t *testing.T) {
	reg := recognizer.NewRegistry(&fakeRecognizer{
		typ: entity.EMAIL,
		spans: []entity.CandidateSpan{
			{Type: entity.EMAIL, Text: "wrong@example.com", Start: 0, End: 13, Score: 0.9},
		},
	})
	e := NewWithRegistry(reg, Options{MinScore: 0.5})
	_, err := e.Detect(context.Background(), "a@example.com")
	if !errors.Is(err, pii.ErrInvalidSpan) {
		t.Fatalf("expected ErrInvalidSpan, got %v", err)
	}
}
