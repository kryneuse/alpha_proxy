package recognizer_test

import (
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/entity"
)

// TestIdentityOverlapTemporaryIDvsINN verifies that a 12-digit number that is a
// valid INN is treated as the temporary ID when in explicit document context,
// and as INN otherwise.
func TestIdentityOverlapTemporaryIDvsINN(t *testing.T) {
	// Valid INN12 in document context -> document wins.
	ents := analyze(t, "Временное удостоверение личности № 500100732259")
	doc := findIdentity(ents)
	if doc == nil {
		t.Fatalf("expected temporary ID in document context, got %+v", ents)
	}
	if doc.Subtype != entity.TemporaryIDRF {
		t.Fatalf("expected temporary_id_rf, got %s", doc.Subtype)
	}

	// Same number without document context -> INN preserved.
	ents = analyze(t, "ИНН 500100732259")
	for _, en := range ents {
		if en.Type == entity.IDENTITY_DOCUMENT {
			t.Fatalf("expected no identity document for bare INN, got %+v", ents)
		}
	}
	foundINN := false
	for _, en := range ents {
		if en.Type == entity.INN {
			foundINN = true
		}
	}
	if !foundINN {
		t.Fatalf("expected INN preserved without document context, got %+v", ents)
	}
}

// TestIdentityOverlapForeignPassportVsGeneric verifies that a 9-digit number is
// not treated as a foreign passport without context, even if it overlaps a
// generic numeric candidate.
func TestIdentityOverlapForeignPassportVsGeneric(t *testing.T) {
	for _, tc := range []string{
		"Заказ 621234567",
		"Номер договора 62 1234567",
		"Артикул 621234567",
	} {
		ents := analyze(t, tc)
		for _, en := range ents {
			if en.Type == entity.IDENTITY_DOCUMENT {
				t.Fatalf("expected no identity document in %q, got %+v", tc, ents)
			}
		}
	}
}

// TestIdentityOverlapMilitaryVsName verifies that a military-ID-like number
// near a name is not treated as a document without military context.
func TestIdentityOverlapMilitaryVsName(t *testing.T) {
	ents := analyze(t, "Иванов Иван ГД 1234567")
	for _, en := range ents {
		if en.Type == entity.IDENTITY_DOCUMENT {
			t.Fatalf("expected no identity document near name without military context, got %+v", ents)
		}
	}
}

// TestIdentityOverlapBirthCertificateVsGeneric verifies that a birth-certificate
// series without document context is not treated as a document.
func TestIdentityOverlapBirthCertificateVsGeneric(t *testing.T) {
	for _, tc := range []string{
		"II-МЮ № 123456",
		"серия II-МЮ, номер 123456",
		"справка II-МЮ № 123456",
	} {
		ents := analyze(t, tc)
		for _, en := range ents {
			if en.Type == entity.IDENTITY_DOCUMENT {
				t.Fatalf("expected no identity document in %q, got %+v", tc, ents)
			}
		}
	}
}

// TestIdentityOverlapTemporaryID8Digits verifies that 8 digits are only a
// temporary ID with strong context, not a generic number.
func TestIdentityOverlapTemporaryID8Digits(t *testing.T) {
	// With context -> temporary ID.
	ents := analyze(t, "Временное удостоверение личности № 12345678")
	if findIdentity(ents) == nil {
		t.Fatalf("expected temporary ID with context, got %+v", ents)
	}
	// Without context -> not a document.
	ents = analyze(t, "Заказ 12345678")
	for _, en := range ents {
		if en.Type == entity.IDENTITY_DOCUMENT {
			t.Fatalf("expected no identity document for bare 8 digits, got %+v", ents)
		}
	}
}
