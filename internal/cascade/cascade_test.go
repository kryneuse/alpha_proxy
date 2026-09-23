package cascade

import (
	"context"
	"errors"
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/entity"
)

// fakeEngine is a rule engine that returns a fixed set of entities.
type fakeEngine struct {
	entities []entity.Entity
}

func (f *fakeEngine) Analyze(text string) []entity.Entity {
	return f.entities
}

// fakeExpensive is an expensive extractor with a fixed set of entities.
type fakeExpensive struct {
	entities []entity.Entity
	err      error
	calls    int
	lastOrig string
	lastGate string
}

func (f *fakeExpensive) Detect(ctx context.Context, original, gate string) ([]entity.Entity, error) {
	f.calls++
	f.lastOrig = original
	f.lastGate = gate
	if f.err != nil {
		return nil, f.err
	}
	return f.entities, nil
}

// TestAnalyzeRules asserts rules run on the whole text and residual is
// byte-preserving masked.
func TestAnalyzeRules(t *testing.T) {
	engine := &fakeEngine{entities: []entity.Entity{
		{Type: entity.FULL_NAME, Text: "Иван Петров", Start: 8, End: 19},
	}}
	expensive := &fakeExpensive{}
	c := New(engine, expensive)
	text := "Клиент Иван Петров, номер документа 4510 123456"
	ruleEntities, residualText := c.AnalyzeRules(text)
	if len(ruleEntities) != 1 {
		t.Fatalf("expected 1 rule entity, got %d", len(ruleEntities))
	}
	if len(residualText) != len(text) {
		t.Errorf("residual length %d != original %d", len(residualText), len(text))
	}
	for i := 8; i < 19; i++ {
		if residualText[i] != ' ' {
			t.Errorf("expected space at byte %d, got %q", i, residualText[i])
		}
	}
}

// TestDetectChunk asserts the ML extractor receives the (original, gate) pair.
func TestDetectChunk(t *testing.T) {
	engine := &fakeEngine{}
	expensive := &fakeExpensive{entities: []entity.Entity{
		{Type: entity.PHONE, Text: "+7 (912) 345-67-89", Start: 0, End: 18},
	}}
	c := New(engine, expensive)
	entities, err := c.DetectChunk(context.Background(), "call +7 (912) 345-67-89", "call                 ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}
	if expensive.lastOrig != "call +7 (912) 345-67-89" {
		t.Errorf("unexpected original %q", expensive.lastOrig)
	}
	if expensive.lastGate != "call                 " {
		t.Errorf("unexpected gate %q", expensive.lastGate)
	}
}

// TestDetectChunkNilExpensive asserts fail-closed when expensive is nil.
func TestDetectChunkNilExpensive(t *testing.T) {
	engine := &fakeEngine{}
	c := New(engine, nil)
	_, err := c.DetectChunk(context.Background(), "text", "text")
	if err == nil {
		t.Error("expected error when expensive extractor is nil")
	}
}

// TestDetectChunkCancelled asserts a cancelled context stops the cascade.
func TestDetectChunkCancelled(t *testing.T) {
	engine := &fakeEngine{}
	expensive := &fakeExpensive{}
	c := New(engine, expensive)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.DetectChunk(ctx, "text", "text")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// TestDetectChunkErrorPropagates asserts an extractor error is returned.
func TestDetectChunkErrorPropagates(t *testing.T) {
	engine := &fakeEngine{}
	expensive := &fakeExpensive{err: errors.New("ml failed")}
	c := New(engine, expensive)
	_, err := c.DetectChunk(context.Background(), "text", "text")
	if err == nil || err.Error() != "ml failed" {
		t.Fatalf("expected ml error, got %v", err)
	}
}
