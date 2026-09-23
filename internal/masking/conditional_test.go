package masking

import (
	"context"
	"strings"
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/cascade"
	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/ml"
	"github.com/kryneuse/alpha_proxy/internal/pii"
)

// multiEntityRunner returns a fixed set of entities for the whole chunk. It is
// used to exercise conditional masking through the real CascadeMasker path.
type multiEntityRunner struct {
	entities []entity.Entity
}

func (m *multiEntityRunner) Run(_ context.Context, text string) (cascade.Result, error) {
	return cascade.Result{Entities: m.entities}, nil
}

// conditionalPolicy returns a policy that allows all kinds and applies the
// default business rule: PIN requires CARD_NUMBER.
func conditionalPolicy() pii.Policy {
	return pii.Policy{
		AllowedKinds: map[pii.PIIKind]bool{
			pii.PIIKindPIN:      true,
			pii.PIIKindBankCard: true,
		},
		MinConfidence: 0.5,
		MaskConditions: map[pii.PIIKind]pii.MaskCondition{
			pii.PIIKindPIN: {RequiresAll: []pii.PIIKind{pii.PIIKindBankCard}},
		},
	}
}

func newConditionalMasker(t *testing.T, r runner) *CascadeMasker {
	t.Helper()
	cfg := ml.DefaultChunkConfig()
	m, err := NewCascadeMasker(r, cfg, 2)
	if err != nil {
		t.Fatalf("NewCascadeMasker returned error: %v", err)
	}
	return m
}

// span returns a byte-offset entity for the given substring.
func span(typ entity.Type, text, sub string, score float64, reason string) entity.Entity {
	start := strings.Index(text, sub)
	if start < 0 {
		panic("substring not found: " + sub)
	}
	return entity.Entity{Type: typ, Text: sub, Start: start, End: start + len(sub), Score: score, Reason: reason}
}

// CASE 1: PIN only -> not masked.
func TestConditionalMaskingPinOnly(t *testing.T) {
	text := "Пин-код 7305"
	r := &multiEntityRunner{entities: []entity.Entity{
		span(entity.PIN, text, "7305", 0.9, "regex"),
	}}
	m := newConditionalMasker(t, r)

	masked, mappings, err := m.Mask(context.Background(), text, conditionalPolicy())
	if err != nil {
		t.Fatalf("Mask returned error: %v", err)
	}
	if masked != text {
		t.Fatalf("expected PIN not masked, got %q", masked)
	}
	if len(mappings) != 0 {
		t.Fatalf("expected 0 mappings for unmasked PIN, got %d", len(mappings))
	}
}

// CASE 2: PIN + CARD_NUMBER -> both masked.
func TestConditionalMaskingPinAndCard(t *testing.T) {
	text := "Номер карты 4111 1111 1111 1111, пин-код 7305"
	r := &multiEntityRunner{entities: []entity.Entity{
		span(entity.CARD_NUMBER, text, "4111 1111 1111 1111", 0.9, "regex"),
		span(entity.PIN, text, "7305", 0.9, "regex"),
	}}
	m := newConditionalMasker(t, r)

	masked, mappings, err := m.Mask(context.Background(), text, conditionalPolicy())
	if err != nil {
		t.Fatalf("Mask returned error: %v", err)
	}
	if strings.Contains(masked, "4111 1111 1111 1111") {
		t.Fatalf("expected card number masked, got %q", masked)
	}
	if strings.Contains(masked, "7305") {
		t.Fatalf("expected PIN masked, got %q", masked)
	}
	if len(mappings) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(mappings))
	}
}

// CASE 3: CARD_NUMBER present, PIN absent -> card masked as before.
func TestConditionalMaskingCardOnly(t *testing.T) {
	text := "Номер карты 4111 1111 1111 1111"
	r := &multiEntityRunner{entities: []entity.Entity{
		span(entity.CARD_NUMBER, text, "4111 1111 1111 1111", 0.9, "regex"),
	}}
	m := newConditionalMasker(t, r)

	masked, mappings, err := m.Mask(context.Background(), text, conditionalPolicy())
	if err != nil {
		t.Fatalf("Mask returned error: %v", err)
	}
	if strings.Contains(masked, "4111 1111 1111 1111") {
		t.Fatalf("expected card number masked, got %q", masked)
	}
	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(mappings))
	}
}

