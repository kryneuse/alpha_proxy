package tokenizer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

func mergeTestPolicy() pii.Policy {
	return pii.Policy{
		AllowedKinds: map[pii.PIIKind]bool{
			pii.PIIKindPhone:     true,
			pii.PIIKindEmail:     true,
			pii.PIIKindFullName:  true,
			pii.PIIKindFirstName: true,
			pii.PIIKindLastName:  true,
			pii.PIIKindAddress:   true,
			pii.PIIKindCity:      true,
		},
		MinConfidence: 0.5,
	}
}

func TestCombinePlansNonOverlapping(t *testing.T) {
	text := "call 79123456789 or a@example.com"
	mlPlan := []pii.Replacement{
		{Start: 20, End: 33, Token: "<EMAIL_1>", Original: "a@example.com", Kind: pii.PIIKindEmail, Source: pii.SourceML, Confidence: 0.9},
	}
	backendPlan := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}

	plan, err := CombinePlans(context.Background(), text, mlPlan, backendPlan, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}
	if len(plan) != 2 {
		t.Fatalf("expected 2 replacements, got %d", len(plan))
	}
	if plan[0].Token != "<PHONE_1>" || plan[1].Token != "<EMAIL_1>" {
		t.Fatalf("unexpected order: %+v", plan)
	}
}

func TestCombinePlansDoesNotMutateInputs(t *testing.T) {
	text := "call 79123456789"
	mlPlan := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceML, Confidence: 0.9},
	}
	backendPlan := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}

	mlCopy := make([]pii.Replacement, len(mlPlan))
	copy(mlCopy, mlPlan)
	backendCopy := make([]pii.Replacement, len(backendPlan))
	copy(backendCopy, backendPlan)

	if _, err := CombinePlans(context.Background(), text, mlPlan, backendPlan, mergeTestPolicy()); err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}

	if len(mlPlan) != len(mlCopy) || mlPlan[0] != mlCopy[0] {
		t.Fatal("mlPlan was mutated")
	}
	if len(backendPlan) != len(backendCopy) || backendPlan[0] != backendCopy[0] {
		t.Fatal("backendPlan was mutated")
	}
}

func TestCombinePlansPhoneDuplicatePrefersRegex(t *testing.T) {
	text := "call 79123456789"
	mlPlan := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceML, Confidence: 0.9},
	}
	backendPlan := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.8},
	}

	plan, err := CombinePlans(context.Background(), text, mlPlan, backendPlan, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}
	if len(plan) != 1 {
		t.Fatalf("expected 1 replacement, got %d", len(plan))
	}
	if plan[0].Source != pii.SourceReg {
		t.Fatalf("expected regex source for phone, got %q", plan[0].Source)
	}
}

func TestCombinePlansFullNameDuplicatePrefersML(t *testing.T) {
	text := "Иванов Иван Иванович"
	mlPlan := []pii.Replacement{
		{Start: 0, End: 38, Token: "<FULL_NAME_1>", Original: "Иванов Иван Иванович", Kind: pii.PIIKindFullName, Source: pii.SourceML, Confidence: 0.9},
	}
	backendPlan := []pii.Replacement{
		{Start: 0, End: 38, Token: "<FULL_NAME_1>", Original: "Иванов Иван Иванович", Kind: pii.PIIKindFullName, Source: pii.SourceReg, Confidence: 0.9},
	}

	plan, err := CombinePlans(context.Background(), text, mlPlan, backendPlan, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}
	if len(plan) != 1 {
		t.Fatalf("expected 1 replacement, got %d", len(plan))
	}
	if plan[0].Source != pii.SourceML {
		t.Fatalf("expected ml source for full name, got %q", plan[0].Source)
	}
}

func TestCombinePlansPrefersHigherConfidence(t *testing.T) {
	text := "call 79123456789"
	mlPlan := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.7},
	}
	backendPlan := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.95},
	}

	plan, err := CombinePlans(context.Background(), text, mlPlan, backendPlan, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}
	if len(plan) != 1 {
		t.Fatalf("expected 1 replacement, got %d", len(plan))
	}
	if plan[0].Confidence != 0.95 {
		t.Fatalf("expected higher confidence 0.95, got %v", plan[0].Confidence)
	}
}

