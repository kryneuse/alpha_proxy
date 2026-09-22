package tokenizer

import (
	"context"
	"fmt"
	"strings"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

// Detokenize заменяет в тексте все известные токены на их Original согласно
// переданным mappings. Один токен может встречаться несколько раз и
// заменяется везде. Токены вида <PHONE_1> и <PHONE_10> не путаются. Если
// найден токен нашего формата, которого нет в mappings, возвращается ошибка
// с оборачиванием pii.ErrUnknownToken. Обычный текст в угловых скобках, не
// похожий на токен, остаётся без изменений.
func Detokenize(ctx context.Context, text string, mappings []pii.TokenMapping) (string, error) {
	tokenMap := make(map[string]pii.TokenMapping, len(mappings))
	for _, m := range mappings {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if existing, ok := tokenMap[m.Token]; ok {
			if existing.Original != m.Original || existing.Kind != m.Kind {
				return "", fmt.Errorf("token mapping conflict: %w", pii.ErrUnknownToken)
			}
		} else {
			tokenMap[m.Token] = m
		}
	}

	var sb strings.Builder
	i := 0
	for i < len(text) {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		if text[i] == '<' {
			rel := strings.IndexByte(text[i:], '>')
			if rel >= 0 {
				end := i + rel
				inner := text[i+1 : end]
				if isToken(inner) {
					token := text[i : end+1]
					m, ok := tokenMap[token]
					if !ok {
						return "", fmt.Errorf("unknown token: %w", pii.ErrUnknownToken)
					}
					sb.WriteString(m.Original)
					i = end + 1
					continue
				}
			}
		}

		sb.WriteByte(text[i])
		i++
	}

	return sb.String(), nil
}

// isToken проверяет, что строка внутри угловых скобок похожа на токен вида
// KIND_N, где KIND — заглавные буквы и подчёркивания, а N — цифры.
func isToken(s string) bool {
	if len(s) < 3 {
		return false
	}
	us := strings.LastIndexByte(s, '_')
	if us <= 0 || us == len(s)-1 {
		return false
	}
	for _, r := range s[us+1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	for _, r := range s[:us] {
		if r < 'A' || r > 'Z' && r != '_' {
			return false
		}
	}
	return true
}
