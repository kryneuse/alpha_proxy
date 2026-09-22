package cascade

import (
	"context"
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/gate"
)

type benchEngine struct{}

func (b *benchEngine) Analyze(text string) []entity.Entity {
	return []entity.Entity{
		{Type: entity.FULL_NAME, Text: "Иван Петров", Start: 8, End: 19},
	}
}

type benchCheap struct{}

func (b *benchCheap) HasPII(ctx context.Context, text string) (float64, error) {
	return 0.2, nil
}

type benchExpensive struct{}

func (b *benchExpensive) Detect(ctx context.Context, text string) ([]entity.Entity, error) {
	return nil, nil
}

var benchChunk = "Клиент Иван Петров, паспорт 4510 123456, тел. +7 (912) 345-67-89"

func BenchmarkCascadeSafe(b *testing.B) {
	c := New(&benchEngine{}, gate.New(gate.DefaultConfig()), &benchCheap{}, &benchExpensive{})
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Run(ctx, "сегодня хорошая погода")
	}
}

func BenchmarkCascadeUncertain(b *testing.B) {
	c := New(&benchEngine{}, gate.New(gate.DefaultConfig()), &benchCheap{}, &benchExpensive{})
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Run(ctx, benchChunk)
	}
}
