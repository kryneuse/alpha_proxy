package ml

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

func TestTakeBatchLimitByItems(t *testing.T) {
	items := make([]RequestItem, 10)
	for i := range items {
		items[i] = RequestItem{ChunkID: string(rune('a' + i)), Text: "x"}
	}
	cfg := DefaultBatchConfig()
	cfg.MaxItems = 3

	selected, rest, err := TakeBatch(context.Background(), items, cfg)
	if err != nil {
		t.Fatalf("TakeBatch returned error: %v", err)
	}
	if len(selected) != 3 {
		t.Fatalf("expected 3 selected, got %d", len(selected))
	}
	if len(rest) != 7 {
		t.Fatalf("expected 7 remaining, got %d", len(rest))
	}
}

func TestTakeBatchLimitByCodePoints(t *testing.T) {
	items := []RequestItem{
		{ChunkID: "a", Text: strings.Repeat("x", 100)},
		{ChunkID: "b", Text: strings.Repeat("x", 100)},
		{ChunkID: "c", Text: strings.Repeat("x", 100)},
	}
	cfg := DefaultBatchConfig()
	cfg.MaxCodePoints = 250

	selected, rest, err := TakeBatch(context.Background(), items, cfg)
	if err != nil {
		t.Fatalf("TakeBatch returned error: %v", err)
	}
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected (200 <= 250), got %d", len(selected))
	}
	if len(rest) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(rest))
	}
}

func TestTakeBatchCountsCyrillicByRunes(t *testing.T) {
	// Каждый кириллический символ занимает 2 байта, но считается как 1 code point.
	items := []RequestItem{
		{ChunkID: "a", Text: strings.Repeat("я", 100)}, // 100 code points, 200 bytes
		{ChunkID: "b", Text: strings.Repeat("я", 100)}, // 100 code points, 200 bytes
	}
	cfg := DefaultBatchConfig()
	cfg.MaxCodePoints = 150

	selected, rest, err := TakeBatch(context.Background(), items, cfg)
	if err != nil {
		t.Fatalf("TakeBatch returned error: %v", err)
	}
	if len(selected) != 1 {
		t.Fatalf("expected 1 selected (100 <= 150), got %d", len(selected))
	}
	if len(rest) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(rest))
	}
}

func TestTakeBatchFirstItemTooLarge(t *testing.T) {
	items := []RequestItem{
		{ChunkID: "a", Text: strings.Repeat("x", 200)},
	}
	cfg := DefaultBatchConfig()
	cfg.MaxCodePoints = 100

	_, _, err := TakeBatch(context.Background(), items, cfg)
	if !errors.Is(err, pii.ErrPayloadTooLarge) {
		t.Fatalf("expected ErrPayloadTooLarge, got %v", err)
	}
	//nolint:errorlint // Direct comparison intentionally verifies that the sentinel was wrapped.
	if err == pii.ErrPayloadTooLarge {
		t.Fatal("expected a wrapped error, not the sentinel directly")
	}
}

func TestTakeBatchItemExactlyAtMaxCodePoints(t *testing.T) {
	items := []RequestItem{
		{ChunkID: "a", Text: strings.Repeat("x", 100)},
	}
	cfg := DefaultBatchConfig()
	cfg.MaxCodePoints = 100

	selected, rest, err := TakeBatch(context.Background(), items, cfg)
	if err != nil {
		t.Fatalf("TakeBatch returned error: %v", err)
	}
	if len(selected) != 1 {
		t.Fatalf("expected 1 selected, got %d", len(selected))
	}
	if len(rest) != 0 {
		t.Fatalf("expected 0 remaining, got %d", len(rest))
	}
}

func TestTakeBatchEmptyInput(t *testing.T) {
	selected, rest, err := TakeBatch(context.Background(), nil, DefaultBatchConfig())
	if err != nil {
		t.Fatalf("TakeBatch returned error: %v", err)
	}
	if len(selected) != 0 || len(rest) != 0 {
		t.Fatalf("expected empty lists, got %d and %d", len(selected), len(rest))
	}
}

func TestTakeBatchCancelledContext(t *testing.T) {
	items := make([]RequestItem, 5)
	for i := range items {
		items[i] = RequestItem{ChunkID: string(rune('a' + i)), Text: "x"}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := TakeBatch(ctx, items, DefaultBatchConfig())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestTakeBatchInvalidConfig(t *testing.T) {
	cases := []BatchConfig{
		{MaxItems: 0, MaxCodePoints: 11200, MaxWait: 1, QueueCapacity: 1, Workers: 1},
		{MaxItems: 32, MaxCodePoints: 0, MaxWait: 1, QueueCapacity: 1, Workers: 1},
		{MaxItems: 32, MaxCodePoints: 11200, MaxWait: 0, QueueCapacity: 1, Workers: 1},
		{MaxItems: 32, MaxCodePoints: 11200, MaxWait: 1, QueueCapacity: 0, Workers: 1},
		{MaxItems: 32, MaxCodePoints: 11200, MaxWait: 1, QueueCapacity: 1, Workers: 0},
	}
	for _, cfg := range cases {
		if _, _, err := TakeBatch(context.Background(), nil, cfg); err == nil {
			t.Fatalf("expected error for config %+v", cfg)
		}
	}
}

func TestTakeBatchDoesNotMutateInput(t *testing.T) {
	items := []RequestItem{
		{ChunkID: "a", Text: "one"},
		{ChunkID: "b", Text: "two"},
		{ChunkID: "c", Text: "three"},
	}
	original := make([]RequestItem, len(items))
	copy(original, items)

	cfg := DefaultBatchConfig()
	cfg.MaxItems = 2

	if _, _, err := TakeBatch(context.Background(), items, cfg); err != nil {
		t.Fatalf("TakeBatch returned error: %v", err)
	}

	if len(items) != len(original) {
		t.Fatal("input slice length changed")
	}
	for i := range items {
		if items[i] != original[i] {
			t.Fatalf("input item %d was mutated", i)
		}
	}
}
