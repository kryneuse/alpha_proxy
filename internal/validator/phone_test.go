package validator

import "testing"

func TestPhonePlusSeven(t *testing.T) {
	got, ok := NormalizePhone("+7 912 345-67-89")
	if !ok {
		t.Fatal("expected valid +7 number")
	}
	if got != "+79123456789" {
		t.Fatalf("expected +79123456789, got %q", got)
	}
}

func TestPhoneEight(t *testing.T) {
	got, ok := NormalizePhone("8 (912) 345-67-89")
	if !ok {
		t.Fatal("expected valid 8 number")
	}
	if got != "+79123456789" {
		t.Fatalf("expected +79123456789, got %q", got)
	}
}

func TestPhoneTenDigits(t *testing.T) {
	got, ok := NormalizePhone("9123456789")
	if !ok {
		t.Fatal("expected valid 10-digit number")
	}
	if got != "+79123456789" {
		t.Fatalf("expected +79123456789, got %q", got)
	}
}

func TestPhoneInvalidLength(t *testing.T) {
	if _, ok := NormalizePhone("912345678"); ok {
		t.Fatal("expected 9-digit number to fail")
	}
	if _, ok := NormalizePhone("91234567890"); ok {
		t.Fatal("expected 12-digit number to fail")
	}
}

func TestPhoneLettersRejected(t *testing.T) {
	if _, ok := NormalizePhone("+7 912 345-67-8a"); ok {
		t.Fatal("expected letters to fail")
	}
}

func TestPhoneInvalidFirstCode(t *testing.T) {
	if _, ok := NormalizePhone("6123456789"); ok {
		t.Fatal("expected 10-digit number not starting with 9 to fail")
	}
	if _, ok := NormalizePhone("+6 912 345-67-89"); ok {
		t.Fatal("expected 11-digit number not starting with 7/8 to fail")
	}
}

func TestPhonePlusNotAtStart(t *testing.T) {
	if _, ok := NormalizePhone("9123456789+"); ok {
		t.Fatal("expected plus not at start to fail")
	}
}

func TestPhonePlusEightRejected(t *testing.T) {
	if _, ok := NormalizePhone("+8 912 345 67 89"); ok {
		t.Fatal("expected +8 number to fail")
	}
}

func TestPhonePlusTenDigitsRejected(t *testing.T) {
	if _, ok := NormalizePhone("+9123456789"); ok {
		t.Fatal("expected +9 10-digit number to fail")
	}
}

func TestPhoneSecondPlusRejected(t *testing.T) {
	if _, ok := NormalizePhone("+7 912 345 67 8+9"); ok {
		t.Fatal("expected second plus to fail")
	}
}

func TestPhoneOnlySeparatorsRejected(t *testing.T) {
	if _, ok := NormalizePhone("  - ( ) "); ok {
		t.Fatal("expected string of only separators to fail")
	}
}
