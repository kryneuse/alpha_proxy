package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/pii"
)

// Detect реализует pii.BackendDetector поверх rule engine.
func (e *Engine) Detect(ctx context.Context, text string) ([]pii.Entity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	ruleEntities := e.Analyze(text)

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	result := make([]pii.Entity, 0, len(ruleEntities))
	for _, re := range ruleEntities {
		kind, ok := mapKind(re.Type)
		if !ok {
			return nil, errors.New("unknown entity type")
		}

		if re.Start < 0 || re.End > len(text) || re.Start >= re.End {
			return nil, fmt.Errorf("invalid span [%d:%d]: %w", re.Start, re.End, pii.ErrInvalidSpan)
		}
		if text[re.Start:re.End] != re.Text {
			return nil, fmt.Errorf("entity text mismatch at [%d:%d]: %w", re.Start, re.End, pii.ErrInvalidSpan)
		}

		result = append(result, pii.Entity{
			Kind:       kind,
			Start:      re.Start,
			End:        re.End,
			Confidence: re.Score,
			Source:     pii.SourceReg,
		})
		if re.Subtype != "" {
			result[len(result)-1].Metadata = map[string]string{"document_subtype": string(re.Subtype)}
		}
	}

	return result, nil
}

var _ pii.BackendDetector = (*Engine)(nil)

func mapKind(t entity.Type) (pii.PIIKind, bool) {
	switch t {
	case entity.FULL_NAME:
		return pii.PIIKindFullName, true
	case entity.BIRTH_DATE, entity.PASSPORT_ISSUE_DATE:
		return pii.PIIKindDate, true
	case entity.BIRTH_PLACE:
		return pii.PIIKindBirthPlace, true
	case entity.PASSPORT:
		return pii.PIIKindPassport, true
	case entity.CITIZENSHIP:
		return pii.PIIKindCitizenship, true
	case entity.PASSPORT_ISSUER:
		return pii.PIIKindPassportIssuer, true
	case entity.DEPARTMENT_CODE:
		return pii.PIIKindPassportDivision, true
	case entity.DRIVER_LICENSE:
		return pii.PIIKindDriverLicense, true
	case entity.ADDRESS:
		return pii.PIIKindAddress, true
	case entity.EMAIL:
		return pii.PIIKindEmail, true
	case entity.PHONE:
		return pii.PIIKindPhone, true
	case entity.INN:
		return pii.PIIKindINN, true
	case entity.CARD_NUMBER:
		return pii.PIIKindBankCard, true
	case entity.CVV:
		return pii.PIIKindCVV, true
	case entity.PIN:
		return pii.PIIKindPIN, true
	case entity.CARDHOLDER_NAME:
		return pii.PIIKindCardHolderName, true
	case entity.IDENTITY_DOCUMENT:
		return pii.PIIKindIdentityDocument, true
	default:
		return "", false
	}
}
