package gate

import (
	"regexp"
	"strings"
)

// Gate is the heuristic cheap gate. It inspects a residual text and produces
// a suspicion score and routing decision.
type Gate struct {
	cfg Config

	// Structural regexes.
	passportLike *regexp.Regexp
	identifier   *regexp.Regexp
	cardLike     *regexp.Regexp
	phoneLike    *regexp.Regexp
	emailLike    *regexp.Regexp
	dateLike     *regexp.Regexp
	deptCodeLike *regexp.Regexp
	securityCode *regexp.Regexp
	nameLike     *regexp.Regexp
}

// New builds a gate with the given config.
func New(cfg Config) *Gate {
	return &Gate{
		cfg:          cfg,
		passportLike: regexp.MustCompile(`\b\d{2}\s?\d{2}\s?№?\s?\d{6}\b|\b\d{4}\s?\d{6}\b`),
		identifier:   regexp.MustCompile(`\b\d{10}\b|\b\d{12}\b`),
		cardLike:     regexp.MustCompile(`\b\d{4}[\s\-]?\d{4}[\s\-]?\d{4}[\s\-]?\d{4}\b`),
		phoneLike:    regexp.MustCompile(`(?:\+7|8|7)[\s\-]?\(?\d{3}\)?[\s\-]?\d{3}[\s\-]?\d{2}[\s\-]?\d{2}`),
		emailLike:    regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
		dateLike:     regexp.MustCompile(`\b\d{1,2}[./\-]\d{1,2}[./\-]\d{2,4}\b|\b\d{4}-\d{2}-\d{2}\b`),
		deptCodeLike: regexp.MustCompile(`\b\d{3}[-–]\d{3}\b`),
		securityCode: regexp.MustCompile(`\b\d{3}\b|\b\d{4}\b`),
		nameLike:     regexp.MustCompile(`[А-ЯЁ][а-яё]+(?:\s+[А-ЯЁ][а-яё]+){1,2}`),
	}
}

// Evaluate runs the gate on a residual text and returns a decision.
func (g *Gate) Evaluate(residual string) Decision {
	score := 0.0
	var signals []Signal

	lower := strings.ToLower(residual)

	// --- Structural signals ---
	if g.passportLike.MatchString(residual) {
		score += g.cfg.PassportLikeDigits
		signals = append(signals, Signal{Name: "passport_like_digits", Weight: g.cfg.PassportLikeDigits, Contribution: g.cfg.PassportLikeDigits})
	}
	if g.identifier.MatchString(residual) {
		score += g.cfg.IdentifierDigits
		signals = append(signals, Signal{Name: "identifier_digits", Weight: g.cfg.IdentifierDigits, Contribution: g.cfg.IdentifierDigits})
	}
	if g.cardLike.MatchString(residual) {
		score += g.cfg.CardLikeDigits
		signals = append(signals, Signal{Name: "card_like_digits", Weight: g.cfg.CardLikeDigits, Contribution: g.cfg.CardLikeDigits})
	}
	if g.phoneLike.MatchString(residual) {
		score += g.cfg.PhoneLikeDigits
		signals = append(signals, Signal{Name: "phone_like", Weight: g.cfg.PhoneLikeDigits, Contribution: g.cfg.PhoneLikeDigits})
	}
	if g.emailLike.MatchString(residual) {
		score += g.cfg.EmailLike
		signals = append(signals, Signal{Name: "email_like", Weight: g.cfg.EmailLike, Contribution: g.cfg.EmailLike})
	}
	if g.dateLike.MatchString(residual) {
		score += g.cfg.DateLike
		signals = append(signals, Signal{Name: "date_like", Weight: g.cfg.DateLike, Contribution: g.cfg.DateLike})
	}
	if g.deptCodeLike.MatchString(residual) {
		score += g.cfg.DepartmentCodeLike
		signals = append(signals, Signal{Name: "department_code_like", Weight: g.cfg.DepartmentCodeLike, Contribution: g.cfg.DepartmentCodeLike})
	}
	if g.securityCode.MatchString(residual) {
		score += g.cfg.SecurityCodeLike
		signals = append(signals, Signal{Name: "security_code_like", Weight: g.cfg.SecurityCodeLike, Contribution: g.cfg.SecurityCodeLike})
	}
	if g.nameLike.MatchString(residual) {
		score += g.cfg.NameLike
		signals = append(signals, Signal{Name: "name_like", Weight: g.cfg.NameLike, Contribution: g.cfg.NameLike})
	}

	// --- Contextual signals ---
	ctxSignals := []struct {
		name   string
		weight float64
		words  []string
	}{
		{"passport_context", g.cfg.PassportContext, []string{"паспорт", "паспорта", "паспорте"}},
		{"series_number", g.cfg.SeriesNumber, []string{"серия", "номер", "серия и номер"}},
		{"issued", g.cfg.Issued, []string{"выдан", "выдано", "кем выдан"}},
		{"department_code_ctx", g.cfg.DepartmentCodeCtx, []string{"код подразделения", "подразделение"}},
		{"citizenship_ctx", g.cfg.CitizenshipCtx, []string{"гражданство", "гражданин", "гражданка", "гражданином"}},
		{"birth_ctx", g.cfg.BirthCtx, []string{"родился", "родилась", "место рождения", "дата рождения"}},
		{"address_ctx", g.cfg.AddressCtx, []string{"проживает", "адрес", "адрес проживания", "адрес регистрации", "живет по адресу"}},
		{"inn_ctx", g.cfg.InnCtx, []string{"инн", "иин"}},
		{"card_ctx", g.cfg.CardCtx, []string{"карта", "карты", "держатель", "номер карты"}},
		{"cvv_ctx", g.cfg.CvvCtx, []string{"cvv", "cvc", "код безопасности", "защитный код"}},
		{"pin_ctx", g.cfg.PinCtx, []string{"пин", "пин-код", "пин код", "pin"}},
		{"driver_license_ctx", g.cfg.DriverLicenseCtx, []string{"водительское удостоверение", "водительские права", "в/у", "ву"}},
		{"client_ctx", g.cfg.ClientCtx, []string{"клиент", "заявитель", "держатель", "заёмщик", "заемщик", "пациент", "сотрудник"}},
	}
	for _, cs := range ctxSignals {
		if containsAny(lower, cs.words) {
			score += cs.weight
			signals = append(signals, Signal{Name: cs.name, Weight: cs.weight, Contribution: cs.weight})
		}
	}

	// --- Public/non-PII context: small negative signal ---
	publicWords := []string{"офис", "магазин", "ресторан", "служба поддержки", "горячая линия", "компании", "отдел продаж"}
	if containsAny(lower, publicWords) {
		score -= g.cfg.PublicContext
		signals = append(signals, Signal{Name: "public_context", Weight: g.cfg.PublicContext, Contribution: -g.cfg.PublicContext})
	}

	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	return Decision{
		Score:   score,
		Route:   g.cfg.RouteForScore(score),
		Signals: signals,
	}
}

// containsAny reports whether any word is a substring of s.
func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}
