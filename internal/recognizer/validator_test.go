package recognizer

import "testing"

func TestInn10(t *testing.T) {
	// Valid 10-digit INN.
	if !Inn10("7707083893") {
		t.Error("expected valid INN 7707083893")
	}
	// Invalid checksum.
	if Inn10("7707083894") {
		t.Error("expected invalid INN 7707083894")
	}
	if Inn10("123") {
		t.Error("expected invalid short INN")
	}
	// All same digits.
	for _, n := range []string{"0000000000", "1111111111"} {
		if Inn10(n) {
			t.Errorf("expected invalid INN %s", n)
		}
	}
}

func TestInn12(t *testing.T) {
	// Valid 12-digit INN.
	if !Inn12("500100732259") {
		t.Error("expected valid INN 500100732259")
	}
	if Inn12("500100732258") {
		t.Error("expected invalid INN 500100732258")
	}
	// All same digits.
	for _, n := range []string{"000000000000", "111111111111"} {
		if Inn12(n) {
			t.Errorf("expected invalid INN %s", n)
		}
	}
}

func TestLuhn(t *testing.T) {
	// Valid card numbers.
	for _, n := range []string{"4532015112830366", "4916119711304546", "4485275742308327"} {
		if !Luhn(n) {
			t.Errorf("expected valid Luhn for %s", n)
		}
	}
	// Invalid.
	if Luhn("4532015112830367") {
		t.Error("expected invalid Luhn")
	}
	if Luhn("1234567890123456") {
		t.Error("expected invalid Luhn for sequential digits")
	}
	// All same digits.
	if Luhn("0000000000000000") {
		t.Error("expected invalid Luhn for all same digits")
	}
}
