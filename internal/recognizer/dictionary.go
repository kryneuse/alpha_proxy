package recognizer

import (
	"regexp"
	"strings"

	"github.com/kryneuse/alpha_proxy/internal/dict"
	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/normalize"
)

// CitizenshipRecognizer detects citizenship using a country dictionary.
type CitizenshipRecognizer struct {
	countries *dict.Dict
}

// NewCitizenshipRecognizer builds a citizenship recognizer.
func NewCitizenshipRecognizer(countries *dict.Dict) *CitizenshipRecognizer {
	if countries == nil {
		countries = dict.Countries
	}
	return &CitizenshipRecognizer{countries: countries}
}

// Type returns the entity type.
func (r *CitizenshipRecognizer) Type() entity.Type { return entity.CITIZENSHIP }

// Recognize finds citizenship candidates. A country name is only a strong
// candidate when citizenship context is present; the context scorer boosts it
// above the threshold. The base score is kept below the threshold so a bare
// country mention (e.g. "Россия — крупнейшая страна") is not emitted.
func (r *CitizenshipRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan
	words := strings.Fields(norm.Normalized)
	// Compute the byte offset of each token by scanning the normalized text.
	offsets := tokenOffsets(norm.Normalized, words)
	for i := 0; i < len(words); i++ {
		// Try multi-word country names (up to 3 words).
		for n := 3; n >= 1; n-- {
			if i+n > len(words) {
				continue
			}
			phrase := strings.Join(words[i:i+n], " ")
			clean := stripPunct(phrase)
			if !r.matchCountry(clean) {
				continue
			}
			start := offsets[i]
			// Trim trailing punctuation from the emitted span.
			end := start + len(clean)
			oStart, oEnd := norm.MapSpan(norm.ByteToRune(start), norm.ByteToRune(end))
			spans = append(spans, entity.CandidateSpan{
				Type:    entity.CITIZENSHIP,
				Text:    norm.Original[oStart:oEnd],
				Start:   oStart,
				End:     oEnd,
				Score:   0.3,
				Sources: []entity.Source{entity.SourceDictionary},
				Reason:  "dict:country",
			})
			i += n - 1
			break
		}
	}
	return spans
}

// tokenOffsets returns the byte offset of each word in the normalized string.
func tokenOffsets(s string, words []string) []int {
	offsets := make([]int, len(words))
	pos := 0
	for i, w := range words {
		idx := strings.Index(s[pos:], w)
		if idx < 0 {
			offsets[i] = pos
			continue
		}
		offsets[i] = pos + idx
		pos = offsets[i] + len(w)
	}
	return offsets
}

// matchCountry reports whether the phrase matches a country name, tolerating
// common Russian case endings (e.g. "Казахстана" -> "Казахстан",
// "Армении" -> "Армения").
func (r *CitizenshipRecognizer) matchCountry(phrase string) bool {
	if r.countries.Contains(phrase) {
		return true
	}
	words := strings.Fields(phrase)
	if len(words) == 0 {
		return false
	}
	last := words[len(words)-1]

	// Direct case-ending replacements for feminine/neuter countries.
	replacements := []struct{ from, to string }{
		{"ии", "ия"},  // Армении -> Армения, Германии -> Германия
		{"ией", "ия"}, // Арменией -> Армения
		{"ию", "ия"},  // Армению -> Армения
		{"е", "а"},    // Грузии -> Грузия (handled by ии), Канаде -> Канада
		{"е", "я"},    // Турции -> Турция
	}
	for _, rep := range replacements {
		if len(last) > len(rep.from)+2 && strings.HasSuffix(last, rep.from) {
			candidate := last[:len(last)-len(rep.from)] + rep.to
			words[len(words)-1] = candidate
			if r.countries.Contains(strings.Join(words, " ")) {
				return true
			}
		}
	}

	// Try stripping a trailing case ending from the last word.
	suffixes := []string{"ов", "ев", "ин", "ын", "ии", "ой", "ий", "ый", "а", "я", "е", "у", "ом", "ем"}
	for _, suffix := range suffixes {
		if len(last) <= len(suffix)+2 || !strings.HasSuffix(last, suffix) {
			continue
		}
		stem := last[:len(last)-len(suffix)]
		// Try the bare stem.
		words[len(words)-1] = stem
		if r.countries.Contains(strings.Join(words, " ")) {
			return true
		}
		// Try re-appending a nominative ending (feminine "а"/"я").
		for _, nom := range []string{"а", "я"} {
			words[len(words)-1] = stem + nom
			if r.countries.Contains(strings.Join(words, " ")) {
				return true
			}
		}
	}
	return false
}

