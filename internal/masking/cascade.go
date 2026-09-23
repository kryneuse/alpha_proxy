package masking

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/kryneuse/alpha_proxy/internal/cascade"
	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/ml"
	"github.com/kryneuse/alpha_proxy/internal/pii"
	"github.com/kryneuse/alpha_proxy/internal/tokenizer"
)

// runner выполняет cascade для одного чанка.
type runner interface {
	Run(ctx context.Context, text string) (cascade.Result, error)
}

// CascadeMasker маскирует текст через cascade, разбивая его на чанки и
// обрабатывая их параллельно.
type CascadeMasker struct {
	runner      runner
	chunkCfg    ml.ChunkConfig
	maxParallel int
}

// NewCascadeMasker создаёт CascadeMasker.
func NewCascadeMasker(r runner, chunkCfg ml.ChunkConfig, maxParallel int) (*CascadeMasker, error) {
	if r == nil {
		return nil, errors.New("runner must not be nil")
	}
	if chunkCfg.TargetCodePoints <= 0 || chunkCfg.MaxCodePoints <= 0 ||
		chunkCfg.TargetCodePoints > chunkCfg.MaxCodePoints ||
		chunkCfg.OverlapCodePoints < 0 || chunkCfg.OverlapCodePoints >= chunkCfg.MaxCodePoints {
		return nil, errors.New("invalid chunk config")
	}
	if maxParallel <= 0 {
		return nil, errors.New("max parallel must be > 0")
	}
	return &CascadeMasker{runner: r, chunkCfg: chunkCfg, maxParallel: maxParallel}, nil
}

type chunkEntities struct {
	ml      []pii.Entity
	backend []pii.Entity
}

type chunkJob struct {
	index int
	chunk ml.TextChunk
}

// Mask разбивает текст на чанки, обрабатывает их параллельно и собирает
// итоговый masked текст.
func (m *CascadeMasker) Mask(ctx context.Context, text string, policy pii.Policy) (string, []pii.TokenMapping, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}

	chunks, err := ml.SplitText(ctx, text, m.chunkCfg)
	if err != nil {
		return "", nil, err
	}
	if len(chunks) == 0 {
		return "", nil, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan chunkJob)
	results := make([]chunkEntities, len(chunks))

	var firstErr error
	var errOnce sync.Once
	setErr := func(err error) {
		errOnce.Do(func() { firstErr = err })
	}

	var wg sync.WaitGroup
	for i := 0; i < m.maxParallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				entities, err := m.processChunk(ctx, job.chunk)
				if err != nil {
					setErr(err)
					cancel()
					return
				}
				results[job.index] = entities
			}
		}()
	}

sendLoop:
	for i, chunk := range chunks {
		select {
		case jobs <- chunkJob{index: i, chunk: chunk}:
		case <-ctx.Done():
			break sendLoop
		}
	}
	close(jobs)
	wg.Wait()

	if firstErr != nil {
		return "", nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}

	var mlEntities, backendEntities []pii.Entity
	for _, r := range results {
		mlEntities = append(mlEntities, r.ml...)
		backendEntities = append(backendEntities, r.backend...)
	}

	mlPlan, err := tokenizer.BuildReplacementPlan(ctx, text, mlEntities, policy)
	if err != nil {
		return "", nil, err
	}
	backendPlan, err := tokenizer.BuildReplacementPlan(ctx, text, backendEntities, policy)
	if err != nil {
		return "", nil, err
	}
	combined, err := tokenizer.CombinePlans(ctx, text, mlPlan, backendPlan, policy)
	if err != nil {
		return "", nil, err
	}
	masked, mappings, err := tokenizer.ApplyReplacements(ctx, text, combined)
	if err != nil {
		return "", nil, err
	}

	return masked, mappings, nil
}

func (m *CascadeMasker) processChunk(ctx context.Context, chunk ml.TextChunk) (chunkEntities, error) {
	res, err := m.runner.Run(ctx, chunk.Text)
	if err != nil {
		return chunkEntities{}, err
	}

	var out chunkEntities
	for _, en := range res.Entities {
		kind, ok := mapEntityType(en.Type)
		if !ok {
			return chunkEntities{}, errors.New("unknown entity type")
		}
		if en.Start < 0 || en.End > len(chunk.Text) || en.Start >= en.End {
			return chunkEntities{}, fmt.Errorf("invalid span [%d:%d]: %w", en.Start, en.End, pii.ErrInvalidSpan)
		}
		if chunk.Text[en.Start:en.End] != en.Text {
			return chunkEntities{}, fmt.Errorf("entity text mismatch at [%d:%d]: %w", en.Start, en.End, pii.ErrInvalidSpan)
		}

		source := pii.SourceReg
		if en.Reason == "ml" {
			source = pii.SourceML
		}
		pe := pii.Entity{
			Kind:       kind,
			Start:      chunk.StartByte + en.Start,
			End:        chunk.StartByte + en.End,
			Confidence: en.Score,
			Source:     source,
		}
		if source == pii.SourceML {
			out.ml = append(out.ml, pe)
		} else {
			out.backend = append(out.backend, pe)
		}
	}

	return out, nil
}

func mapEntityType(t entity.Type) (pii.PIIKind, bool) {
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
	default:
		return "", false
	}
}
