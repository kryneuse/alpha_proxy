package recognizer

import (
	"regexp"

	"github.com/alpha-proxy/rule-engine/internal/entity"
	"github.com/alpha-proxy/rule-engine/internal/normalize"
)

// regexRecognizer is a base helper for regex-driven recognizers. It runs a
// compiled regex over the normalized text and maps matches back to original
// offsets.
type regexRecognizer struct {
	etype entity.Type
	re    *regexp.Regexp
	score float64
}

// newRegexRecognizer compiles the pattern and returns a helper.
func newRegexRecognizer(etype entity.Type, pattern string, score float64) *regexRecognizer {
	return &regexRecognizer{etype: etype, re: regexp.MustCompile(pattern), score: score}
}

// find returns candidate spans for all non-overlapping matches, mapped to
// original offsets. The optional transform lets a recognizer adjust the
// matched text (e.g. trim surrounding context).
func (r *regexRecognizer) find(norm *normalize.Text, transform func(match string) (string, int, int)) []entity.CandidateSpan {
	var spans []entity.CandidateSpan
	for _, loc := range r.re.FindAllStringIndex(norm.Normalized, -1) {
		// Convert byte offsets to rune offsets for MapSpan.
		start := norm.ByteToRune(loc[0])
		end := norm.ByteToRune(loc[1])
		text := norm.Normalized[loc[0]:loc[1]]
		if transform != nil {
			t, s, e := transform(text)
			text = t
			start, end = s, e
		}
		oStart, oEnd := norm.MapSpan(start, end)
		spans = append(spans, entity.CandidateSpan{
			Type:    r.etype,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   r.score,
			Sources: []entity.Source{entity.SourceRegex},
			Reason:  "regex:" + r.re.String(),
		})
	}
	return spans
}
