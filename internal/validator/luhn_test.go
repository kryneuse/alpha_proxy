package validator

import "testing"

func TestValidCardNumber(t *testing.T) {
	if !IsValidLuhn("4111 1111 1111 1111") {
		t.Fatal("expected valid card number to pass")
	}
}

func TestValidCardNumberWithHyphens(t *testing.T) {
	if !IsValidLuhn("4111-1111-1111-1111") {
		t.Fatal("expected card number with hyphens to pass")
	}
}

func TestInvalidCheckDigit(t *testing.T) {
	if IsValidLuhn("4111 1111 1111 1112") {
		t.Fatal("expected card number with wrong check digit to fail")
	}
}

func TestTooShort(t *testing.T) {
	if IsValidLuhn("4111 1111 1111") {
		t.Fatal("expected too short number to fail")
	}
}

func TestTooLong(t *testing.T) {
	if IsValidLuhn("4111 1111 1111 1111 1111 1") {
		t.Fatal("expected too long number to fail")
	}
}

func TestLettersRejected(t *testing.T) {
	if IsValidLuhn("4111 1111 1111 111a") {
		t.Fatal("expected letters to make number invalid")
	}
}

func TestOtherInvalidCharacters(t *testing.T) {
	if IsValidLuhn("4111_1111_1111_1111") {
		t.Fatal("expected underscores to make number invalid")
	}
}

func TestEmptyString(t *testing.T) {
	if IsValidLuhn("") {
		t.Fatal("expected empty string to fail")
	}
}

func TestAllSameDigits(t *testing.T) {
	if IsValidLuhn("1111 1111 1111 1111") {
		t.Fatal("expected number of all same digits to fail")
	}
}