func TestCombinePlansPolicyFilter(t *testing.T) {
	text := "call 79123456789"
	mlPlan := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceML, Confidence: 0.9},
	}
	backendPlan := []pii.Replacement{
		{Start: 5, End: 16, Token: "<INN_1>", Original: "79123456789", Kind: pii.PIIKindINN, Source: pii.SourceReg, Confidence: 0.9},
	}

	plan, err := CombinePlans(context.Background(), text, mlPlan, backendPlan, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}
	if len(plan) != 1 {
		t.Fatalf("expected 1 replacement after policy filter, got %d", len(plan))
	}
	if plan[0].Kind != pii.PIIKindPhone {
		t.Fatalf("expected phone replacement, got %+v", plan[0])
	}
}

func TestCombinePlansInvalidSpan(t *testing.T) {
	text := "call 79123456789"
	mlPlan := []pii.Replacement{
		{Start: -1, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceML, Confidence: 0.9},
	}

	_, err := CombinePlans(context.Background(), text, mlPlan, nil, mergeTestPolicy())
	if !errors.Is(err, pii.ErrInvalidSpan) {
		t.Fatalf("expected ErrInvalidSpan, got %v", err)
	}
}

func TestCombinePlansOriginalMismatch(t *testing.T) {
	text := "call 79123456789"
	mlPlan := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "wrong-original", Kind: pii.PIIKindPhone, Source: pii.SourceML, Confidence: 0.9},
	}

	_, err := CombinePlans(context.Background(), text, mlPlan, nil, mergeTestPolicy())
	if !errors.Is(err, pii.ErrInvalidSpan) {
		t.Fatalf("expected ErrInvalidSpan for original mismatch, got %v", err)
	}
}

func TestCombinePlansCancelledContext(t *testing.T) {
	text := "call 79123456789"
	mlPlan := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceML, Confidence: 0.9},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := CombinePlans(ctx, text, mlPlan, nil, mergeTestPolicy())
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestCombinePlansSameSpanDifferentKinds(t *testing.T) {
	text := "Иванов Иван Иванович"
	mlPlan := []pii.Replacement{
		{Start: 0, End: 38, Token: "<FULL_NAME_1>", Original: "Иванов Иван Иванович", Kind: pii.PIIKindFullName, Source: pii.SourceML, Confidence: 0.9},
	}
	backendPlan := []pii.Replacement{
		{Start: 0, End: 38, Token: "<FULL_NAME_1>", Original: "Иванов Иван Иванович", Kind: pii.PIIKindFirstName, Source: pii.SourceML, Confidence: 0.9},
	}

	plan, err := CombinePlans(context.Background(), text, mlPlan, backendPlan, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}
	if len(plan) != 1 {
		t.Fatalf("expected 1 replacement, got %d", len(plan))
	}
	if plan[0].Kind != pii.PIIKindFullName {
		t.Fatalf("expected full name to win over first name, got %+v", plan[0])
	}
}

func TestCombinePlansPartialOverlap(t *testing.T) {
	text := "Иванов Иван Иванович Петров"
	mlPlan := []pii.Replacement{
		{Start: 0, End: 38, Token: "<FULL_NAME_1>", Original: "Иванов Иван Иванович", Kind: pii.PIIKindFullName, Source: pii.SourceML, Confidence: 0.9},
	}
	backendPlan := []pii.Replacement{
		{Start: 13, End: 51, Token: "<FULL_NAME_2>", Original: "Иван Иванович Петров", Kind: pii.PIIKindFullName, Source: pii.SourceML, Confidence: 0.9},
	}

	plan, err := CombinePlans(context.Background(), text, mlPlan, backendPlan, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}
	if len(plan) != 1 {
		t.Fatalf("expected 1 replacement for partially overlapping spans, got %d", len(plan))
	}
}

