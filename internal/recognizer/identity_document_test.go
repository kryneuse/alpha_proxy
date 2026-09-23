package recognizer_test

import (
	"strings"
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/engine"
	"github.com/kryneuse/alpha_proxy/internal/entity"
)

// analyze runs the full engine (recognizer + scorer + resolver) on text.
func analyze(t *testing.T, text string) []entity.Entity {
	t.Helper()
	e := engine.New(engine.Options{})
	return e.Analyze(text)
}

// findIdentity returns the IDENTITY_DOCUMENT entity in ents, or nil.
func findIdentity(ents []entity.Entity) *entity.Entity {
	for i := range ents {
		if ents[i].Type == entity.IDENTITY_DOCUMENT {
			return &ents[i]
		}
	}
	return nil
}

// assertIdentity checks that exactly one IDENTITY_DOCUMENT entity of the given
// subtype is present, with the exact text and byte offsets.
func assertIdentity(t *testing.T, text string, subtype entity.DocumentSubtype, wantText string) {
	t.Helper()
	ents := analyze(t, text)
	doc := findIdentity(ents)
	if doc == nil {
		t.Fatalf("expected IDENTITY_DOCUMENT/%s in %q, got entities: %+v", subtype, text, ents)
	}
	if doc.Subtype != subtype {
		t.Fatalf("expected subtype %s, got %s in %q", subtype, doc.Subtype, text)
	}
	if doc.Text != wantText {
		t.Fatalf("expected text %q, got %q in %q", wantText, doc.Text, text)
	}
	start := strings.Index(text, wantText)
	if start < 0 {
		t.Fatalf("wantText %q not found in %q", wantText, text)
	}
	if doc.Start != start || doc.End != start+len(wantText) {
		t.Fatalf("expected offsets [%d:%d], got [%d:%d] in %q", start, start+len(wantText), doc.Start, doc.End, text)
	}
	if doc.Score < 0.5 {
		t.Fatalf("expected score >= 0.5, got %f in %q", doc.Score, text)
	}
}

// --- Foreign passport RF ---

func TestIdentityForeignPassport(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"Загранпаспорт 62 1234567", "62 1234567"},
		{"Загранпаспорт 62 № 1234567", "62 № 1234567"},
		{"Загранпаспорт серия 62 номер 1234567", "серия 62 номер 1234567"},
		{"серия загранпаспорта 62 номер 1234567", "серия загранпаспорта 62 номер 1234567"},
		{"Заграничный паспорт 621234567", "621234567"},
		{"паспорт для выезда за границу 62 1234567", "62 1234567"},
		{"номер загранпаспорта 62 1234567", "62 1234567"},
		{"Загранпаспорт 62-1234567", "62-1234567"},
		{"текст до Загранпаспорт 62 № 1234567 текст после", "62 № 1234567"},
	}
	for _, tc := range cases {
		assertIdentity(t, tc.text, entity.ForeignPassportRF, tc.want)
	}
}

func TestIdentityForeignPassportTwoInPayload(t *testing.T) {
	text := "Загранпаспорт 62 № 1234567 и загранпаспорт 71 № 7654321"
	ents := analyze(t, text)
	var docs []entity.Entity
	for _, en := range ents {
		if en.Type == entity.IDENTITY_DOCUMENT {
			docs = append(docs, en)
		}
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 identity documents, got %d: %+v", len(docs), ents)
	}
	if docs[0].Subtype != entity.ForeignPassportRF || docs[1].Subtype != entity.ForeignPassportRF {
		t.Fatalf("expected both foreign passports, got %+v", docs)
	}
}

// --- Birth certificate ---

func TestIdentityBirthCertificate(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"свидетельство о рождении II-МЮ № 123456", "II-МЮ № 123456"},
		{"свидетельство о рождении I-АБ 123456", "I-АБ 123456"},
		{"свидетельство о рождении III-ВГ №123456", "III-ВГ №123456"},
		{"свидетельство о рождении серия II-МЮ, номер 123456", "серия II-МЮ, номер 123456"},
		{"номер свидетельства о рождении II-МЮ № 123456", "II-МЮ № 123456"},
		{"св-во о рождении II-МЮ № 123456", "II-МЮ № 123456"},
		{"Свидетельство о рождении II-МЮ № 123456", "II-МЮ № 123456"},
	}
	for _, tc := range cases {
		assertIdentity(t, tc.text, entity.BirthCertificate, tc.want)
	}
}

// --- Military ID ---

func TestIdentityMilitaryID(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"Военный билет ГД № 1234567", "ГД № 1234567"},
		{"военный билет НЛ № 7654321", "НЛ № 7654321"},
		{"военный билет офицера запаса ГД № 1234567", "ГД № 1234567"},
		{"Временное удостоверение взамен военного билета НЛ № 7654321", "НЛ № 7654321"},
		{"временное удостоверение взамен военного билета офицера запаса ГД № 1234567", "ГД № 1234567"},
		{"Военный билет ГД N 1234567", "ГД N 1234567"},
		{"военный билет ГД 1234567", "ГД 1234567"},
	}
	for _, tc := range cases {
		assertIdentity(t, tc.text, entity.MilitaryID, tc.want)
	}
}

// --- Temporary ID RF (форма 2П) ---

func TestIdentityTemporaryID(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"Временное удостоверение личности форма 2П № 12345678", "12345678"},
		{"Временное удостоверение личности № 770041160025", "770041160025"},
		{"временное удостоверение личности гражданина РФ № 12345678", "12345678"},
		{"форма № 2П 12345678", "12345678"},
		{"Временное удостоверение личности АБ 12345678", "АБ 12345678"},
	}
	for _, tc := range cases {
		assertIdentity(t, tc.text, entity.TemporaryIDRF, tc.want)
	}
}

// --- UTF-8 offsets ---

func TestIdentityUTF8Offsets(t *testing.T) {
	// Cyrillic prefix shifts byte offsets; the document number must map back
	// to the correct original byte range.
	text := "Загранпаспорт 62 № 1234567"
	ents := analyze(t, text)
	doc := findIdentity(ents)
	if doc == nil {
		t.Fatalf("expected identity document in %q", text)
	}
	if text[doc.Start:doc.End] != "62 № 1234567" {
		t.Fatalf("byte slice mismatch: %q", text[doc.Start:doc.End])
	}
}
