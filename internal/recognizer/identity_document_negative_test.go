package recognizer_test

import (
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/entity"
)

// assertNoIdentity checks that no IDENTITY_DOCUMENT entity is produced for the
// given text. Other entity types (e.g. PASSPORT, INN, PHONE) may still fire.
func assertNoIdentity(t *testing.T, text string) {
	t.Helper()
	ents := analyze(t, text)
	for _, en := range ents {
		if en.Type == entity.IDENTITY_DOCUMENT {
			t.Fatalf("expected no IDENTITY_DOCUMENT in %q, got %+v", text, ents)
		}
	}
}

func TestIdentityHardNegatives(t *testing.T) {
	cases := []string{
		// Foreign passport lookalikes without context.
		"Заказ 621234567",
		"Номер договора 62 1234567",
		"Артикул 62 1234567",
		"Счёт 62 1234567",
		"Телефон 62 1234567",
		"Заявка 621234567",
		// Birth certificate lookalikes without context.
		"II-МЮ № 123456",
		"I-АБ 123456",
		"III-ВГ №123456",
		"серия II-МЮ, номер 123456",
		"номер 123456",
		// Military ID lookalikes without military context.
		"ГД 1234567",
		"НЛ 7654321",
		"Артикул НЛ 1234567",
		"Код товара ГД 1234567",
		"Партия НЛ 7654321",
		// Temporary ID lookalikes without context.
		"Заказ 12345678",
		"Номер 12345678",
		"Артикул 12345678",
		"Заказ 770041160025",
		"Номер договора 770041160025",
		// Valid/invalid INN without document context.
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
		// Arbitrary numbers.
		"12345678",
		"770041160025",
		"621234567",
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
	for _, tc := range cases {
		assertNoIdentity(t, tc)
	}
}

// TestIdentityNoBirthCertificateWithoutContext verifies that a birth
// certificate series without the number, or without document context, is not
// treated as an identity document.
func TestIdentityNoBirthCertificateWithoutContext(t *testing.T) {
	for _, tc := range []string{
		"II-МЮ",
		"серия II-МЮ",
		"II-МЮ №",
		"свидетельство о браке II-МЮ № 123456",
		"справка II-МЮ № 123456",
	} {
		assertNoIdentity(t, tc)
	}
}

// TestIdentityNoMilitaryWithoutContext verifies military-ID-like numbers are
// not documents without military context.
func TestIdentityNoMilitaryWithoutContext(t *testing.T) {
	for _, tc := range []string{
		"ГД 1234567",
		"НЛ 7654321",
		"артикул НЛ 1234567",
		"код товара ГД 1234567",
	} {
		assertNoIdentity(t, tc)
	}
}
