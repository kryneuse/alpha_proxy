// Command identity-eval evaluates the identity-document recognizer on a
// dedicated dataset (positive examples per subtype + hard negatives). It is a
// research/report tool and does not affect the 17 mandatory PII types.
//
// Metrics: TP / FP / FN, Precision, Recall, F1, exact span accuracy, and
// per-subtype recall.
package main

import (
	"fmt"
	"os"

	"github.com/kryneuse/alpha_proxy/internal/engine"
	"github.com/kryneuse/alpha_proxy/internal/entity"
)

type positive struct {
	text    string
	subtype entity.DocumentSubtype
	want    string // exact expected text (span)
}

// positives contains 15+ examples per subtype, with variants not literally
// present in the unit tests.
var positives = []positive{
	// Foreign passport RF.
	{"Загранпаспорт 62 1234567", entity.ForeignPassportRF, "62 1234567"},
	{"Загранпаспорт 62 № 1234567", entity.ForeignPassportRF, "62 № 1234567"},
	{"Загранпаспорт серия 62 номер 1234567", entity.ForeignPassportRF, "серия 62 номер 1234567"},
	{"Заграничный паспорт 621234567", entity.ForeignPassportRF, "621234567"},
	{"паспорт для выезда за границу 62 1234567", entity.ForeignPassportRF, "62 1234567"},
	{"номер загранпаспорта 62 1234567", entity.ForeignPassportRF, "62 1234567"},
	{"Загранпаспорт 62-1234567", entity.ForeignPassportRF, "62-1234567"},
	{"загранпаспорт 71 № 7654321", entity.ForeignPassportRF, "71 № 7654321"},
	{"Загранпаспорт 55 1234567", entity.ForeignPassportRF, "55 1234567"},
	{"заграничный паспорт 63 № 2345678", entity.ForeignPassportRF, "63 № 2345678"},
	{"паспорт для выезда за границу 64 3456789", entity.ForeignPassportRF, "64 3456789"},
	{"серия загранпаспорта 65 номер 4567890", entity.ForeignPassportRF, "серия загранпаспорта 65 номер 4567890"},
	{"Загранпаспорт 66 № 5678901", entity.ForeignPassportRF, "66 № 5678901"},
	{"загран паспорт 67 6789012", entity.ForeignPassportRF, "67 6789012"},
	{"Загранпаспорт 68 № 7890123", entity.ForeignPassportRF, "68 № 7890123"},
	{"заграничный паспорт 69 8901234", entity.ForeignPassportRF, "69 8901234"},
	{"Загранпаспорт 70 № 9012345", entity.ForeignPassportRF, "70 № 9012345"},
	{"паспорт для выезда за границу 71 0123456", entity.ForeignPassportRF, "71 0123456"},

	// Birth certificate.
	{"свидетельство о рождении II-МЮ № 123456", entity.BirthCertificate, "II-МЮ № 123456"},
	{"свидетельство о рождении I-АБ 123456", entity.BirthCertificate, "I-АБ 123456"},
	{"свидетельство о рождении III-ВГ №123456", entity.BirthCertificate, "III-ВГ №123456"},
	{"свидетельство о рождении серия II-МЮ, номер 123456", entity.BirthCertificate, "серия II-МЮ, номер 123456"},
	{"номер свидетельства о рождении II-МЮ № 123456", entity.BirthCertificate, "II-МЮ № 123456"},
	{"св-во о рождении II-МЮ № 123456", entity.BirthCertificate, "II-МЮ № 123456"},
	{"Свидетельство о рождении IV-ДЕ № 654321", entity.BirthCertificate, "IV-ДЕ № 654321"},
	{"свидетельство о рождении V-ЖЗ № 111111", entity.BirthCertificate, "V-ЖЗ № 111111"},
	{"свидетельство о рождении II-КЛ № 222222", entity.BirthCertificate, "II-КЛ № 222222"},
	{"свидетельство о рождении I-МН № 333333", entity.BirthCertificate, "I-МН № 333333"},
	{"свидетельство о рождении III-ОП № 444444", entity.BirthCertificate, "III-ОП № 444444"},
	{"свидетельство о рождении II-РС № 555555", entity.BirthCertificate, "II-РС № 555555"},
	{"свидетельство о рождении I-ТУ № 666666", entity.BirthCertificate, "I-ТУ № 666666"},
	{"свидетельство о рождении II-ФХ № 777777", entity.BirthCertificate, "II-ФХ № 777777"},
	{"свидетельство о рождении III-ЦЧ № 888888", entity.BirthCertificate, "III-ЦЧ № 888888"},
	{"свидетельство о рождении II-ШЩ № 999999", entity.BirthCertificate, "II-ШЩ № 999999"},
	{"свидетельство о рождении I-ЭЮ № 100000", entity.BirthCertificate, "I-ЭЮ № 100000"},

	// Military ID.
	{"Военный билет ГД № 1234567", entity.MilitaryID, "ГД № 1234567"},
	{"военный билет НЛ № 7654321", entity.MilitaryID, "НЛ № 7654321"},
	{"военный билет офицера запаса ГД № 1234567", entity.MilitaryID, "ГД № 1234567"},
	{"Временное удостоверение взамен военного билета НЛ № 7654321", entity.MilitaryID, "НЛ № 7654321"},
	{"временное удостоверение взамен военного билета офицера запаса ГД № 1234567", entity.MilitaryID, "ГД № 1234567"},
	{"Военный билет ГД N 1234567", entity.MilitaryID, "ГД N 1234567"},
	{"военный билет ГД 1234567", entity.MilitaryID, "ГД 1234567"},
	{"военный билет АБ № 2345678", entity.MilitaryID, "АБ № 2345678"},
	{"военный билет ВГ № 3456789", entity.MilitaryID, "ВГ № 3456789"},
	{"военный билет ДЕ № 4567890", entity.MilitaryID, "ДЕ № 4567890"},
	{"военный билет ЖЗ № 5678901", entity.MilitaryID, "ЖЗ № 5678901"},
	{"военный билет ИК № 6789012", entity.MilitaryID, "ИК № 6789012"},
	{"военный билет ЛМ № 7890123", entity.MilitaryID, "ЛМ № 7890123"},
	{"военный билет НО № 8901234", entity.MilitaryID, "НО № 8901234"},
	{"военный билет ПР № 9012345", entity.MilitaryID, "ПР № 9012345"},
	{"военный билет СТ № 0123456", entity.MilitaryID, "СТ № 0123456"},
	{"военный билет УФ № 123456", entity.MilitaryID, "УФ № 123456"},

	// Temporary ID RF.
	{"Временное удостоверение личности форма 2П № 12345678", entity.TemporaryIDRF, "12345678"},
	{"Временное удостоверение личности № 770041160025", entity.TemporaryIDRF, "770041160025"},
	{"временное удостоверение личности гражданина РФ № 12345678", entity.TemporaryIDRF, "12345678"},
	{"форма № 2П 12345678", entity.TemporaryIDRF, "12345678"},
	{"Временное удостоверение личности АБ 12345678", entity.TemporaryIDRF, "АБ 12345678"},
	{"временное удостоверение личности № 23456789", entity.TemporaryIDRF, "23456789"},
	{"временное удостоверение личности гражданина РФ № 34567890", entity.TemporaryIDRF, "34567890"},
	{"форма 2П № 45678901", entity.TemporaryIDRF, "45678901"},
	{"временное удостоверение личности № 56789012", entity.TemporaryIDRF, "56789012"},
	{"временное удостоверение личности № 678901234567", entity.TemporaryIDRF, "678901234567"},
	{"временное удостоверение личности гражданина РФ № 789012345678", entity.TemporaryIDRF, "789012345678"},
	{"форма № 2П 89012345", entity.TemporaryIDRF, "89012345"},
	{"временное удостоверение личности ВГ 90123456", entity.TemporaryIDRF, "ВГ 90123456"},
	{"временное удостоверение личности № 01234567", entity.TemporaryIDRF, "01234567"},
	{"временное удостоверение личности гражданина РФ № 123456789012", entity.TemporaryIDRF, "123456789012"},
	{"форма 2П № 23456789", entity.TemporaryIDRF, "23456789"},
	{"временное удостоверение личности № 34567890", entity.TemporaryIDRF, "34567890"},
}

