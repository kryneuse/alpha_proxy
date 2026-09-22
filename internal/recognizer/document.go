package recognizer

import (
	"regexp"
	"strings"

	"github.com/alpha-proxy/rule-engine/internal/entity"
	"github.com/alpha-proxy/rule-engine/internal/normalize"
)

// PassportRecognizer detects Russian passport series+number.
type PassportRecognizer struct {
	re      *regexp.Regexp
	splitRe *regexp.Regexp
}

// NewPassportRecognizer builds a passport recognizer.
func NewPassportRecognizer() *PassportRecognizer {
	return &PassportRecognizer{
		re:      regexp.MustCompile(`\b\d{4}\s?\d{6}\b`),
		splitRe: regexp.MustCompile(`серия\s+\d{2}\s?\d{2}\s+номер\s+\d{6}`),
	}
}

// Type returns the entity type.
func (r *PassportRecognizer) Type() entity.Type { return entity.PASSPORT }

// Recognize finds passport candidates.
func (r *PassportRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan
	for _, loc := range r.re.FindAllStringIndex(norm.Normalized, -1) {
		oStart, oEnd := norm.MapSpan(norm.ByteToRune(loc[0]), norm.ByteToRune(loc[1]))
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.PASSPORT,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   0.7,
			Sources: []entity.Source{entity.SourceRegex, entity.SourceFormat},
			Reason:  "regex:passport",
		})
	}
	for _, loc := range r.splitRe.FindAllStringIndex(norm.Normalized, -1) {
		oStart, oEnd := norm.MapSpan(norm.ByteToRune(loc[0]), norm.ByteToRune(loc[1]))
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.PASSPORT,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   0.75,
			Sources: []entity.Source{entity.SourceRegex, entity.SourceFormat},
			Reason:  "regex:passport-split",
		})
	}
	return spans
}

// DepartmentCodeRecognizer detects Russian department codes XXX-XXX.
type DepartmentCodeRecognizer struct {
	re *regexp.Regexp
}

// NewDepartmentCodeRecognizer builds a department code recognizer.
func NewDepartmentCodeRecognizer() *DepartmentCodeRecognizer {
	return &DepartmentCodeRecognizer{
		re: regexp.MustCompile(`\b\d{3}[-–]\d{3}\b`),
	}
}

// Type returns the entity type.
func (r *DepartmentCodeRecognizer) Type() entity.Type { return entity.DEPARTMENT_CODE }

// Recognize finds department code candidates. Context decides whether it is
// a passport department code.
func (r *DepartmentCodeRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan
	for _, loc := range r.re.FindAllStringIndex(norm.Normalized, -1) {
		oStart, oEnd := norm.MapSpan(norm.ByteToRune(loc[0]), norm.ByteToRune(loc[1]))
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.DEPARTMENT_CODE,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   0.4,
			Sources: []entity.Source{entity.SourceRegex, entity.SourceFormat},
			Reason:  "regex:department-code",
		})
	}
	return spans
}

// DriverLicenseRecognizer detects Russian driver license numbers.
type DriverLicenseRecognizer struct {
	re *regexp.Regexp
}

// NewDriverLicenseRecognizer builds a driver license recognizer.
func NewDriverLicenseRecognizer() *DriverLicenseRecognizer {
	return &DriverLicenseRecognizer{
		re: regexp.MustCompile(`\b\d{4}\s?\d{6}\b`),
	}
}

// Type returns the entity type.
func (r *DriverLicenseRecognizer) Type() entity.Type { return entity.DRIVER_LICENSE }

// Recognize finds driver license candidates.
func (r *DriverLicenseRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan
	for _, loc := range r.re.FindAllStringIndex(norm.Normalized, -1) {
		oStart, oEnd := norm.MapSpan(norm.ByteToRune(loc[0]), norm.ByteToRune(loc[1]))
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.DRIVER_LICENSE,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   0.5,
			Sources: []entity.Source{entity.SourceRegex, entity.SourceFormat},
			Reason:  "regex:driver-license",
		})
	}
	return spans
}

// PassportIssuerRecognizer detects passport issuer text.
type PassportIssuerRecognizer struct {
	re *regexp.Regexp
}

// NewPassportIssuerRecognizer builds a passport issuer recognizer.
func NewPassportIssuerRecognizer() *PassportIssuerRecognizer {
	return &PassportIssuerRecognizer{
		re: regexp.MustCompile(`(?:выдан|выдано|орган, выдавший|кем выдан)[:\s]+([а-яёa-z\s.\-]{5,80}?)(?:,|$)`),
	}
}

// Type returns the entity type.
func (r *PassportIssuerRecognizer) Type() entity.Type { return entity.PASSPORT_ISSUER }

// Recognize finds passport issuer candidates.
func (r *PassportIssuerRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan
	for _, loc := range r.re.FindAllStringIndex(norm.Normalized, -1) {
		oStart, oEnd := norm.MapSpan(norm.ByteToRune(loc[0]), norm.ByteToRune(loc[1]))
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.PASSPORT_ISSUER,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   0.7,
			Sources: []entity.Source{entity.SourceRegex, entity.SourceContext},
			Reason:  "regex:passport-issuer",
		})
	}
	return spans
}

// normalizeIssuerText trims leading keywords from issuer text.
func normalizeIssuerText(s string) string {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	for _, kw := range []string{"выдан:", "выдано:", "кем выдан:", "орган, выдавший:", "выдан ", "выдано "} {
		if strings.HasPrefix(lower, kw) {
			s = strings.TrimSpace(s[len(kw):])
			break
		}
	}
	return s
}
