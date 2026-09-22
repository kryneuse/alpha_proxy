package recognizer

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/alpha-proxy/rule-engine/internal/entity"
	"github.com/alpha-proxy/rule-engine/internal/normalize"
)

// DateRecognizer detects dates in numeric and word forms. The context scorer
// decides whether a date is a BIRTH_DATE or PASSPORT_ISSUE_DATE.
type DateRecognizer struct {
	numericRe *regexp.Regexp
	wordRe    *regexp.Regexp
}

// NewDateRecognizer builds a date recognizer.
func NewDateRecognizer() *DateRecognizer {
	return &DateRecognizer{
		numericRe: regexp.MustCompile(`\b\d{1,2}[./\-]\d{1,2}[./\-]\d{2,4}\b`),
		wordRe:    regexp.MustCompile(`\b\d{1,2}\s+(января|февраля|марта|апреля|мая|июня|июля|августа|сентября|октября|ноября|декабря)\s+\d{4}\b`),
	}
}

// Type returns the entity type.
func (r *DateRecognizer) Type() entity.Type { return entity.BIRTH_DATE }

// Recognize finds date candidates. Both BIRTH_DATE and PASSPORT_ISSUE_DATE
// are emitted; the resolver/context decides which applies.
func (r *DateRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan

	for _, loc := range r.numericRe.FindAllStringIndex(norm.Normalized, -1) {
		text := norm.Normalized[loc[0]:loc[1]]
		if !validDate(text) {
			continue
		}
		oStart, oEnd := norm.MapSpan(norm.ByteToRune(loc[0]), norm.ByteToRune(loc[1]))
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.BIRTH_DATE,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   0.4,
			Sources: []entity.Source{entity.SourceRegex, entity.SourceFormat},
			Reason:  "regex:date",
		})
	}

	for _, loc := range r.wordRe.FindAllStringIndex(norm.Normalized, -1) {
		oStart, oEnd := norm.MapSpan(norm.ByteToRune(loc[0]), norm.ByteToRune(loc[1]))
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.BIRTH_DATE,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   0.45,
			Sources: []entity.Source{entity.SourceRegex, entity.SourceFormat},
			Reason:  "regex:date-words",
		})
	}

	return spans
}

// validDate checks that a numeric date is plausible (day 1-31, month 1-12).
func validDate(s string) bool {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '.' || r == '/' || r == '-'
	})
	if len(parts) != 3 {
		return false
	}
	day, err1 := strconv.Atoi(parts[0])
	month, err2 := strconv.Atoi(parts[1])
	year, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return false
	}
	if day < 1 || day > 31 || month < 1 || month > 12 {
		return false
	}
	if year < 1900 || year > 2100 {
		return false
	}
	return true
}
