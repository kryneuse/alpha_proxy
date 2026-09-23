package pii

import "time"

type PIIKind string

const (
	PIIKindFullName         PIIKind = "full_name"
	PIIKindFirstName        PIIKind = "first_name"
	PIIKindLastName         PIIKind = "last_name"
	PIIKindMiddleName       PIIKind = "patronymic"
	PIIKindAddress          PIIKind = "address"
	PIIKindCity             PIIKind = "city"
	PIIKindStreet           PIIKind = "street"
	PIIKindHouse            PIIKind = "house"
	PIIKindApartment        PIIKind = "apartment"
	PIIKindBirthPlace       PIIKind = "place_of_birth"
	PIIKindCitizenship      PIIKind = "citizenship"
	PIIKindPassportIssuer   PIIKind = "passport_issuer"
	PIIKindCardHolderName   PIIKind = "cardholder_name"
	PIIKindEmail            PIIKind = "email"
	PIIKindPhone            PIIKind = "phone"
	PIIKindINN              PIIKind = "inn"
	PIIKindBankCard         PIIKind = "card"
	PIIKindPassport         PIIKind = "passport"
	PIIKindPassportDivision PIIKind = "department_code"
	PIIKindDate             PIIKind = "date"
	PIIKindDriverLicense    PIIKind = "driver_license"
	PIIKindCVV              PIIKind = "cvv"
	PIIKindPIN              PIIKind = "pin"
	PIIKindPostalCode       PIIKind = "postal_code"
	// PIIKindIdentityDocument is a bonus kind covering identity documents other
	// than the Russian passport (foreign passport RF, birth certificate,
	// military ID, temporary ID). The concrete subtype is carried in metadata.
	PIIKindIdentityDocument PIIKind = "identity_document"
)

type Source string

const (
	SourceML  Source = "ml"
	SourceReg Source = "regex"
)

type Replacement struct {
	Start      int
	End        int
	Token      string
	Original   string
	Kind       PIIKind
	Source     Source
	Confidence float64
}

type TokenMapping struct {
	Token    string
	Original string
	Kind     PIIKind
	Source   Source
	Start    int
	End      int
}

type SessionStatus string

const (
	SessionStatusProcessing SessionStatus = "PROCESSING"
	SessionStatusReady      SessionStatus = "READY"
	SessionStatusFailed     SessionStatus = "FAILED"
)

type Session struct {
	PayloadID   string
	PayloadHash [32]byte
	Mappings    []TokenMapping
	Status      SessionStatus
	CreatedAt   time.Time
	ExpiresAt   time.Time
}
