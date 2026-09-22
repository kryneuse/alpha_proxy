package engine

import (
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/entity"
)

// TestHardNegativeCases asserts that specific non-personal inputs produce no
// entities.
func TestHardNegativeCases(t *testing.T) {
	e := New(Options{})
	cases := []string{
		// Known persons in literary/historical context.
		"Александр Сергеевич Пушкин — великий русский поэт.",
		"Лев Николаевич Толстой написал роман «Война и мир».",
		"Поэт Александр Сергеевич Пушкин родился в Москве.",
		"Владимир Путин — президент России.",
		"Юрий Гагарин — первый космонавт.",
		"Пётр Чайковский — великий русский композитор.",
		// Public/bank branch addresses.
		"Отделение банка находится по адресу: г. Москва, ул. Тверская, д. 1.",
		"Офис банка расположен по адресу: г. Казань, ул. Баумана, д. 5.",
		"Магазин находится по адресу: г. Москва, ул. Тверская, д. 1.",
		"Адрес ресторана: г. Москва, ул. Тверская, д. 1.",
		// Order/article/document numbers that are not passports.
		"Номер заказа 1234567890123456.",
		"Номер заказа 1234567890.",
		"Артикул 4510123456.",
		"Артикул 7707083893.",
		"Артикул 500100732259.",
		"Номер документа 4510 123456.",
		"Номер накладной 1234567890.",
		"Трек-номер 1234567890.",
		"Номер договора 1234567890.",
		"Счёт-фактура 1234567890.",
		"Квитанция 1234567890.",
		"Номер заявки 1234567890.",
		"Номер партии 1234567890.",
		"Серийный номер 4510123456.",
		"Номер счёта 1234567890.",
		// Department codes without passport context.
		"Код 770-001 указан в заявке.",
		"Код товара 770-001.",
		"Партия товара 770-001.",
		// PIN/CVV without banking context.
		"Код доступа 7305.",
		"Пароль 1234.",
		"Код подтверждения 1234.",
		"Код клиента 1234.",
		// Auditorium/room numbers that are not CVV.
		"Лекция пройдёт в аудитории 314.",
		"Кабинет 314.",
		"Комната 314.",
		"Офис 314.",
		"Помещение 314.",
		// Event dates that are not birth dates.
		"Конференция состоится 15.03.2025.",
		"Встреча назначена на 20.04.2025 в 15:00.",
		"Дата рождения 31.02.2000.",
		// Bare country mentions that are not citizenship.
		"Россия — крупнейшая страна мира.",
		"Германия — страна в центре Европы.",
		// Metro/transport cards that are not bank cards.
		"Карта метро 4532 0151 1283 0366.",
		"Транспортная карта 4916 1197 1130 4546.",
		// Public organization contacts.
		"Служба поддержки: 8 800 123 45 67.",
		"Горячая линия: +7 (495) 123-45-67.",
		"Наш email: support@example.com.",
		"Обращайтесь по телефону 8 800 555 35 35.",
		"Телефон офиса: +7 (495) 123-45-67.",
		"Email компании: info@company.ru.",
		"Email отдела продаж: sales@company.ru.",
		// Cardholder without banking context.
		"hello world",
		"Open AI platform",
		// Loyalty/transport cards that are not bank cards.
		"Карта лояльности 4111 1111 1111 1111.",
		// Non-bank CVV/PIN.
		"Код безопасности 123 для замка.",
		"PIN 1234 от телефона.",
	}
	for _, c := range cases {
		got := e.Analyze(c)
		if len(got) > 0 {
			t.Errorf("expected no entities for %q, got %+v", c, got)
		}
	}
}

// TestPositiveCases asserts that specific personal-data inputs produce the
// expected entity types.
func TestPositiveCases(t *testing.T) {
	e := New(Options{})
	cases := []struct {
		text string
		typ  entity.Type
	}{
		{"Клиент живет по адресу: г. Москва, ул. Тверская, д. 10, кв. 5", entity.ADDRESS},
		{"Гражданство заявителя Россия", entity.CITIZENSHIP},
		{"Гражданин Казахстана", entity.CITIZENSHIP},
		{"CVV 123", entity.CVV},
		{"Пин-код 7305", entity.PIN},
		{"Дата рождения: 1990-03-15", entity.BIRTH_DATE},
		{"Родился в 1990 году в Москве", entity.BIRTH_PLACE},
		{"Россия — страна. Гражданство клиента Россия", entity.CITIZENSHIP},
		{"Паспорт: серия 4510, номер 123456", entity.PASSPORT},
	}
	for _, c := range cases {
		got := e.Analyze(c.text)
		found := false
		for _, g := range got {
			if g.Type == c.typ {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s in %q, got %+v", c.typ, c.text, got)
		}
	}
}

// TestBareNumbersNotDetected asserts that bare 3/4-digit numbers without
// banking context are not CVV/PIN.
func TestBareNumbersNotDetected(t *testing.T) {
	e := New(Options{})
	for _, c := range []string{"123", "7305", "314", "1234"} {
		got := e.Analyze(c)
		if len(got) > 0 {
			t.Errorf("expected no entities for %q, got %+v", c, got)
		}
	}
}

// TestCitizenshipSecondOccurrence asserts that the correct (second) "Россия"
// is detected, not the first occurrence via strings.Index.
func TestCitizenshipSecondOccurrence(t *testing.T) {
	e := New(Options{})
	text := "Россия — страна. Гражданство клиента Россия"
	got := e.Analyze(text)
	if len(got) != 1 || got[0].Type != entity.CITIZENSHIP {
		t.Fatalf("expected 1 CITIZENSHIP, got %+v", got)
	}
	// The detected span must be the second "Россия" (offset 69), not the first.
	if got[0].Text != "Россия" || got[0].Start != 69 {
		t.Errorf("expected second 'Россия' at offset 69, got %+v", got[0])
	}
	if text[got[0].Start:got[0].End] != got[0].Text {
		t.Errorf("offset mismatch: %q != %q", text[got[0].Start:got[0].End], got[0].Text)
	}
}

// TestISODateRecognized asserts that an ISO date is recognized as a whole.
func TestISODateRecognized(t *testing.T) {
	e := New(Options{})
	text := "Дата рождения: 1990-03-15"
	got := e.Analyze(text)
	if len(got) != 1 || got[0].Type != entity.BIRTH_DATE {
		t.Fatalf("expected 1 BIRTH_DATE, got %+v", got)
	}
	if got[0].Text != "1990-03-15" {
		t.Errorf("expected full ISO date, got %q", got[0].Text)
	}
}

// TestBirthPlaceWithYear asserts that a birth place after a year is captured.
func TestBirthPlaceWithYear(t *testing.T) {
	e := New(Options{})
	text := "Родился в 1990 году в Москве"
	got := e.Analyze(text)
	hasPlace := false
	for _, g := range got {
		if g.Type == entity.BIRTH_PLACE && g.Text == "Москве" {
			hasPlace = true
		}
	}
	if !hasPlace {
		t.Errorf("expected BIRTH_PLACE 'Москве', got %+v", got)
	}
}
