// Package entity defines the core data types shared across the rule engine:
// entity types, candidate spans and the final typed entities.
package entity

// Type is the identifier of a personal-data entity type.
type Type string

// All supported personal-data entity types.
const (
	FULL_NAME           Type = "FULL_NAME"
	BIRTH_DATE          Type = "BIRTH_DATE"
	BIRTH_PLACE         Type = "BIRTH_PLACE"
	PASSPORT            Type = "PASSPORT"
	CITIZENSHIP         Type = "CITIZENSHIP"
	PASSPORT_ISSUER     Type = "PASSPORT_ISSUER"
	DEPARTMENT_CODE     Type = "DEPARTMENT_CODE"
	PASSPORT_ISSUE_DATE Type = "PASSPORT_ISSUE_DATE"
	DRIVER_LICENSE      Type = "DRIVER_LICENSE"
	ADDRESS             Type = "ADDRESS"
	EMAIL               Type = "EMAIL"
	PHONE               Type = "PHONE"
	INN                 Type = "INN"
	CARD_NUMBER         Type = "CARD_NUMBER"
	CVV                 Type = "CVV"
	PIN                 Type = "PIN"
	CARDHOLDER_NAME     Type = "CARDHOLDER_NAME"
	// Новые типы для V2 (NER v14a). COUNTRY/REGION/DISTRICT — отдельные
	// адресные компоненты; DATE_OF_BIRTH соответствует BIRTH_DATE, а
	// PASSPORT_ISSUE_DATE уже существует.
	COUNTRY  Type = "COUNTRY"
	REGION   Type = "REGION"
	DISTRICT Type = "DISTRICT"
	// Адресные компоненты, которые NER v14a выдаёт отдельно от ADDRESS.
	// Сохраняются до построения плана замен, чтобы не терять покрытие.
	CITY       Type = "CITY"
	STREET     Type = "STREET"
	HOUSE      Type = "HOUSE"
	APARTMENT  Type = "APARTMENT"
	POSTAL_CODE Type = "POSTAL_CODE"
)

// AllTypes returns every supported entity type.
func AllTypes() []Type {
	return []Type{
		FULL_NAME, BIRTH_DATE, BIRTH_PLACE, PASSPORT, CITIZENSHIP,
		PASSPORT_ISSUER, DEPARTMENT_CODE, PASSPORT_ISSUE_DATE, DRIVER_LICENSE,
		ADDRESS, EMAIL, PHONE, INN, CARD_NUMBER, CVV, PIN, CARDHOLDER_NAME,
		COUNTRY, REGION, DISTRICT, CITY, STREET, HOUSE, APARTMENT, POSTAL_CODE,
	}
}

// Source describes how a candidate was produced.
type Source string

const (
	SourceRegex      Source = "regex"
	SourceChecksum   Source = "checksum"
	SourceDictionary Source = "dictionary"
	SourceContext    Source = "context"
	SourceFormat     Source = "format"
)

// CandidateSpan is a raw recognition result produced by a recognizer.
// Offsets are always relative to the ORIGINAL input text.
type CandidateSpan struct {
	Type       Type
	Text       string
	Start      int
	End        int
	Score      float64
	Sources    []Source
	Reason     string
	Contextual bool
}

// EvidenceStrength ranks how strong the evidence for a candidate is.
// Higher is stronger. Checksum/validated + context is the strongest signal;
// a bare regex match is the weakest.
func (c CandidateSpan) EvidenceStrength() int {
	hasChecksum := false
	hasContext := false
	hasDict := false
	for _, s := range c.Sources {
		switch s {
		case SourceChecksum:
			hasChecksum = true
		case SourceContext:
			hasContext = true
		case SourceDictionary:
			hasDict = true
		}
	}
	switch {
	case hasChecksum && hasContext:
		return 5
	case hasChecksum:
		return 4
	case hasDict && hasContext:
		return 3
	case hasDict:
		return 2
	case hasContext:
		return 2
	default:
		return 1
	}
}

// Entity is the final, resolved personal-data entity returned by the engine.
type Entity struct {
	Type   Type
	Text   string
	Start  int
	End    int
	Score  float64
	Reason string
}
