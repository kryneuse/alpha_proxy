// Command gateeval-audit audits the gateeval dataset labels. It lists every
// positive sample and classifies it as confirmed PII vs ambiguous/suspicious,
// and validates INN checksums for 10/12-digit numeric samples.
//
// This is a research/audit tool only. It does NOT modify the dataset or any
// production code.
package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kryneuse/alpha_proxy/internal/gateeval"
	"github.com/kryneuse/alpha_proxy/internal/recognizer"
)

var (
	re10 = regexp.MustCompile(`\b\d{10}\b`)
	re12 = regexp.MustCompile(`\b\d{12}\b`)
	re16 = regexp.MustCompile(`\b\d{16}\b`)
)

func main() {
	samples := gateeval.Dataset()
	fmt.Println("=== Positive samples audit (HasPII=1) ===")
	fmt.Printf("%-4s %-8s %-14s %s\n", "idx", "class", "inn/luhn", "text")
	for i, s := range samples {
		if s.HasPII != 1 {
			continue
		}
		cls := classify(s.Text)
		check := checksumNote(s.Text)
		fmt.Printf("%-4d %-8s %-14s %q\n", i, cls, check, s.Text)
	}
}

// classify returns a coarse class for a positive sample.
func classify(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "паспорт"), strings.Contains(lower, "серия"),
		strings.Contains(lower, "водительск"), strings.Contains(lower, "в/у"),
		strings.Contains(lower, "права"):
		return "DOCUMENT"
	case strings.Contains(lower, "инн"):
		return "INN"
	case strings.Contains(lower, "карт"), strings.Contains(lower, "cvv"),
		strings.Contains(lower, "держатель"):
		return "CARD"
	case strings.Contains(lower, "тел"), strings.Contains(lower, "+7"),
		strings.Contains(lower, "8 "):
		return "PHONE"
	case strings.Contains(lower, "email"), strings.Contains(lower, "@"):
		return "EMAIL"
	case strings.Contains(lower, "родил"), strings.Contains(lower, "дата рождения"),
		strings.Contains(lower, "день рождения"):
		return "BIRTH"
	case strings.Contains(lower, "граждан"), strings.Contains(lower, "гражданин"):
		return "CITIZENSHIP"
	case strings.Contains(lower, "адрес"), strings.Contains(lower, "проживает"),
		strings.Contains(lower, "живет"), strings.Contains(lower, "зарегистрирован"):
		return "ADDRESS"
	case strings.Contains(lower, "код"), strings.Contains(lower, "пароль"),
		strings.Contains(lower, "номер"), strings.Contains(lower, "пин"):
		return "AMBIG_NUM"
	case re16.MatchString(text):
		return "CARD_NUM"
	case re12.MatchString(text):
		return "INN12"
	case re10.MatchString(text):
		return "INN10"
	default:
		return "NAME/OTHER"
	}
}

// checksumNote validates INN checksums and Luhn for numeric samples.
// It strips spaces/dashes so grouped numbers (e.g. card numbers) are checked
// as a single digit run.
func checksumNote(text string) string {
	compact := regexp.MustCompile(`[\s\-]`).ReplaceAllString(text, "")
	digits := regexp.MustCompile(`\d+`).FindAllString(compact, -1)
	var notes []string
	for _, d := range digits {
		switch len(d) {
		case 10:
			if recognizer.Inn10(d) {
				notes = append(notes, "INN10-valid")
			} else {
				notes = append(notes, "INN10-INVALID")
			}
		case 12:
			if recognizer.Inn12(d) {
				notes = append(notes, "INN12-valid")
			} else {
				notes = append(notes, "INN12-INVALID")
			}
		case 16:
			if recognizer.Luhn(d) {
				notes = append(notes, "Luhn-valid")
			} else {
				notes = append(notes, "Luhn-INVALID")
			}
		}
	}
	if len(notes) == 0 {
		return "-"
	}
	return strings.Join(notes, ",")
}