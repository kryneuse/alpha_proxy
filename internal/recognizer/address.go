package recognizer

import (
	"regexp"
	"strings"

	"github.com/kryneuse/alpha_proxy/internal/dict"
	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/normalize"
)

// AddressRecognizer detects addresses using context and address structure.
type AddressRecognizer struct {
	structure *dict.Dict
	re        *regexp.Regexp
}

// NewAddressRecognizer builds an address recognizer.
func NewAddressRecognizer(structure *dict.Dict) *AddressRecognizer {
	if structure == nil {
		structure = dict.AddressStructureKeywords
	}
	return &AddressRecognizer{
		structure: structure,
		re:        regexp.MustCompile(`(?i)(?:адрес|адрес проживания|адрес регистрации|место жительства|место регистрации|проживает по адресу|живет по адресу|живёт по адресу|зарегистрирован по адресу|зарегистрирована по адресу|прописан по адресу|прописана по адресу)[:\s]+([^\n]{5,120})`),
	}
}

// Type returns the entity type.
func (r *AddressRecognizer) Type() entity.Type { return entity.ADDRESS }

// Recognize finds address candidates.
func (r *AddressRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan
	for _, loc := range r.re.FindAllStringIndex(norm.Normalized, -1) {
		full := norm.Normalized[loc[0]:loc[1]]
		addr := extractAddress(full)
		if !r.hasStructure(addr) {
			continue
		}
		// Locate the address substring within the normalized match.
		addrIdx := strings.Index(full, addr)
		if addrIdx < 0 {
			continue
		}
		startByte := loc[0] + addrIdx
		endByte := startByte + len(addr)
		oStart, oEnd := norm.MapSpan(norm.ByteToRune(startByte), norm.ByteToRune(endByte))
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

// extractAddress returns the address portion after the leading keyword,
// trimmed at the first field separator keyword.
func extractAddress(s string) string {
	idx := strings.Index(s, ":")
	if idx >= 0 {
		s = s[idx+1:]
	} else {
		// No colon; strip the keyword.
		for _, kw := range []string{"проживает по адресу", "зарегистрирован по адресу", "зарегистрирована по адресу", "прописан по адресу", "прописана по адресу", "место жительства", "место регистрации", "адрес проживания", "адрес регистрации", "адрес"} {
			if strings.HasPrefix(s, kw) {
				s = s[len(kw):]
				break
			}
		}
	}
	s = strings.TrimSpace(s)

	// Trim at field separator keywords.
	lower := strings.ToLower(s)
	for _, sep := range []string{", гражданство", ", граждан", ", тел", ", телефон", ", email", ", e-mail", ", инн", ", паспорт", ", дата", ", фио", ", ф.и.о", ", код", ", номер карты", ", cvv", ", пин"} {
		if i := strings.Index(lower, sep); i >= 0 {
			s = s[:i]
			break
		}
	}
	return strings.TrimSpace(s)
}

// hasStructure reports whether the address contains structural keywords.
func (r *AddressRecognizer) hasStructure(addr string) bool {
	words := strings.Fields(addr)
	for _, w := range words {
		if r.structure.Contains(w) {
			return true
		}
	}
	// Also check for a postal index (6 digits).
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
