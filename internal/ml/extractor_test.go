package ml

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/pii"
)

// entityClient возвращает заданные entities для каждого chunk запроса.
type entityClient struct {
	entities []MLEntity
	err      error
}

func (e *entityClient) ProcessBatch(_ context.Context, req BatchRequest) (BatchResponse, error) {
	if e.err != nil {
		return BatchResponse{}, e.err
	}
	results := make([]ChunkResult, 0, len(req.Items))
	for _, item := range req.Items {
		results = append(results, ChunkResult{ChunkID: item.ChunkID, Entities: e.entities})
	}
	return BatchResponse{
		BatchID:      req.BatchID,
		ModelVersion: "v1",
		OffsetUnit:   OffsetsUnicodeCodePoints,
		Results:      results,
	}, nil
}

func newTestExtractor(t *testing.T, client Client) *Extractor {
	t.Helper()
	cfg := DefaultBatchConfig()
	cfg.MaxItems = 1
	cfg.MaxWait = 10 * time.Millisecond
	cfg.Workers = 1
	b, err := NewBatcher(client, cfg)
	if err != nil {
		t.Fatalf("NewBatcher returned error: %v", err)
	}
	t.Cleanup(b.Close)
	e, err := NewExtractor(b)
	if err != nil {
		t.Fatalf("NewExtractor returned error: %v", err)
	}
	return e
}

func TestExtractorASCII(t *testing.T) {
	ec := &entityClient{entities: []MLEntity{
		{Type: "phone", Start: 5, End: 16, Confidence: 0.9},
	}}
	e := newTestExtractor(t, ec)

	ents, err := e.Detect(context.Background(), "call 79123456789")
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(ents))
	}
	en := ents[0]
	if en.Type != entity.PHONE || en.Start != 5 || en.End != 16 || en.Text != "79123456789" {
		t.Fatalf("unexpected entity: %+v", en)
	}
	if en.Score != 0.9 || en.Reason != "ml" {
		t.Fatalf("unexpected score/reason: %+v", en)
	}
}

func TestExtractorCyrillic(t *testing.T) {
	ec := &entityClient{entities: []MLEntity{
		{Type: "phone", Start: 6, End: 17, Confidence: 0.9},
	}}
	e := newTestExtractor(t, ec)

	ents, err := e.Detect(context.Background(), "звони 79123456789")
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(ents))
	}
	en := ents[0]
	// "звони " = 5 символов × 2 байта = 10 байт + 1 пробел = 11; телефон с 11 по 22.
	if en.Start != 11 || en.End != 22 || en.Text != "79123456789" {
		t.Fatalf("unexpected entity: %+v", en)
	}
}

func TestExtractorEmoji(t *testing.T) {
	ec := &entityClient{entities: []MLEntity{
		{Type: "phone", Start: 2, End: 13, Confidence: 0.9},
	}}
	e := newTestExtractor(t, ec)

	ents, err := e.Detect(context.Background(), "😀 79123456789")
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(ents))
	}
	en := ents[0]
	// "😀 " = 4 + 1 = 5 байт; телефон с 5 по 16.
	if en.Start != 5 || en.End != 16 || en.Text != "79123456789" {
		t.Fatalf("unexpected entity: %+v", en)
	}
}

func TestExtractorTypeMapping(t *testing.T) {
	cases := []struct {
		mlType string
		want   entity.Type
	}{
		{"full_name", entity.FULL_NAME},
		{"first_name", entity.FULL_NAME},
		{"last_name", entity.FULL_NAME},
		{"patronymic", entity.FULL_NAME},
		{"address", entity.ADDRESS},
		{"city", entity.ADDRESS},
		{"street", entity.ADDRESS},
		{"house", entity.ADDRESS},
		{"apartment", entity.ADDRESS},
		{"postal_code", entity.ADDRESS},
		{"place_of_birth", entity.BIRTH_PLACE},
		{"citizenship", entity.CITIZENSHIP},
		{"passport_issuer", entity.PASSPORT_ISSUER},
		{"cardholder_name", entity.CARDHOLDER_NAME},
		{"email", entity.EMAIL},
		{"phone", entity.PHONE},
		{"inn", entity.INN},
		{"passport", entity.PASSPORT},
		{"department_code", entity.DEPARTMENT_CODE},
		{"driver_license", entity.DRIVER_LICENSE},
		{"cvv", entity.CVV},
		{"pin", entity.PIN},
		{"card", entity.CARD_NUMBER},
		{"date", entity.BIRTH_DATE},
	}
	for _, c := range cases {
		t.Run(c.mlType, func(t *testing.T) {
			ec := &entityClient{entities: []MLEntity{
				{Type: c.mlType, Start: 0, End: 1, Confidence: 0.9},
			}}
			e := newTestExtractor(t, ec)
			ents, err := e.Detect(context.Background(), "x")
			if err != nil {
				t.Fatalf("Detect returned error: %v", err)
			}
			if len(ents) != 1 || ents[0].Type != c.want {
				t.Fatalf("expected type %s, got %+v", c.want, ents)
			}
		})
	}
}

func TestExtractorUnknownType(t *testing.T) {
	ec := &entityClient{entities: []MLEntity{
		{Type: "unknown", Start: 0, End: 1, Confidence: 0.9},
	}}
	e := newTestExtractor(t, ec)

	_, err := e.Detect(context.Background(), "x")
	if !errors.Is(err, pii.ErrInvalidMLResponse) {
		t.Fatalf("expected ErrInvalidMLResponse, got %v", err)
	}
}

func TestExtractorInvalidCoords(t *testing.T) {
	cases := []struct {
		name  string
		start int
		end   int
	}{
		{name: "negative start", start: -1, end: 1},
		{name: "end beyond text", start: 0, end: 10},
		{name: "start not less than end", start: 2, end: 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ec := &entityClient{entities: []MLEntity{
				{Type: "phone", Start: c.start, End: c.end, Confidence: 0.9},
			}}
			e := newTestExtractor(t, ec)
			_, err := e.Detect(context.Background(), "x")
			if !errors.Is(err, pii.ErrInvalidMLResponse) {
				t.Fatalf("expected ErrInvalidMLResponse, got %v", err)
			}
		})
	}
}

func TestExtractorEmptyText(t *testing.T) {
	ec := &entityClient{entities: []MLEntity{
		{Type: "phone", Start: 0, End: 1, Confidence: 0.9},
	}}
	e := newTestExtractor(t, ec)

	ents, err := e.Detect(context.Background(), "")
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if len(ents) != 0 {
		t.Fatalf("expected 0 entities, got %d", len(ents))
	}
}

func TestExtractorBatcherError(t *testing.T) {
	ec := &entityClient{err: errors.New("batcher failed")}
	e := newTestExtractor(t, ec)

	_, err := e.Detect(context.Background(), "call 79123456789")
	if err == nil || err.Error() != "batcher failed" {
		t.Fatalf("expected batcher error, got %v", err)
	}
}

func TestExtractorCancelledContext(t *testing.T) {
	ec := &entityClient{entities: []MLEntity{
		{Type: "phone", Start: 0, End: 1, Confidence: 0.9},
	}}
	e := newTestExtractor(t, ec)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := e.Detect(ctx, "call 79123456789")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestNewExtractorNilBatcher(t *testing.T) {
	if _, err := NewExtractor(nil); err == nil {
		t.Fatal("expected error for nil batcher")
	}
}
