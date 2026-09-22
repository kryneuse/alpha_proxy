package recognizer

import (
	"regexp"
	"strings"

	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/normalize"
)

// EmailRecognizer detects email addresses.
type EmailRecognizer struct {
	re *regexp.Regexp
}

// NewEmailRecognizer builds an email recognizer.
func NewEmailRecognizer() *EmailRecognizer {
	return &EmailRecognizer{
		re: regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
	}
}

// Type returns the entity type.
func (r *EmailRecognizer) Type() entity.Type { return entity.EMAIL }

// Recognize finds email candidates.
func (r *EmailRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan
	for _, loc := range r.re.FindAllStringIndex(norm.Normalized, -1) {
		oStart, oEnd := norm.MapSpan(norm.ByteToRune(loc[0]), norm.ByteToRune(loc[1]))
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.EMAIL,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   0.6,
			Sources: []entity.Source{entity.SourceRegex},
			Reason:  "regex:email",
		})
	}
	return spans
}

// PhoneRecognizer detects phone numbers in various formats.
type PhoneRecognizer struct {
	re *regexp.Regexp
}

// NewPhoneRecognizer builds a phone recognizer.
func NewPhoneRecognizer() *PhoneRecognizer {
	return &PhoneRecognizer{
		re: regexp.MustCompile(`(?:\+7|8|7)[\s\-]?(?:\(\d{3}\)|\d{3})[\s\-]?\d{3}[\s\-]?\d{2}[\s\-]?\d{2}`),
	}
}

// Type returns the entity type.
func (r *PhoneRecognizer) Type() entity.Type { return entity.PHONE }

// Recognize finds phone candidates.
func (r *PhoneRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan
	for _, loc := range r.re.FindAllStringIndex(norm.Normalized, -1) {
		oStart, oEnd := norm.MapSpan(norm.ByteToRune(loc[0]), norm.ByteToRune(loc[1]))
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.PHONE,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   0.6,
			Sources: []entity.Source{entity.SourceRegex},
			Reason:  "regex:phone",
		})
	}
	return spans
}

// InnRecognizer detects INN candidates and validates control digits.
type InnRecognizer struct {
	re *regexp.Regexp
}

// NewInnRecognizer builds an INN recognizer.
func NewInnRecognizer() *InnRecognizer {
	return &InnRecognizer{
		re: regexp.MustCompile(`\b\d{10}\b|\b\d{12}\b`),
	}
}

// Type returns the entity type.
func (r *InnRecognizer) Type() entity.Type { return entity.INN }

// Recognize finds INN candidates and validates them.
func (r *InnRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan
	for _, loc := range r.re.FindAllStringIndex(norm.Normalized, -1) {
		digits := strings.ReplaceAll(norm.Normalized[loc[0]:loc[1]], " ", "")
		valid := false
		if len(digits) == 10 {
			valid = Inn10(digits)
		} else if len(digits) == 12 {
			valid = Inn12(digits)
		}
		if !valid {
			continue
		}
		oStart, oEnd := norm.MapSpan(norm.ByteToRune(loc[0]), norm.ByteToRune(loc[1]))
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.INN,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   0.9,
			Sources: []entity.Source{entity.SourceRegex, entity.SourceChecksum},
			Reason:  "inn-checksum",
		})
	}
	return spans
}
