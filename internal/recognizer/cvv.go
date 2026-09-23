package recognizer

import (
	"strings"
	"unicode"

	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/normalize"
)

// CvvRecognizer detects CVV codes only when a 3-digit value is bound to an
// explicit signature (CVV, CVC, "код безопасности", etc.). A bare 3-digit
// number is never a CVV.
type CvvRecognizer struct{}

// NewCvvRecognizer builds a CVV recognizer.
func NewCvvRecognizer() *CvvRecognizer {
	return &CvvRecognizer{}
}

// Type returns the entity type.
func (r *CvvRecognizer) Type() entity.Type { return entity.CVV }

// Recognize finds CVV candidates that are bound to a signature.
func (r *CvvRecognizer) Recognize(norm *normalize.Text) []entity.CandidateSpan {
	runes := []rune(norm.Normalized)
	var spans []entity.CandidateSpan
	for _, cand := range findCvvCandidates(runes) {
		if !boundToSignature(runes, cand) {
			continue
		}
		oStart, oEnd := norm.MapSpan(cand.start, cand.end)
		spans = append(spans, entity.CandidateSpan{
			Type:    entity.CVV,
			Text:    norm.Original[oStart:oEnd],
			Start:   oStart,
			End:     oEnd,
			Score:   0.7,
			Sources: []entity.Source{entity.SourceRegex, entity.SourceContext},
			Reason:  "cvv-signature-bound",
		})
	}
	return spans
}

// cvvCandidate is a 3-digit candidate in normalized rune positions.
type cvvCandidate struct{ start, end int }

// findCvvCandidates finds 3-digit values that are not fragments of longer
// numbers. The returned span includes internal separators (spaces, dashes,
// tabs, zero-width characters).
func findCvvCandidates(runes []rune) []cvvCandidate {
	var out []cvvCandidate
	i := 0
	n := len(runes)
	for i < n {
		if !unicode.IsDigit(runes[i]) {
			i++
			continue
		}
		runStart := i
		j := i
		digitCount := 0
		for j < n {
			r := runes[j]
			if unicode.IsDigit(r) {
				digitCount++
				j++
			} else if isDigitSep(r) {
				j++
			} else {
				break
			}
		}
		runEnd := j
		if digitCount == 3 {
			firstDigit := runStart
			for firstDigit < runEnd && !unicode.IsDigit(runes[firstDigit]) {
				firstDigit++
			}
			lastDigit := runEnd - 1
			for lastDigit >= runStart && !unicode.IsDigit(runes[lastDigit]) {
				lastDigit--
			}
			if !wordCharBefore(runes, firstDigit) && !wordCharAfter(runes, lastDigit+1) {
				out = append(out, cvvCandidate{start: firstDigit, end: lastDigit + 1})
			}
		}
		i = runEnd
	}
	return out
}

// isDigitSep reports whether r is an allowed separator between CVV digits.
func isDigitSep(r rune) bool {
	switch r {
	case ' ', '\t', '-', '\u200B', '\u200C', '\u200D', '\uFEFF', '\u2060':
		return true
	}
	return false
}

// wordCharBefore reports whether the rune immediately before pos is a word
// character (letter or digit).
func wordCharBefore(runes []rune, pos int) bool {
	if pos <= 0 {
		return false
	}
	return isWordChar(runes[pos-1])
}

// wordCharAfter reports whether the rune immediately after pos is a word
// character (letter or digit).
func wordCharAfter(runes []rune, pos int) bool {
	if pos >= len(runes) {
		return false
	}
	return isWordChar(runes[pos])
}

// isWordChar reports whether r is a letter or digit.
func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// cvvWindow is the maximum number of Unicode characters on each side of a
// candidate within which a signature may be found.
const cvvWindow = 160

// cvvSignatures are the explicit signatures that bind a 3-digit value to a
// CVV. Longer variants are listed first.
var cvvSignatures = []string{
	"три цифры на обороте карты",
	"код с обратной стороны",
	"код на обороте",
	"код безопасности",
	"защитный код",
	"трёхзначный код",
	"трехзначный код",
	"cvv2", "cvc2", "cvv", "cvc",
	"цвв", "цвс", "свв", "свс",
}

// cvvCompetingKeywords are assignments that make a nearby number something
// other than a CVV.
var cvvCompetingKeywords = []string{
	"сумма", "цена", "рублей", "руб", "стоимость", "₽", "оплата", "платеж", "платёж",
	"заказ", "заявка", "артикул", "договор",
	"sms", "otp", "одноразовый", "код подтверждения", "смс",
	"ошибка", "http", "код ошибки",
	"кабинет", "аудитория", "комната", "офис", "помещение",
	"домофон", "замок", "сейф", "кодовый замок", "шкафчик", "ячейка",
	"телефон", "дом", "квартира",
	"пин", "pin", "дата", "срок действия", "срок",
}

