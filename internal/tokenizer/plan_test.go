package tokenizer

import (
	"context"
	"errors"
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

func testPolicy() pii.Policy {
	return pii.Policy{
		AllowedKinds: map[pii.PIIKind]bool{
			pii.PIIKindPhone:    true,
			pii.PIIKindEmail:    true,
			pii.PIIKindFullName: true,
		},
		MinConfidence: 0.5,
	}
}

func TestBuildReplacementPlanBasic(t *testing.T) {
	text := "call 79123456789 or a@example.com"
	entities := []pii.Entity{
		{Kind: pii.PIIKindPhone, Start: 5, End: 16, Confidence: 0.9, Source: pii.SourceReg},
		{Kind: pii.PIIKindEmail, Start: 20, End: 33, Confidence: 0.9, Source: pii.SourceML},
	}

	plan, err := BuildReplacementPlan(context.Background(), text, entities, testPolicy())
	if err != nil {
		t.Fatalf("BuildReplacementPlan returned error: %v", err)
	}
	if len(plan) != 2 {
		t.Fatalf("expected 2 replacements, got %d", len(plan))
	}
	if plan[0].Token != "<PHONE_1>" || plan[0].Original != "79123456789" {
		t.Fatalf("unexpected phone replacement: %+v", plan[0])
	}
	if plan[1].Token != "<EMAIL_1>" || plan[1].Original != "a@example.com" {
		t.Fatalf("unexpected email replacement: %+v", plan[1])
	}
}

func TestFilterByAllowedKinds(t *testing.T) {
	text := "call 79123456789"
	entities := []pii.Entity{
		{Kind: pii.PIIKindPhone, Start: 5, End: 16, Confidence: 0.9},
		{Kind: pii.PIIKindINN, Start: 5, End: 16, Confidence: 0.9},
	}

	plan, err := BuildReplacementPlan(context.Background(), text, entities, testPolicy())
	if err != nil {
		t.Fatalf("BuildReplacementPlan returned error: %v", err)
	}
	if len(plan) != 1 {
		t.Fatalf("expected 1 replacement after kind filter, got %d", len(plan))
	}
	if plan[0].Kind != pii.PIIKindPhone {
		t.Fatalf("expected phone replacement, got %+v", plan[0])
	}
}

func TestFilterByConfidence(t *testing.T) {
	text := "call 79123456789"
	entities := []pii.Entity{
		{Kind: pii.PIIKindPhone, Start: 5, End: 16, Confidence: 0.9},
		{Kind: pii.PIIKindEmail, Start: 5, End: 16, Confidence: 0.1},
	}

	plan, err := BuildReplacementPlan(context.Background(), text, entities, testPolicy())
	if err != nil {
		t.Fatalf("BuildReplacementPlan returned error: %v", err)
	}
	if len(plan) != 1 {
		t.Fatalf("expected 1 replacement after confidence filter, got %d", len(plan))
	}
	if plan[0].Kind != pii.PIIKindPhone {
		t.Fatalf("expected phone replacement, got %+v", plan[0])
	}
}

func TestInvalidNegativeSpan(t *testing.T) {
	text := "call 79123456789"
	entities := []pii.Entity{
		{Kind: pii.PIIKindPhone, Start: -1, End: 16, Confidence: 0.9},
	}

	_, err := BuildReplacementPlan(context.Background(), text, entities, testPolicy())
	if !errors.Is(err, pii.ErrInvalidSpan) {
		t.Fatalf("expected ErrInvalidSpan, got %v", err)
	}
}

func TestInvalidSpanBeyondText(t *testing.T) {
	text := "call 79123456789"
	entities := []pii.Entity{
		{Kind: pii.PIIKindPhone, Start: 5, End: 100, Confidence: 0.9},
	}

	_, err := BuildReplacementPlan(context.Background(), text, entities, testPolicy())
	if !errors.Is(err, pii.ErrInvalidSpan) {
		t.Fatalf("expected ErrInvalidSpan, got %v", err)
	}
}

func TestInvalidSpanStartNotLessThanEnd(t *testing.T) {
	text := "call 79123456789"
	entities := []pii.Entity{
		{Kind: pii.PIIKindPhone, Start: 10, End: 10, Confidence: 0.9},
	}

	_, err := BuildReplacementPlan(context.Background(), text, entities, testPolicy())
	if !errors.Is(err, pii.ErrInvalidSpan) {
		t.Fatalf("expected ErrInvalidSpan, got %v", err)
	}
}

func TestSpanCuttingCyrillic(t *testing.T) {
	text := "привет 79123456789"
	entities := []pii.Entity{
		{Kind: pii.PIIKindPhone, Start: 6, End: 7, Confidence: 0.9},
	}

	_, err := BuildReplacementPlan(context.Background(), text, entities, testPolicy())
	if !errors.Is(err, pii.ErrInvalidSpan) {
		t.Fatalf("expected ErrInvalidSpan for span cutting a UTF-8 char, got %v", err)
	}
}

func TestSpanCuttingEmoji(t *testing.T) {
	text := "hello 😀 79123456789"
	entities := []pii.Entity{
		{Kind: pii.PIIKindPhone, Start: 6, End: 7, Confidence: 0.9},
	}

	_, err := BuildReplacementPlan(context.Background(), text, entities, testPolicy())
	if !errors.Is(err, pii.ErrInvalidSpan) {
		t.Fatalf("expected ErrInvalidSpan for span cutting an emoji, got %v", err)
	}
}

func TestCorrectByteOffsetsForCyrillic(t *testing.T) {
	text := "звони 79123456789"
	entities := []pii.Entity{
		{Kind: pii.PIIKindPhone, Start: 11, End: 22, Confidence: 0.9},
	}

	plan, err := BuildReplacementPlan(context.Background(), text, entities, testPolicy())
	if err != nil {
		t.Fatalf("BuildReplacementPlan returned error: %v", err)
	}
	if len(plan) != 1 {
		t.Fatalf("expected 1 replacement, got %d", len(plan))
	}
	if plan[0].Original != "79123456789" {
		t.Fatalf("expected original 79123456789, got %q", plan[0].Original)
	}
}

func TestDeterministicNumberingByPosition(t *testing.T) {
	text := "a@example.com and 79123456789"

	// Unsorted input: phone comes first in the slice but email is earlier in text.
	unsorted := []pii.Entity{
		{Kind: pii.PIIKindPhone, Start: 18, End: 29, Confidence: 0.9},
		{Kind: pii.PIIKindEmail, Start: 0, End: 13, Confidence: 0.9},
	}

	plan, err := BuildReplacementPlan(context.Background(), text, unsorted, testPolicy())
	if err != nil {
		t.Fatalf("BuildReplacementPlan returned error: %v", err)
	}
	if len(plan) != 2 {
		t.Fatalf("expected 2 replacements, got %d", len(plan))
	}
	if plan[0].Token != "<EMAIL_1>" {
		t.Fatalf("expected <EMAIL_1> first by position, got %q", plan[0].Token)
	}
	if plan[1].Token != "<PHONE_1>" {
		t.Fatalf("expected <PHONE_1> second by position, got %q", plan[1].Token)
	}
}

func TestSameValueSameTokenAcrossPositions(t *testing.T) {
	text := "79123456789 and 79123456789"
	entities := []pii.Entity{
		{Kind: pii.PIIKindPhone, Start: 0, End: 11, Confidence: 0.9},
		{Kind: pii.PIIKindPhone, Start: 16, End: 27, Confidence: 0.9},
	}

	plan, err := BuildReplacementPlan(context.Background(), text, entities, testPolicy())
	if err != nil {
		t.Fatalf("BuildReplacementPlan returned error: %v", err)
	}
	if len(plan) != 2 {
		t.Fatalf("expected 2 replacements, got %d", len(plan))
	}
	if plan[0].Token != plan[1].Token {
		t.Fatalf("expected same token for same value, got %q and %q", plan[0].Token, plan[1].Token)
	}
}

func TestInvalidSpanForbiddenKindStillErrors(t *testing.T) {
	text := "call 79123456789"
	entities := []pii.Entity{
		{Kind: pii.PIIKindINN, Start: -1, End: 16, Confidence: 0.9},
	}

	_, err := BuildReplacementPlan(context.Background(), text, entities, testPolicy())
	if !errors.Is(err, pii.ErrInvalidSpan) {
		t.Fatalf("expected ErrInvalidSpan even for forbidden kind, got %v", err)
	}
}

func TestInvalidSpanLowConfidenceStillErrors(t *testing.T) {
	text := "call 79123456789"
	entities := []pii.Entity{
		{Kind: pii.PIIKindPhone, Start: -1, End: 16, Confidence: 0.1},
	}

	_, err := BuildReplacementPlan(context.Background(), text, entities, testPolicy())
	if !errors.Is(err, pii.ErrInvalidSpan) {
		t.Fatalf("expected ErrInvalidSpan even for low confidence, got %v", err)
	}
}

func TestCancelledContext(t *testing.T) {
	text := "call 79123456789"
	entities := []pii.Entity{
		{Kind: pii.PIIKindPhone, Start: 5, End: 16, Confidence: 0.9},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := BuildReplacementPlan(ctx, text, entities, testPolicy())
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}
