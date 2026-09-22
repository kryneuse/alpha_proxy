package recognizer

import (
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/normalize"
)

func findSpans(t *testing.T, rec Recognizer, text string) []entity.CandidateSpan {
	t.Helper()
	return rec.Recognize(normalize.New(text))
}

func TestEmailRecognizer(t *testing.T) {
	rec := NewEmailRecognizer()
	spans := findSpans(t, rec, "Свяжитесь: ivan.petrov@example.com")
	if len(spans) != 1 {
		t.Fatalf("expected 1 email, got %d", len(spans))
	}
	if spans[0].Text != "ivan.petrov@example.com" {
		t.Errorf("unexpected text %q", spans[0].Text)
	}
	if spans[0].Start != 20 || spans[0].End != 43 {
		t.Errorf("unexpected offsets %d-%d", spans[0].Start, spans[0].End)
	}
}

func TestPhoneRecognizer(t *testing.T) {
	rec := NewPhoneRecognizer()
	for _, tc := range []string{
		"+7 (912) 345-67-89",
		"8 912 345 67 89",
		"+79123456789",
	} {
		spans := findSpans(t, rec, "тел: "+tc)
		if len(spans) != 1 {
			t.Errorf("expected 1 phone for %q, got %d", tc, len(spans))
		}
	}
}

func TestInnRecognizer(t *testing.T) {
	rec := NewInnRecognizer()
	spans := findSpans(t, rec, "ИНН 7707083893")
	if len(spans) != 1 {
		t.Fatalf("expected 1 INN, got %d", len(spans))
	}
	if spans[0].Type != entity.INN {
		t.Errorf("unexpected type %s", spans[0].Type)
	}
	// Invalid checksum should be rejected.
	spans = findSpans(t, rec, "ИНН 7707083894")
	if len(spans) != 0 {
		t.Errorf("expected 0 INN for invalid checksum, got %d", len(spans))
	}
}

func TestCardNumberRecognizer(t *testing.T) {
	rec := NewCardNumberRecognizer()
	spans := findSpans(t, rec, "номер карты 4532 0151 1283 0366")
	if len(spans) != 1 {
		t.Fatalf("expected 1 card, got %d", len(spans))
	}
	// Sequential digits must NOT be a card.
	spans = findSpans(t, rec, "номер заказа 1234567890123456")
	if len(spans) != 0 {
		t.Errorf("expected 0 cards for sequential digits, got %d", len(spans))
	}
}

func TestCvvRecognizer(t *testing.T) {
	rec := NewCvvRecognizer()
	// Bare 3 digits are candidates but low score; context decides.
	spans := findSpans(t, rec, "CVV 123")
	if len(spans) != 1 {
		t.Fatalf("expected 1 CVV candidate, got %d", len(spans))
	}
	if spans[0].Score > 0.5 {
		t.Errorf("bare CVV candidate should have low score, got %f", spans[0].Score)
	}
}

func TestPinRecognizer(t *testing.T) {
	rec := NewPinRecognizer()
	spans := findSpans(t, rec, "пин-код 7305")
	if len(spans) != 1 {
		t.Fatalf("expected 1 PIN candidate, got %d", len(spans))
	}
	if spans[0].Score > 0.5 {
		t.Errorf("bare PIN candidate should have low score, got %f", spans[0].Score)
	}
}

func TestPassportRecognizer(t *testing.T) {
	rec := NewPassportRecognizer()
	spans := findSpans(t, rec, "паспорт 4510 123456")
	if len(spans) != 1 {
		t.Fatalf("expected 1 passport, got %d", len(spans))
	}
	if spans[0].Text != "4510 123456" {
		t.Errorf("unexpected text %q", spans[0].Text)
	}
}

func TestDepartmentCodeRecognizer(t *testing.T) {
	rec := NewDepartmentCodeRecognizer()
	spans := findSpans(t, rec, "код подразделения 770-001")
	if len(spans) != 1 {
		t.Fatalf("expected 1 department code, got %d", len(spans))
	}
}

