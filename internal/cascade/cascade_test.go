package cascade

import (
	"context"
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/gate"
)

// fakeEngine is a rule engine that returns a fixed set of entities.
type fakeEngine struct {
	entities []entity.Entity
}

func (f *fakeEngine) Analyze(text string) []entity.Entity {
	return f.entities
}

// fakeCheap is a cheap classifier with a fixed score.
type fakeCheap struct {
	score float64
	calls int
}

func (f *fakeCheap) HasPII(ctx context.Context, text string) (float64, error) {
	f.calls++
	return f.score, nil
}

// fakeExpensive is an expensive extractor with a fixed set of entities.
type fakeExpensive struct {
	entities []entity.Entity
	calls    int
}

func (f *fakeExpensive) Detect(ctx context.Context, text string) ([]entity.Entity, error) {
	f.calls++
	return f.entities, nil
}

// TestRouteSafe asserts that SAFE does not invoke cheap or expensive.
func TestRouteSafe(t *testing.T) {
	engine := &fakeEngine{}
	cheap := &fakeCheap{}
	expensive := &fakeExpensive{}
	g := gate.New(gate.DefaultConfig())
	c := New(engine, g, cheap, expensive)
	res, err := c.Run(context.Background(), "сегодня хорошая погода")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Route != gate.SAFE {
		t.Errorf("expected SAFE, got %s", res.Route)
	}
	if cheap.calls != 0 {
		t.Errorf("expected cheap not invoked, got %d calls", cheap.calls)
	}
	if expensive.calls != 0 {
		t.Errorf("expected expensive not invoked, got %d calls", expensive.calls)
	}
}

// TestRouteUncertainCheapNegative asserts UNCERTAIN + cheap negative does not
// invoke expensive.
func TestRouteUncertainCheapNegative(t *testing.T) {
	engine := &fakeEngine{}
	cheap := &fakeCheap{score: 0.2}
	expensive := &fakeExpensive{}
	g := gate.New(gate.DefaultConfig())
	c := New(engine, g, cheap, expensive)
	res, err := c.Run(context.Background(), "Иван Петров")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Route != gate.UNCERTAIN {
		t.Errorf("expected UNCERTAIN, got %s", res.Route)
	}
	if cheap.calls != 1 {
		t.Errorf("expected cheap invoked once, got %d", cheap.calls)
	}
	if expensive.calls != 0 {
		t.Errorf("expected expensive not invoked, got %d", expensive.calls)
	}
}

// TestRouteUncertainCheapPositive asserts UNCERTAIN + cheap positive invokes
// expensive.
func TestRouteUncertainCheapPositive(t *testing.T) {
	engine := &fakeEngine{}
	cheap := &fakeCheap{score: 0.8}
	expensive := &fakeExpensive{entities: []entity.Entity{
		{Type: entity.PHONE, Text: "+7 (912) 345-67-89", Start: 0, End: 18},
	}}
	g := gate.New(gate.DefaultConfig())
	c := New(engine, g, cheap, expensive)
	res, err := c.Run(context.Background(), "Иван Петров")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cheap.calls != 1 {
		t.Errorf("expected cheap invoked once, got %d", cheap.calls)
	}
	if expensive.calls != 1 {
		t.Errorf("expected expensive invoked once, got %d", expensive.calls)
	}
	if len(res.Entities) != 1 {
		t.Errorf("expected 1 merged entity, got %d (%+v)", len(res.Entities), res.Entities)
	}
}

// TestRouteLikelyPII asserts LIKELY_PII skips cheap and invokes expensive.
func TestRouteLikelyPII(t *testing.T) {
	engine := &fakeEngine{}
	cheap := &fakeCheap{}
	expensive := &fakeExpensive{entities: []entity.Entity{
		{Type: entity.PASSPORT, Text: "4510 123456", Start: 8, End: 19},
	}}
	g := gate.New(gate.DefaultConfig())
	c := New(engine, g, cheap, expensive)
	res, err := c.Run(context.Background(), "паспорт 4510 123456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Route != gate.LIKELY_PII {
		t.Errorf("expected LIKELY_PII, got %s", res.Route)
	}
	if cheap.calls != 0 {
		t.Errorf("expected cheap NOT invoked, got %d", cheap.calls)
	}
	if expensive.calls != 1 {
		t.Errorf("expected expensive invoked once, got %d", expensive.calls)
	}
}

