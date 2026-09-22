package recognizer

import (
	"regexp"
	"strings"

	"github.com/alpha-proxy/rule-engine/internal/dict"
	"github.com/alpha-proxy/rule-engine/internal/entity"
	"github.com/alpha-proxy/rule-engine/internal/normalize"
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

// Recognize finds citizenship candidates.
func (r *CitizenshipRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan
	words := strings.Fields(norm.Normalized)
	for i := 0; i < len(words); i++ {
		// Try multi-word country names (up to 3 words).
		for n := 3; n >= 1; n-- {
			if i+n > len(words) {
				continue
			}
			phrase := strings.Join(words[i:i+n], " ")
			if r.countries.Contains(phrase) {
				start := indexOf(norm.Normalized, words[i])
				end := start + len(phrase)
				oStart, oEnd := norm.MapSpan(norm.ByteToRune(start), norm.ByteToRune(end))
				spans = append(spans, entity.CandidateSpan{
					Type:    entity.CITIZENSHIP,
					Text:    norm.Original[oStart:oEnd],
					Start:   oStart,
					End:     oEnd,
					Score:   0.6,
					Sources: []entity.Source{entity.SourceDictionary},
					Reason:  "dict:country",
				})
				i += n - 1
				break
			}
		}
	}
	return spans
}

// BirthPlaceRecognizer detects birth places using context constructions.
type BirthPlaceRecognizer struct {
	re *regexp.Regexp
}

// NewBirthPlaceRecognizer builds a birth place recognizer.
func NewBirthPlaceRecognizer() *BirthPlaceRecognizer {
	return &BirthPlaceRecognizer{
		re: regexp.MustCompile(`(?:место рождения|родился|родилась|уроженец|уроженка|родом из)[:\s]+([а-яёa-z\s.,\-]{2,60})`),
	}
}

// Type returns the entity type.
func (r *BirthPlaceRecognizer) Type() entity.Type { return entity.BIRTH_PLACE }

// Recognize finds birth place candidates.
func (r *BirthPlaceRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	var spans []entity.CandidateSpan
	for _, loc := range r.re.FindAllStringIndex(norm.Normalized, -1) {
		oStart, oEnd := norm.MapSpan(norm.ByteToRune(loc[0]), norm.ByteToRune(loc[1]))
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

// indexOf returns the byte index of the first occurrence of word in s.
func indexOf(s, word string) int {
	return strings.Index(s, word)
}