func TestCitizenshipRecognizer(t *testing.T) {
	rec := NewCitizenshipRecognizer(nil)
	spans := findSpans(t, rec, "гражданство: Российская Федерация")
	if len(spans) != 1 {
		t.Fatalf("expected 1 citizenship, got %d", len(spans))
	}
	if spans[0].Text != "Российская Федерация" {
		t.Errorf("unexpected text %q", spans[0].Text)
	}
	// Case-ending tolerance: "Казахстана" should match "Казахстан".
	spans = findSpans(t, rec, "Гражданин Казахстана")
	if len(spans) != 1 || spans[0].Text != "Казахстана" {
		t.Errorf("expected citizenship 'Казахстана', got %+v", spans)
	}
	// Punctuation tolerance: "РФ." should match "РФ".
	spans = findSpans(t, rec, "Гражданство: РФ.")
	if len(spans) != 1 || spans[0].Text != "РФ" {
		t.Errorf("expected citizenship 'РФ', got %+v", spans)
	}
}

func TestBirthPlaceRecognizer(t *testing.T) {
	rec := NewBirthPlaceRecognizer()
	spans := findSpans(t, rec, "место рождения: город Москва")
	if len(spans) != 1 {
		t.Fatalf("expected 1 birth place, got %d", len(spans))
	}
}

func TestFullNameRecognizer(t *testing.T) {
	rec := NewFullNameRecognizer(nil, nil, nil)
	// A known person is emitted by the recognizer but with a reduced score;
	// the context scorer suppresses it in literary context. Here we only
	// verify the recognizer still finds the name tokens.
	spans := findSpans(t, rec, "Александр Сергеевич Пушкин")
	if len(spans) != 1 {
		t.Errorf("expected 1 name candidate for known person, got %d", len(spans))
	}
	// Plausible client name with dictionary match.
	spans = findSpans(t, rec, "Иван Петров")
	if len(spans) != 1 {
		t.Errorf("expected 1 name, got %d", len(spans))
	}
	// Leading context word and trailing ordinary word must not be included.
	spans = findSpans(t, rec, "Сотрудник Иван Петров подал заявку")
	if len(spans) != 1 || spans[0].Text != "Иван Петров" {
		t.Errorf("expected span 'Иван Петров', got %+v", spans)
	}
	spans = findSpans(t, rec, "Иван Петров гулял в парке")
	if len(spans) != 1 || spans[0].Text != "Иван Петров" {
		t.Errorf("expected span 'Иван Петров', got %+v", spans)
	}
}

func TestAddressRecognizer(t *testing.T) {
	rec := NewAddressRecognizer(nil)
	spans := findSpans(t, rec, "адрес: г. Москва, ул. Тверская, д. 10, кв. 5")
	if len(spans) != 1 {
		t.Fatalf("expected 1 address, got %d", len(spans))
	}
	// A bare city name should not be an address.
	spans = findSpans(t, rec, "Москва")
	if len(spans) != 0 {
		t.Errorf("expected 0 for bare city, got %d", len(spans))
	}
}

func TestDateRecognizer(t *testing.T) {
	rec := NewDateRecognizer()
	for _, tc := range []string{"01.02.2000", "01-02-2000", "01/02/2000", "15 марта 1990"} {
		spans := findSpans(t, rec, "дата: "+tc)
		if len(spans) != 1 {
			t.Errorf("expected 1 date for %q, got %d", tc, len(spans))
		}
	}
	// Invalid dates.
	for _, tc := range []string{"32.13.2000", "31.02.2000", "30.02.2001", "31.04.2000"} {
		spans := findSpans(t, rec, "дата: "+tc)
		if len(spans) != 0 {
			t.Errorf("expected 0 for invalid date %q, got %d", tc, len(spans))
		}
	}
	// Leap year: Feb 29 is valid in 2000, invalid in 2001.
	if spans := findSpans(t, rec, "дата: 29.02.2000"); len(spans) != 1 {
		t.Errorf("expected 1 for leap-year date 29.02.2000, got %d", len(spans))
	}
	if spans := findSpans(t, rec, "дата: 29.02.2001"); len(spans) != 0 {
		t.Errorf("expected 0 for non-leap-year date 29.02.2001, got %d", len(spans))
	}
}
