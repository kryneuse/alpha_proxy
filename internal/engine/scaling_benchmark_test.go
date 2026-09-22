package engine

import (
	"strings"
	"testing"
)

// buildNeutralText builds a neutral Russian text with the given number of
// tokens (words). It contains no personal data.
func buildNeutralText(tokens int) string {
	words := []string{
		"сегодня", "погода", "была", "хорошая", "и", "солнечная", "на", "улице",
		"люди", "гуляли", "по", "парку", "дети", "играли", "в", "мяч",
		"птицы", "пели", "на", "деревьях", "ветер", "дул", "с", "моря",
		"облака", "плыли", "по", "небу", "город", "просыпался", "медленно",
	}
	var sb strings.Builder
	for i := 0; i < tokens; i++ {
		sb.WriteString(words[i%len(words)])
		sb.WriteByte(' ')
	}
	return sb.String()
}

// buildEntityRichText builds a text with many repeated personal-data entities
// (FULL_NAME, EMAIL, PHONE) to stress candidate generation.
func buildEntityRichText(entities int) string {
	var sb strings.Builder
	for i := 0; i < entities; i++ {
		sb.WriteString("Клиент Иванов Иван Петрович, email ivanov@example.com, тел. +7 (912) 345-67-89. ")
	}
	return sb.String()
}

func BenchmarkPipelineNeutral1k(b *testing.B) {
	e := New(Options{})
	text := buildNeutralText(1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Analyze(text)
	}
}

func BenchmarkPipelineNeutral10k(b *testing.B) {
	e := New(Options{})
	text := buildNeutralText(10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Analyze(text)
	}
}

func BenchmarkPipelineNeutral100k(b *testing.B) {
	e := New(Options{})
	text := buildNeutralText(100000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Analyze(text)
	}
}

func BenchmarkPipelineEntityRich1k(b *testing.B) {
	e := New(Options{})
	text := buildEntityRichText(1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Analyze(text)
	}
}

func BenchmarkPipelineEntityRich10k(b *testing.B) {
	e := New(Options{})
	text := buildEntityRichText(10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Analyze(text)
	}
}
