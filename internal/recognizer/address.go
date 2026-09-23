package recognizer

import (
	"strings"
	"unicode"

	"github.com/kryneuse/alpha_proxy/internal/dict"
	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/normalize"
)

// AddressRecognizer detects addresses by first finding field signatures and
// then extracting the value after each signature. This supports multiple
// addresses in one text and values on the following line.
type AddressRecognizer struct {
	structure *dict.Dict
}

// NewAddressRecognizer builds an address recognizer.
func NewAddressRecognizer(structure *dict.Dict) *AddressRecognizer {
	if structure == nil {
		structure = dict.AddressStructureKeywords
	}
	return &AddressRecognizer{structure: structure}
}

// Type returns the entity type.
func (r *AddressRecognizer) Type() entity.Type { return entity.ADDRESS }

// addressSignatures are the field signatures, longer variants first.
var addressSignatures = []string{
	"адрес фактического проживания",
	"адрес фактической регистрации",
	"адрес проживания",
	"адрес регистрации",
	"адрес доставки",
	"место жительства",
	"место регистрации",
	"проживает по адресу",
	"живет по адресу",
	"живёт по адресу",
	"зарегистрирован по адресу",
	"зарегистрирована по адресу",
	"прописан по адресу",
	"прописана по адресу",
	"адрес",
}

// addressNextFields are the field signatures that terminate an address value.
var addressNextFields = []string{
	"телефон", "тел", "email", "e-mail", "инн", "паспорт", "гражданство",
	"дата", "cvv", "пин", "покупатель", "получатель", "сумма", "договор",
	"фио", "ф.и.о", "код", "номер карты",
}

// maxAddressRunes is the maximum length of an address candidate.
const maxAddressRunes = 120

// Recognize finds address candidates.
func (r *AddressRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	runes := []rune(norm.Normalized)
	var spans []entity.CandidateSpan
	for _, sig := range findAddressSignatures(runes) {
		valueStart, valueEnd, ok := r.extractValue(runes, sig)
		if !ok {
			continue
		}
		value := string(runes[valueStart:valueEnd])
		if !r.hasStructure(value) {
			continue
		}
		oStart, oEnd := norm.MapSpan(valueStart, valueEnd)
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.ADDRESS,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   0.6,
			Sources: []entity.Source{entity.SourceRegex, entity.SourceContext},
			Reason:  "regex:address",
		})
	}
	return spans
}

// findAddressSignatures returns all address signature matches, keeping the
// longest match at each start position, sorted by position.
func findAddressSignatures(runes []rune) []sigMatch {
	var matches []sigMatch
	for _, sig := range addressSignatures {
		sigRunes := []rune(sig)
		for i := 0; i+len(sigRunes) <= len(runes); i++ {
			if matchRunes(runes, i, sigRunes) && wordBounded(runes, i, i+len(sigRunes)) {
				matches = append(matches, sigMatch{start: i, end: i + len(sigRunes)})
			}
		}
	}
	// Keep the longest match at each start position.
	byStart := make(map[int]sigMatch)
	for _, m := range matches {
		if prev, ok := byStart[m.start]; !ok || m.end-m.start > prev.end-prev.start {
			byStart[m.start] = m
		}
	}
	out := make([]sigMatch, 0, len(byStart))
	for _, m := range byStart {
		out = append(out, m)
	}
	// Sort by start position.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].start < out[j-1].start; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// extractValue extracts the address value after a signature. It skips a
// colon and whitespace, then extends until the next field signature, a
// semicolon, or the length limit.
func (r *AddressRecognizer) extractValue(runes []rune, sig sigMatch) (int, int, bool) {
	i := sig.end
	for i < len(runes) && (runes[i] == ':' || unicode.IsSpace(runes[i])) {
		i++
	}
	valueStart := i
	limit := i + maxAddressRunes
	if limit > len(runes) {
		limit = len(runes)
	}
	valueEnd := i
	for valueEnd < limit {
		if runes[valueEnd] == ';' {
			break
		}
		if nextFieldAt(runes, valueEnd) {
			break
		}
		valueEnd++
	}
	// Trim trailing whitespace and separators.
	for valueEnd > valueStart {
		ch := runes[valueEnd-1]
		if unicode.IsSpace(ch) || ch == ',' || ch == ';' || ch == ':' {
			valueEnd--
		} else {
			break
		}
	}
	if valueEnd <= valueStart {
		return 0, 0, false
	}
	return valueStart, valueEnd, true
}

// nextFieldAt reports whether an address signature or a next-field signature
// starts at pos.
func nextFieldAt(runes []rune, pos int) bool {
	for _, sig := range addressSignatures {
		sigRunes := []rune(sig)
		if pos+len(sigRunes) <= len(runes) && matchRunes(runes, pos, sigRunes) && wordBounded(runes, pos, pos+len(sigRunes)) {
			return true
		}
	}
	for _, sig := range addressNextFields {
		sigRunes := []rune(sig)
		if pos+len(sigRunes) <= len(runes) && matchRunes(runes, pos, sigRunes) && wordBounded(runes, pos, pos+len(sigRunes)) {
			return true
		}
	}
	return false
}

// hasStructure reports whether the address contains structural keywords.
func (r *AddressRecognizer) hasStructure(addr string) bool {
	words := strings.Fields(addr)
	for _, w := range words {
		if r.structure.Contains(w) {
			return true
		}
	}
	for _, w := range words {
		if len(w) == 6 && isAllDigits(w) {
			return true
		}
	}
	return false
}

// isAllDigits reports whether s consists only of digits.
func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}