// hardNegatives must NOT produce any IDENTITY_DOCUMENT entity.
var hardNegatives = []string{
	// Foreign passport lookalikes.
	"Заказ 621234567",
	"Номер договора 62 1234567",
	"Артикул 62 1234567",
	"Счёт 62 1234567",
	"Телефон 62 1234567",
	"Заявка 621234567",
	"Партия 62 1234567",
	"Серийный номер 62 1234567",
	// Birth certificate lookalikes.
	"II-МЮ № 123456",
	"I-АБ 123456",
	"III-ВГ №123456",
	"серия II-МЮ, номер 123456",
	"номер 123456",
	"свидетельство о браке II-МЮ № 123456",
	"справка II-МЮ № 123456",
	"II-МЮ",
	"серия II-МЮ",
	// Military ID lookalikes.
	"ГД 1234567",
	"НЛ 7654321",
	"Артикул НЛ 1234567",
	"Код товара ГД 1234567",
	"Партия НЛ 7654321",
	"Серийный номер ГД 1234567",
	// Temporary ID lookalikes.
	"Заказ 12345678",
	"Номер 12345678",
	"Артикул 12345678",
	"Заказ 770041160025",
	"Номер договора 770041160025",
	"12345678",
	"770041160025",
	"621234567",
	// INN without document context.
	"ИНН 7707083893",
	"ИНН 500100732259",
	"ИНН 770041160025",
	"123456789012",
	"1234567890",
	// Ordinary Russian passport / driver license / department code.
	"паспорт 4510 123456",
	"серия 4510 номер 123456",
	"водительское удостоверение 77 12 345678",
	"код подразделения 770-001",
	// Foreign passport split without foreign-passport context.
	"серия паспорта 62 номер 1234567",
	"серия документа 62 номер 1234567",
	"серия свидетельства 62 номер 1234567",
	"Документ: серия 62 номер 1234567",
	// UTF-8 token boundary: Cyrillic/Latin prefix or suffix.
	"Военный билет АБВГ 1234567",
	"Военный билет тестГД 1234567",
	"Временное удостоверение личности ХХАБ 12345678",
	"Временное удостоверение личности testАБ 12345678",
	"свидетельство о рождении АII-МЮ № 123456",
	"свидетельство о рождении testII-МЮ № 123456",
	"Военный билет ГД № 1234567abc",
	"свидетельство о рождении II-МЮ № 123456abc",
	// Birth certificate with a single Cyrillic letter in the series.
	"свидетельство о рождении I-А № 123456",
	// Boundary: document inside a longer Cyrillic/Latin token, or trailing
	// extra digits, or "серия" inside a longer word.
	"Загранпаспорт А621234567Б",
	"Загранпаспорт 621234567А",
	"Загранпаспорт А62 1234567",
	"Загранпаспорт серия 62 номер 12345678",
	"Загранпаспорт суперсерия 62 номер 1234567",
	"свидетельство о рождении серия II-МЮ, номер 1234567",
	"свидетельство о рождении суперсерия II-МЮ, номер 123456",
}

