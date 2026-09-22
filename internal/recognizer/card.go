package recognizer

import (
	"regexp"
	"strings"

	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/normalize"
)

// CardNumberRecognizer detects bank card numbers using format + Luhn.
type CardNumberRecognizer struct {
	re *regexp.Regexp
}

// NewCardNumberRecognizer builds a card number recognizer.
func NewCardNumberRecognizer() *CardNumberRecognizer {
	return &CardNumberRecognizer{
		re: regexp.MustCompile(`\b\d{4}[\s\-]?\d{4}[\s\-]?\d{4}[\s\-]?\d{4}\b`),
	}
}

// Type returns the entity type.
func (r *CardNumberRecognizer) Type() entity.Type { return entity.CARD_NUMBER }

// Recognize finds card number candidates and validates with Luhn.
func (r *CardNumberRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan
	for _, loc := range r.re.FindAllStringIndex(norm.Normalized, -1) {
		digits := strings.ReplaceAll(norm.Normalized[loc[0]:loc[1]], " ", "")
		digits = strings.ReplaceAll(digits, "-", "")
		if len(digits) != 16 || !Luhn(digits) {
			continue
		}
		oStart, oEnd := norm.MapSpan(norm.ByteToRune(loc[0]), norm.ByteToRune(loc[1]))
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.CARD_NUMBER,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   0.4,
			Sources: []entity.Source{entity.SourceRegex, entity.SourceChecksum},
			Reason:  "card-luhn",
		})
	}
	return spans
}

// CvvRecognizer detects CVV codes only in banking context.
type CvvRecognizer struct {
	re *regexp.Regexp
}

// NewCvvRecognizer builds a CVV recognizer.
func NewCvvRecognizer() *CvvRecognizer {
	return &CvvRecognizer{
		re: regexp.MustCompile(`\b\d{3}\b`),
	}
}

// Type returns the entity type.
func (r *CvvRecognizer) Type() entity.Type { return entity.CVV }

// Recognize finds CVV candidates. The context scorer decides whether the
// surrounding text is banking-related; a bare 3-digit number is not CVV.
func (r *CvvRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan
	for _, loc := range r.re.FindAllStringIndex(norm.Normalized, -1) {
		oStart, oEnd := norm.MapSpan(norm.ByteToRune(loc[0]), norm.ByteToRune(loc[1]))
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.CVV,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   0.3,
			Sources: []entity.Source{entity.SourceRegex},
			Reason:  "regex:cvv-candidate",
		})
	}
	return spans
}

// PinRecognizer detects PIN codes only in banking context.
type PinRecognizer struct {
	re *regexp.Regexp
}

// NewPinRecognizer builds a PIN recognizer.
func NewPinRecognizer() *PinRecognizer {
	return &PinRecognizer{
		re: regexp.MustCompile(`\b\d{4}\b`),
	}
}

// Type returns the entity type.
func (r *PinRecognizer) Type() entity.Type { return entity.PIN }

// Recognize finds PIN candidates. Context decides whether it is a PIN.
func (r *PinRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan
	for _, loc := range r.re.FindAllStringIndex(norm.Normalized, -1) {
		oStart, oEnd := norm.MapSpan(norm.ByteToRune(loc[0]), norm.ByteToRune(loc[1]))
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.PIN,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   0.3,
			Sources: []entity.Source{entity.SourceRegex},
			Reason:  "regex:pin-candidate",
		})
	}
	return spans
}

// CardholderRecognizer detects cardholder names in banking context.
type CardholderRecognizer struct {
	re *regexp.Regexp
}

// NewCardholderRecognizer builds a cardholder recognizer.
func NewCardholderRecognizer() *CardholderRecognizer {
	return &CardholderRecognizer{
		re: regexp.MustCompile(`(?i)\b[a-z][a-z\s]{2,40}\b`),
	}
}

// Type returns the entity type.
func (r *CardholderRecognizer) Type() entity.Type { return entity.CARDHOLDER_NAME }

// Recognize finds cardholder candidates (latin names).
func (r *CardholderRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan
	for _, loc := range r.re.FindAllStringIndex(norm.Normalized, -1) {
		text := norm.Normalized[loc[0]:loc[1]]
		// Require at least two words to be a plausible name.
		if strings.Count(strings.TrimSpace(text), " ") < 1 {
			continue
		}
		oStart, oEnd := norm.MapSpan(norm.ByteToRune(loc[0]), norm.ByteToRune(loc[1]))
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.CARDHOLDER_NAME,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   0.3,
			Sources: []entity.Source{entity.SourceRegex},
			Reason:  "regex:cardholder",
		})
	}
	return spans
}