func TestCombinePlansNestedSpans(t *testing.T) {
	text := "Иванов Иван Иванович"
	mlPlan := []pii.Replacement{
		{Start: 0, End: 38, Token: "<FULL_NAME_1>", Original: "Иванов Иван Иванович", Kind: pii.PIIKindFullName, Source: pii.SourceML, Confidence: 0.9},
	}
	backendPlan := []pii.Replacement{
		{Start: 13, End: 21, Token: "<FIRST_NAME_1>", Original: "Иван", Kind: pii.PIIKindFirstName, Source: pii.SourceML, Confidence: 0.9},
	}

	plan, err := CombinePlans(context.Background(), text, mlPlan, backendPlan, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}
	if len(plan) != 1 {
		t.Fatalf("expected 1 replacement, got %d", len(plan))
	}
	if plan[0].Kind != pii.PIIKindFullName {
		t.Fatalf("expected full name to win over nested first name, got %+v", plan[0])
	}
}

func TestCombinePlansFullNameVsNestedFirstName(t *testing.T) {
	text := "Иванов Иван Иванович"
	mlPlan := []pii.Replacement{
		{Start: 0, End: 38, Token: "<FULL_NAME_1>", Original: "Иванов Иван Иванович", Kind: pii.PIIKindFullName, Source: pii.SourceML, Confidence: 0.9},
	}
	backendPlan := []pii.Replacement{
		{Start: 13, End: 21, Token: "<FIRST_NAME_1>", Original: "Иван", Kind: pii.PIIKindFirstName, Source: pii.SourceML, Confidence: 0.95},
	}

	plan, err := CombinePlans(context.Background(), text, mlPlan, backendPlan, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}
	if len(plan) != 1 {
		t.Fatalf("expected 1 replacement, got %d", len(plan))
	}
	if plan[0].Kind != pii.PIIKindFullName {
		t.Fatalf("expected full name to win over nested first name despite higher confidence, got %+v", plan[0])
	}
}

func TestCombinePlansAddressVsNestedCity(t *testing.T) {
	text := "Москва, ул. Тверская"
	mlPlan := []pii.Replacement{
		{Start: 0, End: 36, Token: "<ADDRESS_1>", Original: "Москва, ул. Тверская", Kind: pii.PIIKindAddress, Source: pii.SourceML, Confidence: 0.9},
	}
	backendPlan := []pii.Replacement{
		{Start: 0, End: 12, Token: "<CITY_1>", Original: "Москва", Kind: pii.PIIKindCity, Source: pii.SourceML, Confidence: 0.95},
	}

	plan, err := CombinePlans(context.Background(), text, mlPlan, backendPlan, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}
	if len(plan) != 1 {
		t.Fatalf("expected 1 replacement, got %d", len(plan))
	}
	if plan[0].Kind != pii.PIIKindAddress {
		t.Fatalf("expected address to win over nested city, got %+v", plan[0])
	}
}

func TestCombinePlansPrimaryOwnerSameTypePriority(t *testing.T) {
	text := "call 79123456789"
	mlPlan := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceML, Confidence: 0.9},
	}
	backendPlan := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}

	plan, err := CombinePlans(context.Background(), text, mlPlan, backendPlan, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}
	if len(plan) != 1 {
		t.Fatalf("expected 1 replacement, got %d", len(plan))
	}
	if plan[0].Source != pii.SourceReg {
		t.Fatalf("expected regex primary owner for phone, got %q", plan[0].Source)
	}
}

func TestCombinePlansHigherConfidenceWins(t *testing.T) {
	text := "Иванов Иван Иванович"
	mlPlan := []pii.Replacement{
		{Start: 0, End: 38, Token: "<FULL_NAME_1>", Original: "Иванов Иван Иванович", Kind: pii.PIIKindFullName, Source: pii.SourceML, Confidence: 0.7},
	}
	backendPlan := []pii.Replacement{
		{Start: 0, End: 38, Token: "<FULL_NAME_1>", Original: "Иванов Иван Иванович", Kind: pii.PIIKindFullName, Source: pii.SourceML, Confidence: 0.95},
	}

	plan, err := CombinePlans(context.Background(), text, mlPlan, backendPlan, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}
	if len(plan) != 1 {
		t.Fatalf("expected 1 replacement, got %d", len(plan))
	}
	if plan[0].Confidence != 0.95 {
		t.Fatalf("expected higher confidence 0.95, got %v", plan[0].Confidence)
	}
}