// sigMatch is a signature match in normalized rune positions.
type sigMatch struct{ start, end int }

// boundToSignature reports whether the candidate is bound to a CVV signature
// within cvvWindow characters on either side.
func boundToSignature(runes []rune, cand cvvCandidate) bool {
	// Signature before the candidate.
	leftStart := cand.start - cvvWindow
	if leftStart < 0 {
		leftStart = 0
	}
	for _, sig := range findSignatures(runes, leftStart, cand.start) {
		if validGap(runes, sig.end, cand.start, cand) {
			return true
		}
	}
	// Signature after the candidate.
	rightEnd := cand.end + cvvWindow
	if rightEnd > len(runes) {
		rightEnd = len(runes)
	}
	for _, sig := range findSignatures(runes, cand.end, rightEnd) {
		if validGap(runes, cand.end, sig.start, cand) {
			return true
		}
	}
	return false
}

// findSignatures returns all signature matches within [from, to).
func findSignatures(runes []rune, from, to int) []sigMatch {
	var out []sigMatch
	for _, sig := range cvvSignatures {
		sigRunes := []rune(sig)
		for i := from; i+len(sigRunes) <= to; i++ {
			if matchRunes(runes, i, sigRunes) && wordBounded(runes, i, i+len(sigRunes)) {
				out = append(out, sigMatch{start: i, end: i + len(sigRunes)})
			}
		}
	}
	return out
}

// matchRunes reports whether runes[i:i+len(sig)] equals sig.
func matchRunes(runes []rune, i int, sig []rune) bool {
	for k := range sig {
		if runes[i+k] != sig[k] {
			return false
		}
	}
	return true
}

// wordBounded reports whether the range [s,e) is bounded by non-word
// characters on both sides.
func wordBounded(runes []rune, s, e int) bool {
	if s > 0 && isWordChar(runes[s-1]) {
		return false
	}
	if e < len(runes) && isWordChar(runes[e]) {
		return false
	}
	return true
}

// validGap reports whether the gap between two ranges satisfies the binding
// constraints: no semicolon, at most one sentence-ending mark, no competing
// field keyword, no other numeric value, and no competing field near the
// candidate.
func validGap(runes []rune, gapStart, gapEnd int, cand cvvCandidate) bool {
	sentenceEnds := 0
	for i := gapStart; i < gapEnd; i++ {
		switch runes[i] {
		case ';':
			return false
		case '.', '!', '?', '…':
			sentenceEnds++
			if sentenceEnds > 1 {
				return false
			}
		}
	}
	if containsKeyword(runes, gapStart, gapEnd, cvvCompetingKeywords) {
		return false
	}
	if hasNumericValue(runes, gapStart, gapEnd) {
		return false
	}
	if hasCompetingNearValue(runes, cand) {
		return false
	}
	return true
}

// hasNumericValue reports whether the range contains any digit.
func hasNumericValue(runes []rune, from, to int) bool {
	for i := from; i < to; i++ {
		if unicode.IsDigit(runes[i]) {
			return true
		}
	}
	return false
}

// hasCompetingNearValue reports whether a competing field keyword appears
// directly adjacent to the candidate (before a strong separator).
func hasCompetingNearValue(runes []rune, cand cvvCandidate) bool {
	// After the value.
	afterEnd := cand.end + 20
	if afterEnd > len(runes) {
		afterEnd = len(runes)
	}
	stop := cand.end
	for i := cand.end; i < afterEnd; i++ {
		if isStrongSep(runes[i]) {
			stop = i
			break
		}
	}
	if containsKeyword(runes, cand.end, stop, cvvCompetingKeywords) {
		return true
	}
	// Before the value.
	beforeStart := cand.start - 20
	if beforeStart < 0 {
		beforeStart = 0
	}
	stopStart := cand.start
	for i := cand.start - 1; i >= beforeStart; i-- {
		if isStrongSep(runes[i]) {
			stopStart = i + 1
			break
		}
	}
	if containsKeyword(runes, stopStart, cand.start, cvvCompetingKeywords) {
		return true
	}
	return false
}

// isStrongSep reports whether r strongly separates fields.
func isStrongSep(r rune) bool {
	switch r {
	case ',', ';', '.', '!', '?', '\n', '\r':
		return true
	}
	return false
}

// containsKeyword reports whether any keyword is a substring of the range.
func containsKeyword(runes []rune, from, to int, keywords []string) bool {
	if from >= to {
		return false
	}
	lower := strings.ToLower(string(runes[from:to]))
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
