package cascade

import (
	"context"
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/entity"
)

type benchEngine struct{}

func (b *benchEngine) Analyze(text string) []entity.Entity {
	return []entity.Entity{
		{Type: entity.FULL_NAME, Text: "Иван Петров", Start: 8, End: 19},
	}
}

type benchExpensive struct{}

func (b *benchExpensive) Detect(ctx context.Context, original, gate string) ([]entity.Entity, error) {
	return nil, nil
}

var benchChunk = "Клиент Иван Петров, паспорт 4510 123456, тел. +7 (912) 345-67-89"

func BenchmarkCascadeAnalyzeRules(b *testing.B) {
	c := New(&benchEngine{}, &benchExpensive{})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.AnalyzeRules(benchChunk)
	}
}

func BenchmarkCascadeDetectChunk(b *testing.B) {
	c := New(&benchEngine{}, &benchExpensive{})
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.DetectChunk(ctx, benchChunk, benchChunk)
	}
}