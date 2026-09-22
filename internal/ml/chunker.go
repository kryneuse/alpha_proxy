package ml

import (
	"context"
	"errors"
	"unicode/utf8"
)

// ChunkConfig — настройки разбиения текста на chunks.
type ChunkConfig struct {
	TargetCodePoints  int
	MaxCodePoints     int
	OverlapCodePoints int
}

// DefaultChunkConfig возвращает конфигурацию по умолчанию:
// target 300, max 350, overlap 50.
func DefaultChunkConfig() ChunkConfig {
	return ChunkConfig{
		TargetCodePoints:  300,
		MaxCodePoints:     350,
		OverlapCodePoints: 50,
	}
}

// TextChunk — один непрерывный фрагмент исходного текста.
// CodePoint позиции считаются в Unicode code points, Byte позиции — в байтах
// исходной Go строки.
type TextChunk struct {
	Text           string
	StartCodePoint int
	EndCodePoint   int
	StartByte      int
	EndByte        int
}

// SplitText разбивает исходный текст на chunks согласно конфигурации.
// Каждый chunk является непрерывным фрагментом исходного текста и не
// превышает MaxCodePoints. Сначала ищется безопасная граница (абзац или
// предложение) около TargetCodePoints; если она найдена, следующий chunk
// начинается сразу после неё без overlap. Если до MaxCodePoints безопасной
// границы нет, chunk режется на MaxCodePoints, а следующий начинается с
// повторением OverlapCodePoints. UTF-8 символы не разрезаются.
func SplitText(ctx context.Context, text string, cfg ChunkConfig) ([]TextChunk, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if text == "" {
		return nil, nil
	}
	if !utf8.ValidString(text) {
		return nil, errors.New("text is not valid UTF-8")
	}

	runes := []rune(text)
	byteOffsets := make([]int, 0, len(runes)+1)
	for i := range text {
		byteOffsets = append(byteOffsets, i)
	}
	byteOffsets = append(byteOffsets, len(text))

	var chunks []TextChunk
	startCP := 0
	startByte := 0

	for startCP < len(runes) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		endCP, endByte, safe := findChunkEnd(runes, byteOffsets, startCP, cfg)

		chunks = append(chunks, TextChunk{
			Text:           text[startByte:endByte],
			StartCodePoint: startCP,
			EndCodePoint:   endCP,
			StartByte:      startByte,
			EndByte:        endByte,
		})

		if endCP >= len(runes) {
			break
		}

		if safe {
			startCP = endCP
		} else {
			startCP = endCP - cfg.OverlapCodePoints
		}
		startByte = byteOffsets[startCP]
	}

	return chunks, nil
}

func (c ChunkConfig) validate() error {
	if c.TargetCodePoints <= 0 {
		return errors.New("target code points must be > 0")
	}
	if c.MaxCodePoints <= 0 {
		return errors.New("max code points must be > 0")
	}
	if c.TargetCodePoints > c.MaxCodePoints {
		return errors.New("target code points must not exceed max code points")
	}
	if c.OverlapCodePoints < 0 || c.OverlapCodePoints >= c.MaxCodePoints {
		return errors.New("overlap code points must be >= 0 and < max code points")
	}
	return nil
}

func findChunkEnd(runes []rune, byteOffsets []int, startCP int, cfg ChunkConfig) (endCP, endByte int, safe bool) {
	textLen := len(runes)

	// Если весь оставшийся текст умещается в target, берём его целиком.
	if startCP+cfg.TargetCodePoints >= textLen {
		return textLen, byteOffsets[textLen], true
	}

	maxEnd := startCP + cfg.MaxCodePoints
	if maxEnd > textLen {
		maxEnd = textLen
	}

	// Диапазон поиска безопасной границы: от target минус overlap до max.
	low := startCP + cfg.TargetCodePoints - cfg.OverlapCodePoints
	if low < startCP+1 {
		low = startCP + 1
	}

	// Ищем безопасную границу, ближайшую к target (при равенстве — более раннюю).
	best := -1
	bestDist := int(^uint(0) >> 1)
	for cp := low; cp <= maxEnd; cp++ {
		if !isBoundary(runes, cp) {
			continue
		}
		dist := cp - (startCP + cfg.TargetCodePoints)
		if dist < 0 {
			dist = -dist
		}
		if dist < bestDist {
			bestDist = dist
			best = cp
		}
	}

	if best >= 0 {
		return best, byteOffsets[best], true
	}

	// Безопасной границы нет — режем на max.
	return maxEnd, byteOffsets[maxEnd], false
}

func isBoundary(runes []rune, cp int) bool {
	if cp <= 0 || cp >= len(runes) {
		return false
	}
	prev := runes[cp-1]
	if prev == '\n' {
		return true
	}
	if prev == '.' || prev == '?' || prev == '!' {
		if cp == len(runes) {
			return true
		}
		next := runes[cp]
		return next == ' ' || next == '\n'
	}
	return false
}
