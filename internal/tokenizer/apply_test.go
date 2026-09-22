package tokenizer

import (
	"context"
	"errors"
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

func TestApplyReplacementsBasic(t *testing.T) {
	text := "call 79123456789 or a@example.com"
	replacements := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
		{Start: 20, End: 33, Token: "<EMAIL_1>", Original: "a@example.com", Kind: pii.PIIKindEmail, Source: pii.SourceML, Confidence: 0.9},
	}

	masked, mappings, err := ApplyReplacements(context.Background(), text, replacements)
	if err != nil {
		t.Fatalf("ApplyReplacements returned error: %v", err)
	}
	if masked != "call <PHONE_1> or <EMAIL_1>" {
		t.Fatalf("unexpected masked text: %q", masked)
	}
	if len(mappings) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(mappings))
	}
}

func TestApplyReplacementsDifferentLengths(t *testing.T) {
	text := "card 4111111111111111"
	replacements := []pii.Replacement{
		{Start: 5, End: 21, Token: "<CARD_1>", Original: "4111111111111111", Kind: pii.PIIKindBankCard, Source: pii.SourceReg, Confidence: 0.9},
	}

	masked, _, err := ApplyReplacements(context.Background(), text, replacements)
	if err != nil {
		t.Fatalf("ApplyReplacements returned error: %v", err)
	}
	if masked != "card <CARD_1>" {
		t.Fatalf("unexpected masked text: %q", masked)
	}
}

func TestApplyReplacementsCyrillicAndEmoji(t *testing.T) {
	text := "звони 😀 79123456789"
	replacements := []pii.Replacement{
		{Start: 16, End: 27, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}

	masked, _, err := ApplyReplacements(context.Background(), text, replacements)
	if err != nil {
		t.Fatalf("ApplyReplacements returned error: %v", err)
	}
	if masked != "звони 😀 <PHONE_1>" {
		t.Fatalf("unexpected masked text: %q", masked)
	}
}

func TestApplyReplacementsRepeatedToken(t *testing.T) {
	text := "79123456789 and 79123456789"
	replacements := []pii.Replacement{
		{Start: 0, End: 11, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
		{Start: 16, End: 27, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}

	masked, mappings, err := ApplyReplacements(context.Background(), text, replacements)
	if err != nil {
		t.Fatalf("ApplyReplacements returned error: %v", err)
	}
	if masked != "<PHONE_1> and <PHONE_1>" {
		t.Fatalf("unexpected masked text: %q", masked)
	}
	if len(mappings) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(mappings))
	}
	if mappings[0].Start != 0 || mappings[1].Start != 16 {
		t.Fatalf("unexpected mapping spans: %+v", mappings)
	}
}

func TestApplyReplacementsAdjacentSpans(t *testing.T) {
	text := "79123456789a@example.com"
	replacements := []pii.Replacement{
		{Start: 0, End: 11, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
		{Start: 11, End: 24, Token: "<EMAIL_1>", Original: "a@example.com", Kind: pii.PIIKindEmail, Source: pii.SourceML, Confidence: 0.9},
	}

	masked, _, err := ApplyReplacements(context.Background(), text, replacements)
	if err != nil {
		t.Fatalf("ApplyReplacements returned error: %v", err)
	}
	if masked != "<PHONE_1><EMAIL_1>" {
		t.Fatalf("unexpected masked text: %q", masked)
	}
}

func TestApplyReplacementsUnsortedInput(t *testing.T) {
	text := "call 79123456789 or a@example.com"
	replacements := []pii.Replacement{
		{Start: 20, End: 33, Token: "<EMAIL_1>", Original: "a@example.com", Kind: pii.PIIKindEmail, Source: pii.SourceML, Confidence: 0.9},
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}

	masked, _, err := ApplyReplacements(context.Background(), text, replacements)
	if err != nil {
		t.Fatalf("ApplyReplacements returned error: %v", err)
	}
	if masked != "call <PHONE_1> or <EMAIL_1>" {
		t.Fatalf("unexpected masked text: %q", masked)
	}
}

func TestApplyReplacementsOverlap(t *testing.T) {
	text := "call 79123456789"
	replacements := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
		{Start: 10, End: 16, Token: "<PHONE_2>", Original: "56789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}

	_, _, err := ApplyReplacements(context.Background(), text, replacements)
	if !errors.Is(err, pii.ErrInvalidSpan) {
		t.Fatalf("expected ErrInvalidSpan for overlap, got %v", err)
	}
}

func TestApplyReplacementsInvalidSpan(t *testing.T) {
	text := "call 79123456789"
	replacements := []pii.Replacement{
		{Start: -1, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}

	_, _, err := ApplyReplacements(context.Background(), text, replacements)
	if !errors.Is(err, pii.ErrInvalidSpan) {
		t.Fatalf("expected ErrInvalidSpan, got %v", err)
	}
}

func TestApplyReplacementsOriginalMismatch(t *testing.T) {
	text := "call 79123456789"
	replacements := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "wrong", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}

	_, _, err := ApplyReplacements(context.Background(), text, replacements)
	if !errors.Is(err, pii.ErrInvalidSpan) {
		t.Fatalf("expected ErrInvalidSpan for original mismatch, got %v", err)
	}
}

func TestApplyReplacementsEmptyToken(t *testing.T) {
	text := "call 79123456789"
	replacements := []pii.Replacement{
		{Start: 5, End: 16, Token: "", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}

	_, _, err := ApplyReplacements(context.Background(), text, replacements)
	if !errors.Is(err, pii.ErrInvalidSpan) {
		t.Fatalf("expected ErrInvalidSpan for empty token, got %v", err)
	}
}

func TestApplyReplacementsTokenConflictingOriginals(t *testing.T) {
	text := "79123456789 and 79234567890"
	replacements := []pii.Replacement{
		{Start: 0, End: 11, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
		{Start: 16, End: 27, Token: "<PHONE_1>", Original: "79234567890", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}

	_, _, err := ApplyReplacements(context.Background(), text, replacements)
	if !errors.Is(err, pii.ErrInvalidSpan) {
		t.Fatalf("expected ErrInvalidSpan for conflicting originals, got %v", err)
	}
}

func TestApplyReplacementsDoesNotMutateInput(t *testing.T) {
	text := "call 79123456789"
	replacements := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}

	original := make([]pii.Replacement, len(replacements))
	copy(original, replacements)

	if _, _, err := ApplyReplacements(context.Background(), text, replacements); err != nil {
		t.Fatalf("ApplyReplacements returned error: %v", err)
	}

	if len(replacements) != len(original) || replacements[0] != original[0] {
		t.Fatal("input slice was mutated")
	}
}

func TestApplyReplacementsCancelledContext(t *testing.T) {
	text := "call 79123456789"
	replacements := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := ApplyReplacements(ctx, text, replacements)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestApplyReplacementsEmptyPlan(t *testing.T) {
	text := "call 79123456789"

	masked, mappings, err := ApplyReplacements(context.Background(), text, nil)
	if err != nil {
		t.Fatalf("ApplyReplacements returned error: %v", err)
	}
	if masked != text {
		t.Fatalf("expected original text, got %q", masked)
	}
	if len(mappings) != 0 {
		t.Fatalf("expected empty mappings, got %d", len(mappings))
	}
}
