package ml

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitTextShortText(t *testing.T) {
	text := "Короткий текст."
	chunks, err := SplitText(context.Background(), text, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("SplitText returned error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Text != text {
		t.Fatalf("expected chunk text %q, got %q", text, chunks[0].Text)
	}
	if chunks[0].StartCodePoint != 0 || chunks[0].EndCodePoint != len([]rune(text)) {
		t.Fatalf("unexpected code point range: %+v", chunks[0])
	}
}

func TestSplitTextBySentences(t *testing.T) {
	text := strings.Repeat("Это предложение. ", 60)
	chunks, err := SplitText(context.Background(), text, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("SplitText returned error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for _, c := range chunks {
		if c.EndCodePoint-c.StartCodePoint > 350 {
			t.Fatalf("chunk exceeds max code points: %+v", c)
		}
	}
}

func TestSplitTextNoOverlapOnSafeBoundary(t *testing.T) {
	// Каждое предложение заканчивается точкой с пробелом — безопасная граница.
	text := strings.Repeat("Это предложение. ", 60)
	chunks, err := SplitText(context.Background(), text, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("SplitText returned error: %v", err)
	}
	for i := 1; i < len(chunks); i++ {
		if chunks[i].StartCodePoint != chunks[i-1].EndCodePoint {
			t.Fatalf("expected no overlap on safe boundary, chunk %d starts at %d but prev ends at %d",
				i, chunks[i].StartCodePoint, chunks[i-1].EndCodePoint)
		}
	}
}

func TestSplitTextOverlapOnLongSentence(t *testing.T) {
	// Одно длинное предложение без безопасных границ внутри — должен быть overlap.
	text := strings.Repeat("а", 1000)
	chunks, err := SplitText(context.Background(), text, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("SplitText returned error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for i := 1; i < len(chunks); i++ {
		expectedStart := chunks[i-1].EndCodePoint - 50
		if chunks[i].StartCodePoint != expectedStart {
			t.Fatalf("expected overlap start %d, got %d", expectedStart, chunks[i].StartCodePoint)
		}
	}
}

func TestSplitTextCyrillic(t *testing.T) {
	text := strings.Repeat("Привет мир. ", 40)
	chunks, err := SplitText(context.Background(), text, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("SplitText returned error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	for _, c := range chunks {
		if c.Text != text[c.StartByte:c.EndByte] {
			t.Fatalf("chunk text does not match byte slice: %+v", c)
		}
	}
}

func TestSplitTextByteOffsets(t *testing.T) {
	text := strings.Repeat("Привет мир. ", 40)
	chunks, err := SplitText(context.Background(), text, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("SplitText returned error: %v", err)
	}
	for _, c := range chunks {
		if c.StartByte != byteOffsetOf(text, c.StartCodePoint) {
			t.Fatalf("StartByte mismatch for chunk %+v", c)
		}
		if c.EndByte != byteOffsetOf(text, c.EndCodePoint) {
			t.Fatalf("EndByte mismatch for chunk %+v", c)
		}
	}
}

func TestSplitTextMaxSize(t *testing.T) {
	text := strings.Repeat("а", 1000)
	chunks, err := SplitText(context.Background(), text, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("SplitText returned error: %v", err)
	}
	for _, c := range chunks {
		if c.EndCodePoint-c.StartCodePoint > 350 {
			t.Fatalf("chunk exceeds max code points: %+v", c)
		}
	}
}

func TestSplitTextInvalidConfig(t *testing.T) {
	cases := []ChunkConfig{
		{TargetCodePoints: 0, MaxCodePoints: 350, OverlapCodePoints: 50},
		{TargetCodePoints: 300, MaxCodePoints: 0, OverlapCodePoints: 50},
		{TargetCodePoints: 400, MaxCodePoints: 350, OverlapCodePoints: 50},
		{TargetCodePoints: 300, MaxCodePoints: 350, OverlapCodePoints: -1},
		{TargetCodePoints: 300, MaxCodePoints: 350, OverlapCodePoints: 350},
	}
	for _, cfg := range cases {
		if _, err := SplitText(context.Background(), "text", cfg); err == nil {
			t.Fatalf("expected error for config %+v", cfg)
		}
	}
}

func TestSplitTextCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := SplitText(ctx, strings.Repeat("а", 1000), DefaultChunkConfig())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestSplitTextCoversWholeText(t *testing.T) {
	text := strings.Repeat("Это предложение. ", 60)
	chunks, err := SplitText(context.Background(), text, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("SplitText returned error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	if chunks[0].StartCodePoint != 0 {
		t.Fatalf("first chunk must start at 0, got %d", chunks[0].StartCodePoint)
	}
	totalCP := len([]rune(text))
	if chunks[len(chunks)-1].EndCodePoint != totalCP {
		t.Fatalf("last chunk must end at %d, got %d", totalCP, chunks[len(chunks)-1].EndCodePoint)
	}
	for i := 1; i < len(chunks); i++ {
		if chunks[i].StartCodePoint > chunks[i-1].EndCodePoint {
			t.Fatalf("gap between chunks %d and %d", i-1, i)
		}
	}
}

func byteOffsetOf(text string, codePoint int) int {
	off := 0
	for i := 0; i < codePoint; i++ {
		_, size := utf8.DecodeRuneInString(text[off:])
		off += size
	}
	return off
}

func TestSplitTextBoundaryBeforeTarget(t *testing.T) {
	// Предложение заканчивается на позиции 290 (чуть раньше target 300).
	// Граница попадает в диапазон поиска [250, 350] и должна быть выбрана
	// без overlap.
	text := strings.Repeat("а", 289) + ". " + strings.Repeat("б", 100)
	chunks, err := SplitText(context.Background(), text, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("SplitText returned error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	if chunks[0].EndCodePoint != 290 {
		t.Fatalf("expected first chunk to end at 290, got %d", chunks[0].EndCodePoint)
	}
	if chunks[1].StartCodePoint != 290 {
		t.Fatalf("expected second chunk to start at 290 without overlap, got %d", chunks[1].StartCodePoint)
	}
}

func TestSplitTextInvalidUTF8(t *testing.T) {
	text := string([]byte{0xff, 0xfe, 0x41})
	_, err := SplitText(context.Background(), text, DefaultChunkConfig())
	if err == nil {
		t.Fatal("expected error for invalid UTF-8")
	}
}
