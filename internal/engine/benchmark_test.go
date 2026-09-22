package engine

import (
	"testing"

	"github.com/alpha-proxy/rule-engine/internal/normalize"
	"github.com/alpha-proxy/rule-engine/internal/recognizer"
)

var benchmarkText = "Клиент: Иванов Иван Петрович, дата рождения 15.03.1990, " +
	"паспорт 4510 123456, выдан ОВМ УМВД России по г. Москве, " +
	"код подразделения 770-001, ИНН 7707083893, " +
	"тел. +7 (912) 345-67-89, email ivanov@example.com, " +
	"адрес: г. Москва, ул. Тверская, д. 10, кв. 5, " +
	"номер карты 4532 0151 1283 0366, CVV 123, пин-код 7305"

func BenchmarkPipeline(b *testing.B) {
	e := New(Options{})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Analyze(benchmarkText)
	}
}

func BenchmarkNormalize(b *testing.B) {
	for i := 0; i < b.N; i++ {
		normalize.New(benchmarkText)
	}
}

func BenchmarkEmailRecognizer(b *testing.B) {
	rec := recognizer.NewEmailRecognizer()
	norm := normalize.New(benchmarkText)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec.Recognize(norm)
	}
}

func BenchmarkInnRecognizer(b *testing.B) {
	rec := recognizer.NewInnRecognizer()
	norm := normalize.New(benchmarkText)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec.Recognize(norm)
	}
}

func BenchmarkCardNumberRecognizer(b *testing.B) {
	rec := recognizer.NewCardNumberRecognizer()
	norm := normalize.New(benchmarkText)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec.Recognize(norm)
	}
}

func BenchmarkFullNameRecognizer(b *testing.B) {
	rec := recognizer.NewFullNameRecognizer(nil, nil, nil)
	norm := normalize.New(benchmarkText)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec.Recognize(norm)
	}
}

func BenchmarkDateRecognizer(b *testing.B) {
	rec := recognizer.NewDateRecognizer()
	norm := normalize.New(benchmarkText)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec.Recognize(norm)
	}
}
