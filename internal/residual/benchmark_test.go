package residual

import (
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/entity"
)

var residualBenchText = "Клиент Иван Петров, паспорт 4510 123456, тел. +7 (912) 345-67-89, email ivanov@example.com"

var residualBenchSpans = []entity.Entity{
	{Type: "FULL_NAME", Text: "Иван Петров", Start: 8, End: 19},
	{Type: "PASSPORT", Text: "4510 123456", Start: 29, End: 40},
	{Type: "PHONE", Text: "+7 (912) 345-67-89", Start: 47, End: 65},
	{Type: "EMAIL", Text: "ivanov@example.com", Start: 73, End: 91},
}

func BenchmarkResidualBuild(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Build(residualBenchText, residualBenchSpans)
	}
}

func BenchmarkResidualNoSpans(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Build(residualBenchText, nil)
	}
}