// stripPunct removes trailing punctuation from a phrase for dictionary lookup.
func stripPunct(s string) string {
	return strings.Trim(s, ".,;:!?()[]{}«»\"'")
}

// stripLeadingPreposition removes a leading preposition "в " (and "в") from a
// place phrase so the preposition is not part of the entity span. It returns
// the stripped phrase and the adjusted byte start offset.
func stripLeadingPreposition(s string, startByte int) (string, int) {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "в ") {
		rest := strings.TrimSpace(s[2:])
		// Find where the trimmed rest begins within the original phrase.
		idx := strings.Index(s, rest)
		if idx < 0 {
			return rest, startByte + 2
		}
		return rest, startByte + idx
	}
	if lower == "в" {
		return "", startByte
	}
	return s, startByte
}

// BirthPlaceRecognizer detects birth places using context constructions.
type BirthPlaceRecognizer struct {
	re *regexp.Regexp
}

// NewBirthPlaceRecognizer builds a birth place recognizer.
func NewBirthPlaceRecognizer() *BirthPlaceRecognizer {
	return &BirthPlaceRecognizer{
		// Capture the place after the birth keyword, optionally skipping a
		// "в YYYY году" year phrase. The place is a sequence of letters,
		// spaces, dots and hyphens.
		re: regexp.MustCompile(`(?:место рождения|родился|родилась|уроженец|уроженка|родом из)[:\s]+(?:в\s+\d{4}\s+году\s+)?([а-яёa-z\s.,\-]{2,60})`),
	}
}

// Type returns the entity type.
func (r *BirthPlaceRecognizer) Type() entity.Type { return entity.BIRTH_PLACE }

// Recognize finds birth place candidates. The emitted span covers only the
// place, trimmed at a comma followed by a field keyword.
func (r *BirthPlaceRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan
	for _, loc := range r.re.FindAllStringIndex(norm.Normalized, -1) {
		sub := r.re.FindStringSubmatchIndex(norm.Normalized[loc[0]:loc[1]])
		if len(sub) < 4 || sub[2] < 0 || sub[3] < 0 {
			continue
		}
		startByte := loc[0] + sub[2]
		endByte := loc[0] + sub[3]
		place := norm.Normalized[startByte:endByte]
		place = trimFieldSeparator(place)
		// A bare preposition is not a place.
		place = strings.TrimSpace(place)
		// Strip a leading preposition "в " so it is not part of the span.
		place, startByte = stripLeadingPreposition(place, startByte)
		if place == "" {
			continue
		}
		// Recompute end after trimming.
		trimmedEnd := startByte + len(place)
		oStart, oEnd := norm.MapSpan(norm.ByteToRune(startByte), norm.ByteToRune(trimmedEnd))
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.BIRTH_PLACE,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   0.7,
			Sources: []entity.Source{entity.SourceRegex, entity.SourceContext},
			Reason:  "regex:birth-place",
		})
	}
	return spans
}

// trimFieldSeparator trims a place/address-like string at a comma followed by
// a field keyword (e.g. ", дата рождения", ", паспорт").
func trimFieldSeparator(s string) string {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	for _, sep := range []string{", дата рождения", ", дата", ", паспорт", ", гражданство", ", граждан", ", тел", ", телефон", ", email", ", e-mail", ", инн", ", фио", ", ф.и.о", ", код", ", адрес", ", номер карты", ", cvv", ", пин"} {
		if i := strings.Index(lower, sep); i >= 0 {
			s = s[:i]
			break
		}
	}
	return strings.TrimSpace(s)
}
