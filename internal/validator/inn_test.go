package validator

import "testing"

func TestValidINNOrganization(t *testing.T) {
	if !IsValidINN("7707083893") {
		t.Fatal("expected valid organization INN to pass")
	}
}

func TestValidINNIndividual(t *testing.T) {
	if !IsValidINN("500100732259") {
		t.Fatal("expected valid individual INN to pass")
	}
}

func TestInvalidINNOrganizationCheckDigit(t *testing.T) {
	if IsValidINN("7707083894") {
		t.Fatal("expected organization INN with wrong check digit to fail")
	}
}

func TestInvalidINNIndividualCheckDigit(t *testing.T) {
	if IsValidINN("500100732250") {
		t.Fatal("expected individual INN with wrong check digit to fail")
	}
}

func TestInvalidLength(t *testing.T) {
	if IsValidINN("770708389") {
		t.Fatal("expected 9-digit INN to fail")
	}
	if IsValidINN("77070838933") {
		t.Fatal("expected 11-digit INN to fail")
	}
	if IsValidINN("770708389333") {
		t.Fatal("expected 13-digit INN to fail")
	}
}

func TestINNEmptyString(t *testing.T) {
	if IsValidINN("") {
		t.Fatal("expected empty string to fail")
	}
}

func TestLettersSpacesHyphensRejected(t *testing.T) {
	if IsValidINN("770708389a") {
		t.Fatal("expected letters to fail")
	}
	if IsValidINN("7707 083893") {
		t.Fatal("expected spaces to fail")
	}
	if IsValidINN("7707-083893") {
		t.Fatal("expected hyphens to fail")
	}
}

func TestINNAllSameDigits(t *testing.T) {
	if IsValidINN("1111111111") {
		t.Fatal("expected 10-digit all-same INN to fail")
	}
	if IsValidINN("111111111111") {
		t.Fatal("expected 12-digit all-same INN to fail")
	}
}
