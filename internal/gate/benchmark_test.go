package gate

import "testing"

var gateBenchText = "паспорт 4510 123456, клиент Иван Петров, тел. +7 (912) 345-67-89"

func BenchmarkGateEvaluate(b *testing.B) {
	g := New(DefaultConfig())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Evaluate(gateBenchText)
	}
}

func BenchmarkGateNeutral(b *testing.B) {
	g := New(DefaultConfig())
	text := "сегодня хорошая погода на улице"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Evaluate(text)
	}
}