func TestCombinePlansLongerSpanWins(t *testing.T) {
	text := "Иванов Иван Иванович Петров"
	mlPlan := []pii.Replacement{
		{Start: 0, End: 38, Token: "<FULL_NAME_1>", Original: "Иванов Иван Иванович", Kind: pii.PIIKindFullName, Source: pii.SourceML, Confidence: 0.9},
	}
	backendPlan := []pii.Replacement{
		{Start: 0, End: 51, Token: "<FULL_NAME_2>", Original: "Иванов Иван Иванович Петров", Kind: pii.PIIKindFullName, Source: pii.SourceML, Confidence: 0.9},
	}

	plan, err := CombinePlans(context.Background(), text, mlPlan, backendPlan, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}
	if len(plan) != 1 {
		t.Fatalf("expected 1 replacement, got %d", len(plan))
	}
	if plan[0].End != 51 {
		t.Fatalf("expected longer span [0:51] to win, got %+v", plan[0])
	}
}

func TestCombinePlansAdjacentNonOverlapping(t *testing.T) {
	text := "call 79123456789 a@example.com"
	mlPlan := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}
	backendPlan := []pii.Replacement{
		{Start: 17, End: 30, Token: "<EMAIL_1>", Original: "a@example.com", Kind: pii.PIIKindEmail, Source: pii.SourceML, Confidence: 0.9},
	}

	plan, err := CombinePlans(context.Background(), text, mlPlan, backendPlan, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}
	if len(plan) != 2 {
		t.Fatalf("expected 2 replacements, got %d", len(plan))
	}
}

func TestCombinePlansReversedInputSameResult(t *testing.T) {
	text := "Иванов Иван Иванович"
	planA := []pii.Replacement{
		{Start: 0, End: 38, Token: "<FULL_NAME_1>", Original: "Иванов Иван Иванович", Kind: pii.PIIKindFullName, Source: pii.SourceML, Confidence: 0.9},
	}
	planB := []pii.Replacement{
		{Start: 13, End: 21, Token: "<FIRST_NAME_1>", Original: "Иван", Kind: pii.PIIKindFirstName, Source: pii.SourceML, Confidence: 0.9},
	}

	res1, err := CombinePlans(context.Background(), text, planA, planB, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}
	res2, err := CombinePlans(context.Background(), text, planB, planA, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}

	if len(res1) != len(res2) {
		t.Fatalf("expected same length, got %d and %d", len(res1), len(res2))
	}
	for i := range res1 {
		if res1[i] != res2[i] {
			t.Fatalf("expected same result, got %+v and %+v", res1[i], res2[i])
		}
	}
}

type cancelAfterCalls struct {
	remaining int
}

func (c *cancelAfterCalls) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterCalls) Done() <-chan struct{}       { return nil }
func (c *cancelAfterCalls) Value(any) any               { return nil }
func (c *cancelAfterCalls) Err() error {
	if c.remaining > 0 {
		c.remaining--
		return nil
	}
	return context.Canceled
}

