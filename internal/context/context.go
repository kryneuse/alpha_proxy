// Package context implements the context scoring layer. It inspects the text
// surrounding a candidate span and adjusts its score based on contextual
// keywords, boosting confident detections and suppressing weak ones.
package context

import (
	"strings"

	"github.com/kryneuse/alpha_proxy/internal/dict"
	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/normalize"
)

// Window is the number of runes of context examined around a span.
const Window = 20

// Scorer adjusts candidate scores based on surrounding context.
type Scorer struct {
	// keywordBoost is added to the score when a matching keyword is found.
	keywordBoost float64
	// keywordSuppress is subtracted when a negative keyword is found.
	keywordSuppress float64
}

// NewScorer builds a context scorer.
func NewScorer() *Scorer {
	return &Scorer{
		keywordBoost:    0.35,
		keywordSuppress: 0.6,
	}
}

// Score returns the adjusted span (with updated score and possibly type) and
// whether the candidate is contextual.
func (s *Scorer) Score(norm *normalize.Text, span entity.CandidateSpan) (entity.CandidateSpan, bool) {
	ctx := s.context(norm, span.Start, span.End)
	lower := strings.ToLower(ctx)

	boost := 0.0
	contextual := false

	switch span.Type {
	case entity.BIRTH_DATE:
		// For dates, use the immediately preceding context to decide whether
		// it is a birth date or a passport issue date.
		pre := s.preceding(norm, span.Start, 25)
		if containsAny(pre, dict.IssueDateKeywords) {
			// A date near issue keywords is a passport issue date.
			span.Type = entity.PASSPORT_ISSUE_DATE
			boost = s.keywordBoost
			contextual = true
		} else if containsAny(lower, dict.BirthDateKeywords) {
			boost = s.keywordBoost
			contextual = true
		}
	case entity.PASSPORT_ISSUE_DATE:
		if containsAny(lower, dict.IssueDateKeywords) {
			boost = s.keywordBoost
			contextual = true
		}
	case entity.BIRTH_PLACE:
		if containsAny(lower, dict.BirthPlaceKeywords) {
			boost = s.keywordBoost
			contextual = true
		}
	case entity.CITIZENSHIP:
		if containsAny(s.preceding(norm, span.Start, 60), dict.CitizenshipKeywords) {
			boost = s.keywordBoost
			contextual = true
		}
	case entity.PASSPORT_ISSUER:
		if containsAny(lower, dict.PassportIssuerKeywords) {
			boost = s.keywordBoost
			contextual = true
		}
	case entity.DEPARTMENT_CODE:
		pre := s.preceding(norm, span.Start, 60)
		if strings.Contains(pre, "код подразделения") || strings.Contains(pre, "кп:") ||
			strings.Contains(pre, "кп ") {
			boost = s.keywordBoost
			contextual = true
		}
	case entity.FULL_NAME:
		if containsAny(lower, dict.FullNameKeywords) {
			boost = s.keywordBoost
			contextual = true
		}
	case entity.CARDHOLDER_NAME:
		if containsAny(lower, dict.CardholderKeywords) {
			boost = s.keywordBoost
			contextual = true
		}
	case entity.ADDRESS:
		if containsAny(lower, dict.AddressKeywords) {
			boost = s.keywordBoost
			contextual = true
		}
	case entity.EMAIL:
		if containsAny(lower, dict.EmailKeywords) {
			boost = s.keywordBoost
			contextual = true
		}
	case entity.PHONE:
		if containsAny(lower, dict.PhoneKeywords) {
			boost = s.keywordBoost
			contextual = true
		}
	case entity.INN:
		if containsAny(lower, dict.InnKeywords) {
			boost = s.keywordBoost
			contextual = true
		}
	case entity.CARD_NUMBER:
		if containsAny(lower, dict.CardNumberKeywords) {
			boost = s.keywordBoost
			contextual = true
		}
	case entity.CVV:
		if containsAny(lower, dict.CvvKeywords) {
			boost = s.keywordBoost
			contextual = true
		}
	case entity.PIN:
		if containsAny(lower, dict.PinKeywords) {
			boost = s.keywordBoost
			contextual = true
		}
	case entity.PASSPORT:
		if containsAny(s.preceding(norm, span.Start, 40), dict.DriverLicenseKeywords) {
			// A 4+6 digit number near driver keywords is a driver license,
			// not a passport.
			span.Type = entity.DRIVER_LICENSE
			boost = s.keywordBoost
			contextual = true
		} else if containsAny(s.preceding(norm, span.Start, 60), dict.PassportContextKeywords) {
			boost = s.keywordBoost
			contextual = true
		}
	case entity.DRIVER_LICENSE:
		if containsAny(s.preceding(norm, span.Start, 60), dict.DriverLicenseKeywords) {
			boost = s.keywordBoost
			contextual = true
		}
	case entity.IDENTITY_DOCUMENT:
		// Identity documents require explicit document context. The boost is
		// applied per subtype so a foreign passport is not boosted by a birth
		// certificate keyword and vice versa.
		pre := s.preceding(norm, span.Start, 80)
		switch span.Subtype {
		case entity.ForeignPassportRF:
			if containsAny(pre, dict.ForeignPassportKeywords) {
				boost = s.keywordBoost
				contextual = true
			}
		case entity.BirthCertificate:
			if containsAny(pre, dict.BirthCertificateKeywords) {
				boost = s.keywordBoost
				contextual = true
			}
		case entity.MilitaryID:
			if containsAny(pre, dict.MilitaryIDKeywords) {
				boost = s.keywordBoost
				contextual = true
			}
		case entity.TemporaryIDRF:
			if containsAny(pre, dict.TemporaryIDRFKeywords) {
				boost = s.keywordBoost
				contextual = true
			}
		}
	}

	// Negative context suppression.
	switch span.Type {
	case entity.CVV:
		if containsAny(lower, dict.AuditoriumKeywords) || containsAny(lower, dict.NonBankCvvKeywords) {
			boost -= s.keywordSuppress
		}
	case entity.PIN:
		if containsAny(s.preceding(norm, span.Start, 60), dict.OrderNumberKeywords) ||
			containsAny(lower, dict.NonBankPinKeywords) {
			boost -= s.keywordSuppress
		}
	case entity.CARD_NUMBER:
		if containsAny(s.preceding(norm, span.Start, 60), dict.OrderNumberKeywords) {
			boost -= s.keywordSuppress
		}
		if containsAny(s.preceding(norm, span.Start, 60), dict.MetroTransportKeywords) {
			boost -= s.keywordSuppress
		}
		if containsAny(s.preceding(norm, span.Start, 60), dict.LoyaltyCardKeywords) {
			boost -= s.keywordSuppress
		}
	case entity.DEPARTMENT_CODE:
		if strings.Contains(lower, "товара") || strings.Contains(lower, "товар") ||
			strings.Contains(lower, "заявке") || strings.Contains(lower, "заявка") ||
			strings.Contains(lower, "заявки") || strings.Contains(lower, "артикул") {
			boost -= s.keywordSuppress
		}
	case entity.PASSPORT:
		if containsAny(s.preceding(norm, span.Start, 60), dict.OrderArticleKeywords) {
			boost -= s.keywordSuppress
		}
		// A generic "номер документа" is not a passport without passport
		// context.
		pre := s.preceding(norm, span.Start, 60)
		if strings.Contains(pre, "номер документа") && !strings.Contains(pre, "паспорт") {
			boost -= s.keywordSuppress
		}
	case entity.ADDRESS:
		if containsAny(lower, dict.BankBranchKeywords) || containsAny(lower, dict.PublicPlaceKeywords) {
			boost -= s.keywordSuppress
		}
	case entity.INN:
		if containsAny(s.preceding(norm, span.Start, 60), dict.OrderArticleKeywords) {
			boost -= s.keywordSuppress
		}
		// A 12-digit number that is a valid INN but appears in explicit
		// temporary-ID document context should be treated as the document, not
		// as an INN. Suppress the INN so the document candidate wins.
		if containsAny(s.preceding(norm, span.Start, 80), dict.TemporaryIDRFKeywords) {
			boost -= s.keywordSuppress
		}
	case entity.PHONE:
		if containsAny(lower, dict.PublicContactKeywords) {
			boost -= s.keywordSuppress
		}
	case entity.EMAIL:
		if containsAny(lower, dict.PublicContactKeywords) {
			boost -= s.keywordSuppress
		}
	case entity.FULL_NAME:
		if containsAny(lower, dict.KnownPersonContextKeywords) {
			boost -= s.keywordSuppress
		}
	case entity.BIRTH_PLACE:
		if containsAny(s.preceding(norm, span.Start, 60), dict.KnownPersonContextKeywords) {
			boost -= s.keywordSuppress
		}
	}

	score := span.Score + boost
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	span.Score = score
	return span, contextual
}