// TestRuleEntitiesPreserved asserts rule entities are in the final result.
func TestRuleEntitiesPreserved(t *testing.T) {
	engine := &fakeEngine{entities: []entity.Entity{
		{Type: entity.FULL_NAME, Text: "Иван Петров", Start: 0, End: 11},
	}}
	cheap := &fakeCheap{score: 0.8}
	expensive := &fakeExpensive{entities: []entity.Entity{
		{Type: entity.PHONE, Text: "+7 (912) 345-67-89", Start: 12, End: 30},
	}}
	g := gate.New(gate.DefaultConfig())
	c := New(engine, g, cheap, expensive)
	chunk := "Иван Петров +7 (912) 345-67-89"
	res, err := c.Run(context.Background(), chunk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, e := range res.Entities {
		if e.Type == entity.FULL_NAME && e.Text == "Иван Петров" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected rule entity preserved, got %+v", res.Entities)
	}
}

// TestResidualMasksRuleSpans asserts the residual masks rule spans with spaces
// while preserving byte length.
func TestResidualMasksRuleSpans(t *testing.T) {
	engine := &fakeEngine{entities: []entity.Entity{
		{Type: entity.FULL_NAME, Text: "Иван Петров", Start: 8, End: 19},
	}}
	cheap := &fakeCheap{score: 0.2}
	expensive := &fakeExpensive{}
	g := gate.New(gate.DefaultConfig())
	c := New(engine, g, cheap, expensive)
	chunk := "Клиент Иван Петров, номер документа 4510 123456"
	res, err := c.Run(context.Background(), chunk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Residual) != len(chunk) {
		t.Errorf("residual length %d != original %d", len(res.Residual), len(chunk))
	}
	for i := 8; i < 19; i++ {
		if res.Residual[i] != ' ' {
			t.Errorf("expected space at byte %d, got %q", i, res.Residual[i])
		}
	}
}

// TestResidualCyrillicOffsets asserts byte offsets are preserved on Cyrillic.
func TestResidualCyrillicOffsets(t *testing.T) {
	// "Иван" is 8 bytes (4 Cyrillic letters x 2 bytes).
	engine := &fakeEngine{entities: []entity.Entity{
		{Type: entity.FULL_NAME, Text: "Иван", Start: 0, End: 8},
	}}
	cheap := &fakeCheap{score: 0.2}
	expensive := &fakeExpensive{}
	g := gate.New(gate.DefaultConfig())
	c := New(engine, g, cheap, expensive)
	chunk := "Иван Петров"
	res, err := c.Run(context.Background(), chunk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Residual) != len(chunk) {
		t.Errorf("residual length %d != original %d", len(res.Residual), len(chunk))
	}
	for i := 0; i < 8; i++ {
		if res.Residual[i] != ' ' {
			t.Errorf("expected space at byte %d, got %q", i, res.Residual[i])
		}
	}
	// "Петров" starts at byte 9 (after 8 masked bytes + 1 space) and is 12 bytes.
	if res.Residual[9:21] != "Петров" {
		t.Errorf("expected 'Петров' preserved, got %q", res.Residual[9:21])
	}
}

// TestUncertainNilCheapRoutesToExpensive asserts that UNCERTAIN with nil cheap
// routes straight to the expensive extractor and does not return an error.
func TestUncertainNilCheapRoutesToExpensive(t *testing.T) {
	engine := &fakeEngine{}
	expensive := &fakeExpensive{}
	g := gate.New(gate.DefaultConfig())
	c := New(engine, g, nil, expensive)
	_, err := c.Run(context.Background(), "Иван Петров")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if expensive.calls != 1 {
		t.Fatalf("expected expensive extractor to be called once, got %d", expensive.calls)
	}
}

// TestFailClosedExpensiveNil asserts that LIKELY_PII with nil expensive fails
// closed.
func TestFailClosedExpensiveNil(t *testing.T) {
	engine := &fakeEngine{}
	cheap := &fakeCheap{}
	g := gate.New(gate.DefaultConfig())
	c := New(engine, g, cheap, nil)
	_, err := c.Run(context.Background(), "паспорт 4510 123456")
	if err == nil {
		t.Error("expected error when expensive extractor is nil on LIKELY_PII route")
	}
}
