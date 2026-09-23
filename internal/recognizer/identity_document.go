package recognizer

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/normalize"
)

// IdentityDocumentRecognizer detects identity documents other than the Russian
// passport: foreign passport RF, birth certificate, military ID, and temporary
// identity document (форма 2П).
//
// The principle: format alone is insufficient when it easily collides with
// ordinary numbers. A candidate is emitted with a weak base score and only
// becomes a strong entity when document context is present (applied by the
// context scorer). Doubtful documents are left to the gate/ML layer rather than
// treating arbitrary numbers as identity documents.
type IdentityDocumentRecognizer struct {
	foreignPassportRe  *regexp.Regexp
	foreignSplitRe     *regexp.Regexp
	birthCertificateRe *regexp.Regexp
	birthSplitRe       *regexp.Regexp
	militaryIDRe       *regexp.Regexp
	temporaryIDRe      *regexp.Regexp
}

// NewIdentityDocumentRecognizer builds the identity document recognizer.
func NewIdentityDocumentRecognizer() *IdentityDocumentRecognizer {
	return &IdentityDocumentRecognizer{
		// Foreign passport RF: series (2 digits) + number (7 digits).
		// "62 1234567", "62 № 1234567", "621234567", "62-1234567".
		foreignPassportRe: regexp.MustCompile(`\b\d{2}\s?[-–]?\s?№?\s?\d{7}\b`),
		// Split form: "серия 62 номер 1234567", "серия загранпаспорта 65 номер 4567890".
		foreignSplitRe: regexp.MustCompile(`серия\s+(?:[а-яё]+\s+)?\d{2}\s*,?\s*номер\s+\d{7}`),
		// Birth certificate: Roman series + exactly 2 Cyrillic letters + 6-digit
		// number. "II-МЮ № 123456", "I-АБ 123456", "III-ВГ №123456".
		// Normalized text is lowercased, so Cyrillic is matched lowercase.
		birthCertificateRe: regexp.MustCompile(`[ivxlc]+[-–]?[а-яё]{2}\s?№?\s?\d{6}`),
		// Split form: "серия II-МЮ, номер 123456".
		birthSplitRe: regexp.MustCompile(`серия\s+[ivxlc]+[-–]?[а-яё]{2}\s*,?\s*номер\s+\d{6}`),
		// Military ID: 2 Cyrillic letters + 6-7 digits. "ГД № 1234567", "ГД N 1234567".
		militaryIDRe: regexp.MustCompile(`[а-яё]{2}\s?[№n]?\s?\d{6,7}`),
		// Temporary ID (форма 2П): 8 digits, 12 digits, or 2 Cyrillic letters
		// + 8 digits. Only near strong context. 12 digits must be tried before
		// 8 so a 12-digit number is not truncated to its first 8 digits.
		temporaryIDRe: regexp.MustCompile(`\d{12}|\d{8}|[а-яё]{2}\s?\d{8}`),
	}
}

// Type returns the entity type.
func (r *IdentityDocumentRecognizer) Type() entity.Type { return entity.IDENTITY_DOCUMENT }

