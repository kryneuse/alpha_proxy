package recognizer

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/normalize"
)

// DateRecognizer detects dates in numeric and word forms. The context scorer
// decides whether a date is a BIRTH_DATE or PASSPORT_ISSUE_DATE.
type DateRecognizer struct {
	numericRe *regexp.Regexp
	isoRe     *regexp.Regexp
	wordRe    *regexp.Regexp
	yearRe    *regexp.Regexp
}

// NewDateRecognizer builds a date recognizer.
func NewDateRecognizer() *DateRecognizer {
	return &DateRecognizer{
		numericRe: regexp.MustCompile(`\b\d{1,2}[./\-]\d{1,2}[./\-]\d{2,4}\b`),
		isoRe:     regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`),
		wordRe:    regexp.MustCompile(`\b\d{1,2}\s+(января|февраля|марта|апреля|мая|июня|июля|августа|сентября|октября|ноября|декабря)\s+\d{4}\b`),
		yearRe:    regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`),
	}
}

// Type returns the entity type.
func (r *DateRecognizer) Type() entity.Type { return entity.BIRTH_DATE }

// Recognize finds date candidates. Both BIRTH_DATE and PASSPORT_ISSUE_DATE
// are emitted; the resolver/context decides which applies.
func (r *DateRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan

	// Collect full-date spans so a bare year inside a full date is not
	// emitted separately.
	var fullDateSpans [][2]int // normalized byte [start,end)

	for _, loc := range r.numericRe.FindAllStringIndex(norm.Normalized, -1) {
		text := norm.Normalized[loc[0]:loc[1]]
		// Mark the span as occupied so a bare year inside it is not emitted,
		// even if the date itself is invalid.
		fullDateSpans = append(fullDateSpans, [2]int{loc[0], loc[1]})
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

	for _, loc := range r.isoRe.FindAllStringIndex(norm.Normalized, -1) {
		text := norm.Normalized[loc[0]:loc[1]]
		fullDateSpans = append(fullDateSpans, [2]int{loc[0], loc[1]})
		if !validISODate(text) {
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
			Reason:  "regex:date-iso",
		})
	}

	for _, loc := range r.wordRe.FindAllStringIndex(norm.Normalized, -1) {
		fullDateSpans = append(fullDateSpans, [2]int{loc[0], loc[1]})
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

	// A bare year is only a birth date when birth context is present; the
	// context scorer boosts it above the threshold. Skip years that are part
	// of a full date.
	for _, loc := range r.yearRe.FindAllStringIndex(norm.Normalized, -1) {
		if overlapsAny(loc[0], loc[1], fullDateSpans) {
			continue
		}
		oStart, oEnd := norm.MapSpan(norm.ByteToRune(loc[0]), norm.ByteToRune(loc[1]))
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.BIRTH_DATE,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   0.3,
			Sources: []entity.Source{entity.SourceRegex, entity.SourceFormat},
			Reason:  "regex:year",
		})
	}

	return spans
}

// overlapsAny reports whether the byte range [start,end) overlaps any span.
func overlapsAny(start, end int, spans [][2]int) bool {
	for _, s := range spans {
		if start < s[1] && s[0] < end {
			return true
		}
	}
	return false
}

// validDate checks that a numeric date is a real calendar date.
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
	if month < 1 || month > 12 || year < 1900 || year > 2100 {
		return false
	}
	daysInMonth := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	if month == 2 && isLeapYear(year) {
		daysInMonth[1] = 29
	}
	return day >= 1 && day <= daysInMonth[month-1]
}

// validISODate checks that an ISO date (YYYY-MM-DD) is a real calendar date.
func validISODate(s string) bool {
	parts := strings.Split(s, "-")
	if len(parts) != 3 {
		return false
	}
	year, err1 := strconv.Atoi(parts[0])
	month, err2 := strconv.Atoi(parts[1])
	day, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return false
	}
	if month < 1 || month > 12 || year < 1900 || year > 2100 {
		return false
	}
	daysInMonth := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	if month == 2 && isLeapYear(year) {
		daysInMonth[1] = 29
	}
	return day >= 1 && day <= daysInMonth[month-1]
}

// isLeapYear reports whether year is a leap year.
func isLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}