func TestCombinePlansCancelledContextDuringOverlapResolution(t *testing.T) {
	text := "Иванов Иван Иванович Петров Сидоров"
	// Several valid overlapping full-name spans.
	replacements := []pii.Replacement{
		{Start: 0, End: 66, Token: "<FULL_NAME_1>", Original: text[0:66], Kind: pii.PIIKindFullName, Source: pii.SourceML, Confidence: 0.9},
		{Start: 13, End: 66, Token: "<FULL_NAME_2>", Original: text[13:66], Kind: pii.PIIKindFullName, Source: pii.SourceML, Confidence: 0.9},
		{Start: 0, End: 51, Token: "<FULL_NAME_3>", Original: text[0:51], Kind: pii.PIIKindFullName, Source: pii.SourceML, Confidence: 0.9},
		{Start: 13, End: 51, Token: "<FULL_NAME_4>", Original: text[13:51], Kind: pii.PIIKindFullName, Source: pii.SourceML, Confidence: 0.9},
	}

	// Allow the first candidate to be processed, then cancel during the
	// next candidate pass or the selected-overlap check.
	ctx := &cancelAfterCalls{remaining: 2}

	_, err := resolveOverlaps(ctx, replacements)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestCombinePlansTokenCollisionResolved(t *testing.T) {
	text := "call 79123456789 and 79234567890"
	mlPlan := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}
	backendPlan := []pii.Replacement{
		{Start: 21, End: 32, Token: "<PHONE_1>", Original: "79234567890", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}

	plan, err := CombinePlans(context.Background(), text, mlPlan, backendPlan, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}
	if len(plan) != 2 {
		t.Fatalf("expected 2 replacements, got %d", len(plan))
	}
	if plan[0].Token == plan[1].Token {
		t.Fatalf("expected distinct tokens for different values, got %q", plan[0].Token)
	}
	if plan[0].Token != "<PHONE_1>" || plan[1].Token != "<PHONE_2>" {
		t.Fatalf("expected <PHONE_1> and <PHONE_2>, got %q and %q", plan[0].Token, plan[1].Token)
	}
}

func TestCombinePlansSamePhoneSameToken(t *testing.T) {
	text := "79123456789 and 79123456789"
	mlPlan := []pii.Replacement{
		{Start: 0, End: 11, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}
	backendPlan := []pii.Replacement{
		{Start: 16, End: 27, Token: "<PHONE_2>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}

	plan, err := CombinePlans(context.Background(), text, mlPlan, backendPlan, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}
	if len(plan) != 2 {
		t.Fatalf("expected 2 replacements, got %d", len(plan))
	}
	if plan[0].Token != plan[1].Token {
		t.Fatalf("expected same token for same value, got %q and %q", plan[0].Token, plan[1].Token)
	}
}

func TestCombinePlansSeparateNumberingPerKind(t *testing.T) {
	text := "call 79123456789 or a@example.com"
	mlPlan := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}
	backendPlan := []pii.Replacement{
		{Start: 20, End: 33, Token: "<EMAIL_1>", Original: "a@example.com", Kind: pii.PIIKindEmail, Source: pii.SourceML, Confidence: 0.9},
	}

	plan, err := CombinePlans(context.Background(), text, mlPlan, backendPlan, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}
	if len(plan) != 2 {
		t.Fatalf("expected 2 replacements, got %d", len(plan))
	}
	if plan[0].Token != "<PHONE_1>" {
		t.Fatalf("expected <PHONE_1>, got %q", plan[0].Token)
	}
	if plan[1].Token != "<EMAIL_1>" {
		t.Fatalf("expected <EMAIL_1>, got %q", plan[1].Token)
	}
}

func TestCombinePlansSkipsTokenAlreadyInText(t *testing.T) {
	text := "contact <PHONE_1> 79123456789"
	mlPlan := []pii.Replacement{
		{Start: 18, End: 29, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}

	plan, err := CombinePlans(context.Background(), text, mlPlan, nil, mergeTestPolicy())
	if err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}
	if len(plan) != 1 {
		t.Fatalf("expected 1 replacement, got %d", len(plan))
	}
	if plan[0].Token != "<PHONE_2>" {
		t.Fatalf("expected <PHONE_2> (skipping existing <PHONE_1>), got %q", plan[0].Token)
	}
}

func TestCombinePlansDoesNotMutateInputsOnRenumber(t *testing.T) {
	text := "call 79123456789 and 79234567890"
	mlPlan := []pii.Replacement{
		{Start: 5, End: 16, Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}
	backendPlan := []pii.Replacement{
		{Start: 21, End: 32, Token: "<PHONE_1>", Original: "79234567890", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Confidence: 0.9},
	}

	mlCopy := make([]pii.Replacement, len(mlPlan))
	copy(mlCopy, mlPlan)
	backendCopy := make([]pii.Replacement, len(backendPlan))
	copy(backendCopy, backendPlan)

	if _, err := CombinePlans(context.Background(), text, mlPlan, backendPlan, mergeTestPolicy()); err != nil {
		t.Fatalf("CombinePlans returned error: %v", err)
	}

	if len(mlPlan) != len(mlCopy) || mlPlan[0] != mlCopy[0] {
		t.Fatal("mlPlan was mutated during token renumbering")
	}
	if len(backendPlan) != len(backendCopy) || backendPlan[0] != backendCopy[0] {
		t.Fatal("backendPlan was mutated during token renumbering")
	}
}