// context returns the normalized text window around [start,end), where
// start/end are ORIGINAL byte offsets.
func (s *Scorer) context(norm *normalize.Text, start, end int) string {
	ns := norm.OriginalToNorm(start)
	ne := norm.OriginalToNorm(end)
	lo := ns - Window
	if lo < 0 {
		lo = 0
	}
	hi := ne + Window
	if hi > norm.Len() {
		hi = norm.Len()
	}
	return norm.Normalized[norm.RuneToByte(lo):norm.RuneToByte(hi)]
}

// preceding returns up to n runes of normalized text immediately before the
// original byte offset start.
func (s *Scorer) preceding(norm *normalize.Text, start, n int) string {
	ns := norm.OriginalToNorm(start)
	lo := ns - n
	if lo < 0 {
		lo = 0
	}
	return norm.Normalized[norm.RuneToByte(lo):norm.RuneToByte(ns)]
}

// containsAny reports whether any keyword is a substring of s.
func containsAny(s string, d *dict.Dict) bool {
	// For substring matching we iterate over the dictionary words.
	// This is fine for the small built-in dictionaries.
	for _, w := range dictWords(d) {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// dictWords returns the words of a dictionary (used for substring matching).
func dictWords(d *dict.Dict) []string {
	return d.Words()
}
