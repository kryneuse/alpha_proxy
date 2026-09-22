package gate

import (
	"testing"
)

func TestNeutralTextSafe(t *testing.T) {
	g := New(DefaultConfig())
	d := g.Evaluate("сегодня хорошая погода на улице")
	if d.Route != SAFE {
		t.Errorf("expected SAFE, got %s (score=%f)", d.Route, d.Score)
	}
}

func TestDocumentLikeNotSafe(t *testing.T) {
	g := New(DefaultConfig())
	d := g.Evaluate("4510 123456")
	if d.Route == SAFE {
		t.Errorf("expected not SAFE for document-like digits, got %s", d.Route)
	}
}

func TestWeakNameLikeUncertain(t *testing.T) {
	g := New(DefaultConfig())
	d := g.Evaluate("Иван Петров")
	// Name-like alone should be a weak signal -> UNCERTAIN (not SAFE, not LIKELY).
	if d.Route != UNCERTAIN {
		t.Errorf("expected UNCERTAIN for weak name-like, got %s (score=%f)", d.Route, d.Score)
	}
}

func TestMultipleWeakSignalsIncreaseScore(t *testing.T) {
	g := New(DefaultConfig())
	weak := g.Evaluate("Иван Петров")
	stronger := g.Evaluate("клиент Иван Петров")
	if stronger.Score <= weak.Score {
		t.Errorf("expected stronger score with context, got %f <= %f", stronger.Score, weak.Score)
	}
}

func TestStrongPIILikely(t *testing.T) {
	g := New(DefaultConfig())
	d := g.Evaluate("паспорт 4510 123456")
	if d.Route != LIKELY_PII {
		t.Errorf("expected LIKELY_PII for passport context + digits, got %s (score=%f)", d.Route, d.Score)
	}
}

func TestLowThresholdChange(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LowThreshold = 0.5
	g := New(cfg)
	// "Иван Петров" has score ~0.2 (name-like). With LowThreshold=0.5 it is SAFE.
	d := g.Evaluate("Иван Петров")
	if d.Route != SAFE {
		t.Errorf("expected SAFE with LowThreshold=0.5, got %s (score=%f)", d.Route, d.Score)
	}
}

func TestHighThresholdChange(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HighThreshold = 0.9
	g := New(cfg)
	// "серия 4510" has score ~0.5 (series_number 0.3 + security_code 0.2).
	// With HighThreshold=0.9 it is UNCERTAIN, not LIKELY_PII.
	d := g.Evaluate("серия 4510")
	if d.Route != UNCERTAIN {
		t.Errorf("expected UNCERTAIN with HighThreshold=0.9, got %s (score=%f)", d.Route, d.Score)
	}
}

func TestPublicContextDoesNotMaskStrongPII(t *testing.T) {
	g := New(DefaultConfig())
	// Strong PII case with public context should still be LIKELY_PII.
	d := g.Evaluate("паспорт 4510 123456, офис компании")
	if d.Route != LIKELY_PII {
		t.Errorf("expected LIKELY_PII despite public context, got %s (score=%f)", d.Route, d.Score)
	}
}

func TestSignalsExplainDecision(t *testing.T) {
	g := New(DefaultConfig())
	d := g.Evaluate("паспорт 4510 123456")
	if len(d.Signals) == 0 {
		t.Fatal("expected signals to be populated")
	}
	found := false
	for _, s := range d.Signals {
		if s.Name == "passport_like_digits" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected passport_like_digits signal, got %+v", d.Signals)
	}
}

func TestScoreRange(t *testing.T) {
	g := New(DefaultConfig())
	for _, text := range []string{"", "обычный текст", "паспорт 4510 123456", "клиент иван петров"} {
		d := g.Evaluate(text)
		if d.Score < 0 || d.Score > 1 {
			t.Errorf("score out of range for %q: %f", text, d.Score)
		}
	}
}