// CASE 4: PIN + supporting CARD_NUMBER from different sources (rule + ML).
func TestConditionalMaskingPinCardDifferentSources(t *testing.T) {
	text := "Номер карты 4111 1111 1111 1111, пин-код 7305"
	r := &multiEntityRunner{entities: []entity.Entity{
		// CARD_NUMBER from rules (regex), PIN from ML.
		span(entity.CARD_NUMBER, text, "4111 1111 1111 1111", 0.9, "regex"),
		span(entity.PIN, text, "7305", 0.9, "ml"),
	}}
	m := newConditionalMasker(t, r)

	masked, mappings, err := m.Mask(context.Background(), text, conditionalPolicy())
	if err != nil {
		t.Fatalf("Mask returned error: %v", err)
	}
	if strings.Contains(masked, "7305") {
		t.Fatalf("expected PIN masked despite different source, got %q", masked)
	}
	if len(mappings) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(mappings))
	}
}

// CASE 5: PIN and CARD_NUMBER in different chunks of one payload.
func TestConditionalMaskingPinCardDifferentChunks(t *testing.T) {
	// Force chunking so card and pin land in separate chunks. A long filler
	// between them guarantees they never straddle a chunk boundary.
	cfg := ml.DefaultChunkConfig()
	cfg.TargetCodePoints = 40
	cfg.MaxCodePoints = 50
	cfg.OverlapCodePoints = 2

	text := "Номер карты 4111 1111 1111 1111" + strings.Repeat(" x", 40) + " пин-код 7305"
	// Chunk-aware runner: returns entities found within the given chunk text,
	// with offsets relative to that chunk.
	r := &chunkAwareRunner{entities: []entity.Entity{
		span(entity.CARD_NUMBER, text, "4111 1111 1111 1111", 0.9, "regex"),
		span(entity.PIN, text, "7305", 0.9, "regex"),
	}}
	m, err := NewCascadeMasker(r, cfg, 2)
	if err != nil {
		t.Fatalf("NewCascadeMasker returned error: %v", err)
	}

	masked, mappings, err := m.Mask(context.Background(), text, conditionalPolicy())
	if err != nil {
		t.Fatalf("Mask returned error: %v", err)
	}
	if strings.Contains(masked, "7305") {
		t.Fatalf("expected PIN masked across chunks, got %q", masked)
	}
	if len(mappings) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(mappings))
	}
}

// chunkAwareRunner returns entities whose spans fall within the given chunk
// text, re-based to chunk-local offsets.
type chunkAwareRunner struct {
	entities []entity.Entity
}

func (c *chunkAwareRunner) Run(_ context.Context, chunkText string) (cascade.Result, error) {
	var out []entity.Entity
	for _, e := range c.entities {
		// Find the entity's text within this chunk.
		idx := strings.Index(chunkText, e.Text)
		if idx < 0 {
			continue
		}
		out = append(out, entity.Entity{
			Type:   e.Type,
			Text:   e.Text,
			Start:  idx,
			End:    idx + len(e.Text),
			Score:  e.Score,
			Reason: e.Reason,
		})
	}
	return cascade.Result{Entities: out}, nil
}

