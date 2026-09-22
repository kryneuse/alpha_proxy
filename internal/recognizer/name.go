package recognizer

import (
	"regexp"
	"strings"

	"github.com/alpha-proxy/rule-engine/internal/dict"
	"github.com/alpha-proxy/rule-engine/internal/entity"
	"github.com/alpha-proxy/rule-engine/internal/normalize"
)

// FullNameRecognizer detects full names using context, a name dictionary and
// Russian name-ending heuristics. It never treats arbitrary capitalized words
// as personal data.
type FullNameRecognizer struct {
	firstNames *dict.Dict
	lastNames  *dict.Dict
	known      *dict.Dict
	re         *regexp.Regexp
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
		re:         regexp.MustCompile(`[а-яё]+(?:\s+[а-яё]+){1,2}`),
	}
}

// Type returns the entity type.
func (r *FullNameRecognizer) Type() entity.Type { return entity.FULL_NAME }

// Recognize finds full name candidates.
func (r *FullNameRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan
	for _, loc := range r.re.FindAllStringIndex(norm.Normalized, -1) {
		text := norm.Normalized[loc[0]:loc[1]]
		words := strings.Fields(text)
		if len(words) < 2 || len(words) > 3 {
			continue
		}

		// Verify word boundaries manually (RE2 \b is ASCII-only).
		if !isWordStart(norm.Normalized, loc[0]) || !isWordEnd(norm.Normalized, loc[1]) {
			continue
		}

		// Skip known famous persons.
		if r.known.Contains(text) {
			continue
		}

		score := r.scoreName(words)
		if score <= 0 {
			continue
		}

		oStart, oEnd := norm.MapSpan(norm.ByteToRune(loc[0]), norm.ByteToRune(loc[1]))
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.FULL_NAME,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   score,
			Sources: []entity.Source{entity.SourceRegex, entity.SourceDictionary},
			Reason:  "name-heuristic",
		})
	}
	return spans
}

// isWordStart reports whether pos is the start of a word (prev not a letter,
// current is a letter).
func isWordStart(s string, pos int) bool {
	runes := []rune(s)
	idx := len([]rune(s[:pos]))
	before := idx > 0 && isLetter(runes[idx-1])
	after := idx < len(runes) && isLetter(runes[idx])
	return !before && after
}

// isWordEnd reports whether pos is the end of a word (prev is a letter,
// current not a letter).
func isWordEnd(s string, pos int) bool {
	runes := []rune(s)
	idx := len([]rune(s[:pos]))
	before := idx > 0 && isLetter(runes[idx-1])
	after := idx < len(runes) && isLetter(runes[idx])
	return before && !after
}

// isLetter reports whether r is a Cyrillic or latin letter.
func isLetter(r rune) bool {
	return (r >= 'а' && r <= 'я') || r == 'ё' ||
		(r >= 'a' && r <= 'z') || (r >= 'А' && r <= 'Я') || r == 'Ё' ||
		(r >= 'A' && r <= 'Z')
}

// scoreName scores a name candidate. Returns 0 if it is not a plausible name.
func (r *FullNameRecognizer) scoreName(words []string) float64 {
	score := 0.0
	matched := 0

	for _, w := range words {
		lower := strings.ToLower(w)
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

	// Require at least one strong signal (dictionary match or patronymic).
	if matched == 0 {
		return 0
	}

	// A 3-word name with a patronymic is a strong signal.
	if len(words) == 3 && isPatronymic(strings.ToLower(words[1])) {
		score += 0.2
	}

	// Normalize to [0,1].
	if score > 1 {
		score = 1
	}
	return score
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
