package ml

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"unicode/utf8"

	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/pii"
)

// Extractor адаптирует Batcher под cascade.ExpensiveExtractor.
type Extractor struct {
	batcher *Batcher
	seq     atomic.Uint64
}

// NewExtractor создаёт Extractor.
func NewExtractor(batcher *Batcher) (*Extractor, error) {
	if batcher == nil {
		return nil, errors.New("batcher must not be nil")
	}
	return &Extractor{batcher: batcher}, nil
}

// Detect находит персональные данные в тексте через Batcher.
func (e *Extractor) Detect(ctx context.Context, text string) ([]entity.Entity, error) {
	if text == "" {
		return nil, nil
	}

	chunkID := fmt.Sprintf("chunk-%d", e.seq.Add(1))
	result, err := e.batcher.Process(ctx, RequestItem{ChunkID: chunkID, Text: text})
	if err != nil {
		return nil, err
	}

	byteOffsets := buildByteOffsets(text)
	runes := []rune(text)

	out := make([]entity.Entity, 0, len(result.Entities))
	for _, me := range result.Entities {
		typ, ok := mapMLType(me.Type)
		if !ok {
			return nil, fmt.Errorf("unknown ml type: %w", pii.ErrInvalidMLResponse)
		}
		if me.Start < 0 || me.End > len(runes) || me.Start >= me.End {
			return nil, fmt.Errorf("invalid unicode span [%d:%d]: %w", me.Start, me.End, pii.ErrInvalidMLResponse)
		}
		startByte := byteOffsets[me.Start]
		endByte := byteOffsets[me.End]

		out = append(out, entity.Entity{
			Type:   typ,
			Text:   text[startByte:endByte],
			Start:  startByte,
			End:    endByte,
			Score:  me.Confidence,
			Reason: "ml",
		})
	}

	return out, nil
}

func buildByteOffsets(text string) []int {
	offsets := make([]int, 0, len([]rune(text))+1)
	off := 0
	for _, r := range text {
		offsets = append(offsets, off)
		off += utf8.RuneLen(r)
	}
	offsets = append(offsets, len(text))
	return offsets
}

func mapMLType(t string) (entity.Type, bool) {
	switch t {
	case "full_name", "first_name", "last_name", "patronymic":
		return entity.FULL_NAME, true
	case "address", "city", "street", "house", "apartment", "postal_code":
		return entity.ADDRESS, true
	case "place_of_birth":
		return entity.BIRTH_PLACE, true
	case "citizenship":
		return entity.CITIZENSHIP, true
	case "passport_issuer":
		return entity.PASSPORT_ISSUER, true
	case "cardholder_name":
		return entity.CARDHOLDER_NAME, true
	case "email":
		return entity.EMAIL, true
	case "phone":
		return entity.PHONE, true
	case "inn":
		return entity.INN, true
	case "passport":
		return entity.PASSPORT, true
	case "department_code":
		return entity.DEPARTMENT_CODE, true
	case "driver_license":
		return entity.DRIVER_LICENSE, true
	case "cvv":
		return entity.CVV, true
	case "pin":
		return entity.PIN, true
	case "card":
		return entity.CARD_NUMBER, true
	case "date":
		return entity.BIRTH_DATE, true
	default:
		return "", false
	}
}
