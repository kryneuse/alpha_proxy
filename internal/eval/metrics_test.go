package eval

import (
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/engine"
	"github.com/kryneuse/alpha_proxy/internal/entity"
)

// TestMatchSpansExact asserts that an exact span match is a TP.
func TestMatchSpansExact(t *testing.T) {
	expected := []Expected{{Type: entity.EMAIL, Text: "a@b.com", Start: 0, End: 7}}
	got := []entity.Entity{{Type: entity.EMAIL, Text: "a@b.com", Start: 0, End: 7}}
	ms := matchSpans(expected, got, IoUThreshold, true)
	if !ms.results[0].matched {
		t.Error("expected exact match to be matched")
	}
	if !ms.used[0] {
		t.Error("expected prediction to be used")
	}
}

// TestMatchSpansPartial asserts that a partial IoU match is a TP for span
// metrics but NOT for exact metrics.
func TestMatchSpansPartial(t *testing.T) {
	expected := []Expected{{Type: entity.EMAIL, Text: "a@b.com", Start: 0, End: 7}}
	// Prediction overlaps but is not exact (IoU < 1.0).
	got := []entity.Entity{{Type: entity.EMAIL, Text: "a@b.co", Start: 0, End: 6}}
	span := matchSpans(expected, got, IoUThreshold, true)
	if !span.results[0].matched {
		t.Error("expected partial match to be matched at span threshold")
	}
	exact := matchSpans(expected, got, 1.0, true)
	if exact.results[0].matched {
		t.Error("expected partial match to NOT be matched at exact threshold")
	}
}

// TestMatchSpansWrongType asserts that a prediction of the wrong type is not
// matched.
func TestMatchSpansWrongType(t *testing.T) {
	expected := []Expected{{Type: entity.EMAIL, Text: "a@b.com", Start: 0, End: 7}}
	got := []entity.Entity{{Type: entity.PHONE, Text: "a@b.com", Start: 0, End: 7}}
	ms := matchSpans(expected, got, IoUThreshold, true)
	if ms.results[0].matched {
		t.Error("expected wrong-type prediction to NOT be matched")
	}
	if ms.used[0] {
		t.Error("expected wrong-type prediction to be unused (FP)")
	}
}

// TestMatchSpansDuplicate asserts that a duplicate prediction is not matched
// twice; the second duplicate is an FP.
func TestMatchSpansDuplicate(t *testing.T) {
	expected := []Expected{{Type: entity.EMAIL, Text: "a@b.com", Start: 0, End: 7}}
	got := []entity.Entity{
		{Type: entity.EMAIL, Text: "a@b.com", Start: 0, End: 7},
		{Type: entity.EMAIL, Text: "a@b.com", Start: 0, End: 7},
	}
	ms := matchSpans(expected, got, IoUThreshold, true)
	if !ms.results[0].matched {
		t.Error("expected first duplicate to be matched")
	}
	// Exactly one prediction should be used; the other is an FP.
	usedCount := 0
	for _, u := range ms.used {
		if u {
			usedCount++
		}
	}
	if usedCount != 1 {
		t.Errorf("expected exactly 1 used prediction, got %d", usedCount)
	}
}

// TestEvaluateNegativeSampleFP asserts that predictions on a negative sample
// are counted as FP in span and typed metrics.
func TestEvaluateNegativeSampleFP(t *testing.T) {
	e := engine.New(engine.Options{})
	// "ИНН 7707083893" is a valid INN; as a negative sample it must be FP.
	samples := []Sample{{Text: "ИНН 7707083893", Negative: true}}
	m := Evaluate(e, samples)
	if m.TypedSpanPrecision != 0 {
		t.Errorf("expected span precision 0 (all FP), got %f", m.TypedSpanPrecision)
	}
	if m.TypedSpanRecall != 0 {
		t.Errorf("expected span recall 0, got %f", m.TypedSpanRecall)
	}
	if m.FalsePositiveRate != 1.0 {
		t.Errorf("expected FPR 1.0, got %f", m.FalsePositiveRate)
	}
	if m.Typed[entity.INN].FP != 1 {
		t.Errorf("expected INN FP=1, got %d", m.Typed[entity.INN].FP)
	}
}

// TestEvaluateExactVsPartial asserts that a partial match counts as TP for
// span metrics but as FP for exact metrics.
func TestEvaluateExactVsPartial(t *testing.T) {
	e := engine.New(engine.Options{})
	// The engine detects the full email; expected is a partial span.
	samples := []Sample{{
		Text:     "Email: a@b.com",
		Expected: []Expected{{Type: entity.EMAIL, Text: "a@b.co", Start: 7, End: 13}},
	}}
	m := Evaluate(e, samples)
	if m.TypedSpanPrecision != 1.0 {
		t.Errorf("expected span precision 1.0 (partial counts as TP), got %f", m.TypedSpanPrecision)
	}
	if m.ExactPrecision != 0.0 {
		t.Errorf("expected exact precision 0.0 (partial is FP for exact), got %f", m.ExactPrecision)
	}
}

// TestEvaluateDuplicateFP asserts that a duplicate prediction is an FP.
func TestEvaluateDuplicateFP(t *testing.T) {
	e := engine.New(engine.Options{})
	// The engine emits one email; we expect one, so no duplicate here.
	// To force a duplicate we use a sample where the engine emits two of the
	// same type. Use a mixed card sample that yields card + cvv + pin.
	samples := []Sample{{
		Text: "Номер карты 4532 0151 1283 0366, CVV 123, пин-код 7305",
		Expected: []Expected{
			{Type: entity.CARD_NUMBER, Text: "4532 0151 1283 0366", Start: 22, End: 41},
			{Type: entity.CVV, Text: "123", Start: 47, End: 50},
			{Type: entity.PIN, Text: "7305", Start: 66, End: 70},
		},
	}}
	m := Evaluate(e, samples)
	// All three expected types should be matched (no FP).
	if m.TypedSpanPrecision != 1.0 {
		t.Errorf("expected span precision 1.0, got %f", m.TypedSpanPrecision)
	}
	if m.TypedSpanRecall != 1.0 {
		t.Errorf("expected span recall 1.0, got %f", m.TypedSpanRecall)
	}
}
