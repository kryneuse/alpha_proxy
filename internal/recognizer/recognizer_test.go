package recognizer

import (
	"testing"

	"github.com/alpha-proxy/rule-engine/internal/entity"
	"github.com/alpha-proxy/rule-engine/internal/normalize"
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
	// Known person should be skipped.
	spans := findSpans(t, rec, "Александр Сергеевич Пушкин")
	if len(spans) != 0 {
		t.Errorf("expected 0 for known person, got %d", len(spans))
	}
	// Plausible client name with dictionary match.
	spans = findSpans(t, rec, "Иван Петров")
	if len(spans) != 1 {
		t.Errorf("expected 1 name, got %d", len(spans))
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
	// Invalid date.
	spans := findSpans(t, rec, "32.13.2000")
	if len(spans) != 0 {
		t.Errorf("expected 0 for invalid date, got %d", len(spans))
	}
}