// Recognize finds identity document candidates.
func (r *IdentityDocumentRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan

	// Foreign passport RF.
	for _, loc := range r.foreignPassportRe.FindAllStringIndex(norm.Normalized, -1) {
		if !r.tokenBoundary(norm.Normalized, loc) {
			continue
		}
		spans = append(spans, r.span(norm, loc, entity.ForeignPassportRF, 0.4, "regex:foreign-passport"))
	}
	// Split form requires foreign-passport context. If the matched text itself
	// contains a foreign-passport keyword (e.g. "серия загранпаспорта 62 номер
	// 1234567"), emit a strong candidate. Otherwise emit a weak candidate that
	// only passes the resolver threshold when foreign-passport context precedes
	// it (applied by the context scorer).
	for _, loc := range r.foreignSplitRe.FindAllStringIndex(norm.Normalized, -1) {
		if !r.tokenBoundary(norm.Normalized, loc) {
			continue
		}
		score := 0.4
		reason := "regex:foreign-passport-split"
		if r.hasForeignPassportContext(norm.Normalized[loc[0]:loc[1]]) {
			score = 0.7
			reason = "regex:foreign-passport-split-context"
		}
		spans = append(spans, r.span(norm, loc, entity.ForeignPassportRF, score, reason))
	}

	// Birth certificate.
	for _, loc := range r.birthCertificateRe.FindAllStringIndex(norm.Normalized, -1) {
		if !r.tokenBoundary(norm.Normalized, loc) {
			continue
		}
		spans = append(spans, r.span(norm, loc, entity.BirthCertificate, 0.4, "regex:birth-certificate"))
	}
	for _, loc := range r.birthSplitRe.FindAllStringIndex(norm.Normalized, -1) {
		if !r.tokenBoundary(norm.Normalized, loc) {
			continue
		}
		spans = append(spans, r.span(norm, loc, entity.BirthCertificate, 0.4, "regex:birth-certificate-split"))
	}

	// Military ID.
	for _, loc := range r.militaryIDRe.FindAllStringIndex(norm.Normalized, -1) {
		if !r.tokenBoundary(norm.Normalized, loc) {
			continue
		}
		spans = append(spans, r.span(norm, loc, entity.MilitaryID, 0.4, "regex:military-id"))
	}

	// Temporary ID (форма 2П).
	for _, loc := range r.temporaryIDRe.FindAllStringIndex(norm.Normalized, -1) {
		if !r.tokenBoundary(norm.Normalized, loc) {
			continue
		}
		spans = append(spans, r.span(norm, loc, entity.TemporaryIDRF, 0.4, "regex:temporary-id"))
	}

	return spans
}

// hasForeignPassportContext reports whether the given (normalized) text
// contains a foreign-passport keyword. Used to decide whether a split-form
// candidate is a strong foreign passport or a generic серия/номер that needs
// external context.
func (r *IdentityDocumentRecognizer) hasForeignPassportContext(s string) bool {
	lower := strings.ToLower(s)
	for _, kw := range []string{
		"загранпаспорт", "загранпаспорта", "заграничный паспорт",
		"заграничного паспорта", "паспорт для выезда за границу",
		"паспорта для выезда за границу", "серия загранпаспорта",
		"номер загранпаспорта", "загран паспорт", "загран паспорта",
	} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// tokenBoundary reports whether the match at [start,end) is a standalone token:
// the preceding and following runes are not Cyrillic letters or digits.
// This is needed because Go's RE2 \b is ASCII-only and does not treat Cyrillic
// letters as word characters. The check is Unicode-safe: it decodes the rune
// immediately before/after the byte range rather than inspecting single bytes.
func (r *IdentityDocumentRecognizer) tokenBoundary(s string, loc []int) bool {
	if loc[0] > 0 {
		prev, _ := utf8.DecodeLastRuneInString(s[:loc[0]])
		if isWordChar(prev) {
			return false
		}
	}
	if loc[1] < len(s) {
		next, _ := utf8.DecodeRuneInString(s[loc[1]:])
		if isWordChar(next) {
			return false
		}
	}
	return true
}

// isWordChar reports whether r is a letter or digit (any script). Used by
// tokenBoundary to ensure a document number is a standalone token and not part
// of a longer word (Cyrillic or Latin).
func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func (r *IdentityDocumentRecognizer) span(norm *normalize.Text, loc []int, subtype entity.DocumentSubtype, score float64, reason string) entity.CandidateSpan {
	oStart, oEnd := norm.MapSpan(norm.ByteToRune(loc[0]), norm.ByteToRune(loc[1]))
	return entity.CandidateSpan{
		Type:    entity.IDENTITY_DOCUMENT,
		Subtype: subtype,
		Text:    norm.Original[oStart:oEnd],
		Start:   oStart,
		End:     oEnd,
		Score:   score,
		Sources: []entity.Source{entity.SourceRegex, entity.SourceFormat},
		Reason:  reason,
	}
}