// CASE 6: CARD_NUMBER candidate below MinConfidence -> does not satisfy PIN.
func TestConditionalMaskingPinCardLowConfidence(t *testing.T) {
	text := "Номер карты 4111 1111 1111 1111, пин-код 7305"
	r := &multiEntityRunner{entities: []entity.Entity{
		// Card below MinConfidence (0.5) -> not a confirmed kind.
		span(entity.CARD_NUMBER, text, "4111 1111 1111 1111", 0.1, "regex"),
		span(entity.PIN, text, "7305", 0.9, "regex"),
	}}
	m := newConditionalMasker(t, r)

	masked, mappings, err := m.Mask(context.Background(), text, conditionalPolicy())
	if err != nil {
		t.Fatalf("Mask returned error: %v", err)
	}
	// Card below confidence -> not a confirmed kind -> PIN must NOT be masked.
	if !strings.Contains(masked, "7305") {
		t.Fatalf("expected PIN NOT masked (card below confidence), got %q", masked)
	}
	// Card itself is below confidence too, so nothing masked.
	if len(mappings) != 0 {
		t.Fatalf("expected 0 mappings, got %d", len(mappings))
	}
}

// CASE 7: consumer A has PIN requires CARD; consumer B has no condition.
func TestConditionalMaskingPerConsumer(t *testing.T) {
	text := "Пин-код 7305"
	r := &multiEntityRunner{entities: []entity.Entity{
		span(entity.PIN, text, "7305", 0.9, "regex"),
	}}
	m := newConditionalMasker(t, r)

	// Consumer A: PIN requires CARD -> PIN not masked.
	polA := conditionalPolicy()
	maskedA, mappingsA, err := m.Mask(context.Background(), text, polA)
	if err != nil {
		t.Fatalf("Mask A returned error: %v", err)
	}
	if maskedA != text {
		t.Fatalf("expected PIN not masked for consumer A, got %q", maskedA)
	}
	if len(mappingsA) != 0 {
		t.Fatalf("expected 0 mappings for A, got %d", len(mappingsA))
	}

	// Consumer B: no MaskConditions -> legacy behavior, PIN masked.
	polB := pii.Policy{
		AllowedKinds:  map[pii.PIIKind]bool{pii.PIIKindPIN: true},
		MinConfidence: 0.5,
	}
	maskedB, mappingsB, err := m.Mask(context.Background(), text, polB)
	if err != nil {
		t.Fatalf("Mask B returned error: %v", err)
	}
	if strings.Contains(maskedB, "7305") {
		t.Fatalf("expected PIN masked for consumer B (legacy), got %q", maskedB)
	}
	if len(mappingsB) != 1 {
		t.Fatalf("expected 1 mapping for B, got %d", len(mappingsB))
	}
}

// Retry/detokenization: unmasked PIN produces no TokenMapping; masked PIN+card
// produces mappings that can be detokenized.
func TestConditionalMaskingDetokenization(t *testing.T) {
	// PIN only -> no mapping, so detokenization has nothing to restore.
	text := "Пин-код 7305"
	r := &multiEntityRunner{entities: []entity.Entity{
		span(entity.PIN, text, "7305", 0.9, "regex"),
	}}
	m := newConditionalMasker(t, r)
	_, mappings, err := m.Mask(context.Background(), text, conditionalPolicy())
	if err != nil {
		t.Fatalf("Mask returned error: %v", err)
	}
	if len(mappings) != 0 {
		t.Fatalf("expected no mappings for unmasked PIN, got %d", len(mappings))
	}

	// PIN + card -> both masked, mappings present.
	text2 := "Номер карты 4111 1111 1111 1111, пин-код 7305"
	r2 := &multiEntityRunner{entities: []entity.Entity{
		span(entity.CARD_NUMBER, text2, "4111 1111 1111 1111", 0.9, "regex"),
		span(entity.PIN, text2, "7305", 0.9, "regex"),
	}}
	m2 := newConditionalMasker(t, r2)
	masked2, mappings2, err := m2.Mask(context.Background(), text2, conditionalPolicy())
	if err != nil {
		t.Fatalf("Mask returned error: %v", err)
	}
	if len(mappings2) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(mappings2))
	}
	// Verify each mapping's token is present in the masked text.
	for _, mp := range mappings2 {
		if !strings.Contains(masked2, mp.Token) {
			t.Fatalf("expected token %q in masked text %q", mp.Token, masked2)
		}
	}
}
