package recognizer

import (
	"strings"

	"github.com/kryneuse/alpha_proxy/internal/dict"
	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/normalize"
)

// FullNameRecognizer detects full names using a name dictionary and Russian
// name-ending heuristics. It finds maximal runs of name-like tokens so that
// leading context words ("Сотрудник") and trailing ordinary words ("гулял")
// are not included in the span. Known-person names are treated as a reference
// signal (lower base score), not an absolute ban; context decides.
type FullNameRecognizer struct {
	firstNames *dict.Dict
	lastNames  *dict.Dict
	known      *dict.Dict
}

// NewFullNameRecognizer builds a full name recognizer.
func NewFullNameRecognizer(firstNames, lastNames, known *dict.Dict) *FullNameRecognizer {
	if firstNames == nil {
		firstNames = dict.FirstNames
	}
	if lastNames == nil {
		lastNames = dict.LastNames
	}
	if known == nil {
		known = dict.KnownPersonNames
	}
	return &FullNameRecognizer{
		firstNames: firstNames,
		lastNames:  lastNames,
		known:      known,
	}
}

// Type returns the entity type.
func (r *FullNameRecognizer) Type() entity.Type { return entity.FULL_NAME }

// Recognize finds full name candidates as maximal runs of name-like tokens.
func (r *FullNameRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	words := tokenize(norm.Normalized)
	var spans []entity.CandidateSpan

	i := 0
	for i < len(words) {
		if !r.isNameLike(words[i]) {
			i++
			continue
		}
		// Build a maximal run of name-like tokens (max 3).
		j := i
		for j < len(words) && j-i < 3 && r.isNameLike(words[j]) {
			j++
		}
		if j-i < 2 {
			i++
			continue
		}
		// The run [i,j) is a candidate name.
		startByte := words[i].start
		endByte := words[j-1].end
		text := norm.Normalized[startByte:endByte]
		score := r.scoreName(words[i:j])
		if score <= 0 {
			i++
			continue
		}
		// Known-person reference: lower the base score as a weak negative
		// signal, but do not drop the candidate outright.
		if r.known.Contains(text) {
			score -= 0.15
			if score < 0 {
				score = 0
			}
		}
		oStart, oEnd := norm.MapSpan(norm.ByteToRune(startByte), norm.ByteToRune(endByte))
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.FULL_NAME,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   score,
			Sources: []entity.Source{entity.SourceRegex, entity.SourceDictionary},
			Reason:  "name-heuristic",
		})
		i = j
	}
	return spans
}

// token is a word with its byte offsets in the normalized string.
type token struct {
	text  string
	start int
	end   int
}

// tokenize splits the normalized text into words with byte offsets.
func tokenize(s string) []token {
	var out []token
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		// Skip non-letters.
		for i < len(runes) && !isLetter(runes[i]) {
			i++
		}
		if i >= len(runes) {
			break
		}
		startRune := i
		for i < len(runes) && isLetter(runes[i]) {
			i++
		}
		startByte := len(string(runes[:startRune]))
		endByte := len(string(runes[:i]))
		out = append(out, token{text: s[startByte:endByte], start: startByte, end: endByte})
	}
	return out
}

// isNameLike reports whether a token looks like a name component.
func (r *FullNameRecognizer) isNameLike(t token) bool {
	lower := strings.ToLower(t.text)
	return r.firstNames.Contains(lower) ||
		r.lastNames.Contains(lower) ||
		isPatronymic(lower) ||
		isLastNameEnding(lower)
}

// scoreName scores a name candidate. Returns 0 if it is not a plausible name.
func (r *FullNameRecognizer) scoreName(tokens []token) float64 {
	score := 0.0
	matched := 0

	for _, t := range tokens {
		lower := strings.ToLower(t.text)
		if r.firstNames.Contains(lower) {
			score += 0.4
			matched++
		} else if r.lastNames.Contains(lower) {
			score += 0.35
			matched++
		} else if isPatronymic(lower) {
			score += 0.3
			matched++
		} else if isLastNameEnding(lower) {
			score += 0.2
			matched++
		}
	}

	if matched == 0 {
		return 0
	}

	// A 3-word name with a patronymic is a strong signal.
	if len(tokens) == 3 && isPatronymic(strings.ToLower(tokens[1].text)) {
		score += 0.2
	}

	if score > 1 {
		score = 1
	}
	return score
}

// isLetter reports whether r is a Cyrillic or latin letter.
func isLetter(r rune) bool {
	return (r >= 'а' && r <= 'я') || r == 'ё' ||
		(r >= 'a' && r <= 'z') || (r >= 'А' && r <= 'Я') || r == 'Ё' ||
		(r >= 'A' && r <= 'Z')
}

// isPatronymic reports whether a word looks like a Russian patronymic.
func isPatronymic(w string) bool {
	for _, suffix := range []string{"ович", "евич", "овна", "евна", "ична", "ич", "чна"} {
		if strings.HasSuffix(w, suffix) && len(w) > len(suffix)+2 {
			return true
		}
	}
	return false
}

// isLastNameEnding reports whether a word looks like a Russian last name.
func isLastNameEnding(w string) bool {
	for _, suffix := range []string{"ов", "ев", "ин", "ын", "ский", "ской", "цкий", "цкой", "ова", "ева", "ина", "ына", "ская", "цкая"} {
		if strings.HasSuffix(w, suffix) && len(w) > len(suffix)+2 {
			return true
		}
	}
	return false
}