func main() {
	e := engine.New(engine.Options{})

	// Per-subtype stats.
	type subStats struct {
		tp, fn, exact int
	}
	sub := map[entity.DocumentSubtype]*subStats{
		entity.ForeignPassportRF: {},
		entity.BirthCertificate:  {},
		entity.MilitaryID:        {},
		entity.TemporaryIDRF:     {},
	}

	tp, fp, fn := 0, 0, 0
	exact := 0

	// Positives.
	for _, p := range positives {
		ents := e.Analyze(p.text)
		doc := findDoc(ents)
		if doc == nil {
			fn++
			sub[p.subtype].fn++
			fmt.Printf("FN %-20s %q\n", p.subtype, p.text)
			continue
		}
		if doc.Subtype != p.subtype {
			fp++
			fmt.Printf("FP subtype %-20s want %s got %s %q\n", p.subtype, p.subtype, doc.Subtype, p.text)
			continue
		}
		tp++
		sub[p.subtype].tp++
		if doc.Text == p.want {
			exact++
			sub[p.subtype].exact++
		} else {
			fmt.Printf("SPAN %-20s want %q got %q %q\n", p.subtype, p.want, doc.Text, p.text)
		}
	}

	// Hard negatives.
	for _, h := range hardNegatives {
		ents := e.Analyze(h)
		if doc := findDoc(ents); doc != nil {
			fp++
			fmt.Printf("FP hard-neg %-20s %q -> %s/%s\n", doc.Subtype, h, doc.Type, doc.Subtype)
		}
	}

	totalPos := len(positives)
	precision := float64(tp) / float64(tp+fp)
	recall := float64(tp) / float64(tp+fn)
	f1 := 2 * precision * recall / (precision + recall)
	spanAcc := float64(exact) / float64(tp)

	fmt.Println("\n=== Identity document evaluation ===")
	fmt.Printf("positives=%d hard_negatives=%d\n", totalPos, len(hardNegatives))
	fmt.Printf("TP=%d FP=%d FN=%d\n", tp, fp, fn)
	fmt.Printf("Precision=%.3f Recall=%.3f F1=%.3f\n", precision, recall, f1)
	fmt.Printf("exact span accuracy=%.3f (%d/%d)\n", spanAcc, exact, tp)
	fmt.Println("\nper-subtype recall:")
	for _, st := range []entity.DocumentSubtype{
		entity.ForeignPassportRF, entity.BirthCertificate, entity.MilitaryID, entity.TemporaryIDRF,
	} {
		s := sub[st]
		denom := s.tp + s.fn
		r := 0.0
		if denom > 0 {
			r = float64(s.tp) / float64(denom)
		}
		fmt.Printf("  %-20s TP=%d FN=%d recall=%.3f exact=%d\n", st, s.tp, s.fn, r, s.exact)
	}

	if tp+fn != totalPos {
		fmt.Printf("\nWARNING: positives accounted %d != %d\n", tp+fn, totalPos)
		os.Exit(1)
	}
}

func findDoc(ents []entity.Entity) *entity.Entity {
	for i := range ents {
		if ents[i].Type == entity.IDENTITY_DOCUMENT {
			return &ents[i]
		}
	}
	return nil
}
