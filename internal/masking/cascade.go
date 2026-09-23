package masking

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"unicode/utf8"

	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/ml"
	"github.com/kryneuse/alpha_proxy/internal/pii"
	"github.com/kryneuse/alpha_proxy/internal/tokenizer"
)

// cascadeRunner runs the rule engine on the whole text and the ML extractor on
// (original_text, gate_text) chunk pairs. Chunking happens on the original text
// after the rules; the gate_text is the byte-preserving masked residual of the
// same chunk.
type cascadeRunner interface {
	// AnalyzeRules runs the rule engine on the whole text and returns the rule
	// entities and the residual text.
	AnalyzeRules(text string) ([]entity.Entity, string)
	// DetectChunk runs the ML extractor on a (original_text, gate_text) pair
	// and returns ML entities with offsets relative to original.
	DetectChunk(ctx context.Context, original, gate string) ([]entity.Entity, error)
}

// CascadeMasker маскирует текст через cascade: правила на всём тексте, затем
// чанкинг оригинала и обработка пар (original, gate) параллельно.
type CascadeMasker struct {
	casc        cascadeRunner
	chunkCfg    ml.ChunkConfig
	maxParallel int
}

// NewCascadeMasker создаёт CascadeMasker.
func NewCascadeMasker(r cascadeRunner, chunkCfg ml.ChunkConfig, maxParallel int) (*CascadeMasker, error) {
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
	return &CascadeMasker{casc: r, chunkCfg: chunkCfg, maxParallel: maxParallel}, nil
}

type chunkJob struct {
	index    int
	chunk    ml.TextChunk
	original string
	gate     string
}

// Mask обрабатывает текст: правила на всём тексте, затем чанкинг оригинала и
// обработка пар (original, gate) параллельно, затем сборка итогового masked
// текста.
func (m *CascadeMasker) Mask(ctx context.Context, text string, policy pii.Policy) (string, []pii.TokenMapping, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}

	// 1. Правила на всём тексте.
	ruleEntities, residualText := m.casc.AnalyzeRules(text)

	// 2. Конвертируем rule-сущности в pii.Entity (Source: reg).
	backendEntities, err := ruleEntitiesToPII(text, ruleEntities)
	if err != nil {
		return "", nil, err
	}

	// 3. Чанкинг оригинала (не residual). residual имеет ту же байтовую длину,
	//    что и original, поэтому gate[a:b] соответствует original[a:b].
	chunks, err := ml.SplitText(ctx, text, m.chunkCfg)
	if err != nil {
		return "", nil, err
	}

	// 4. Обработка пар (original, gate) параллельно.
	var mlEntities []pii.Entity
	if len(chunks) > 0 {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		jobs := make(chan chunkJob)
		results := make([][]pii.Entity, len(chunks))

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
					entities, err := m.processChunk(ctx, job)
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
			job := chunkJob{
				index:    i,
				chunk:    chunk,
				original: text[chunk.StartByte:chunk.EndByte],
				gate:     residualText[chunk.StartByte:chunk.EndByte],
			}
			select {
			case jobs <- job:
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

		for _, r := range results {
			mlEntities = append(mlEntities, r...)
		}
	}

	// 5. Сборка планов замен и маскирование.
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

// processChunk обрабатывает один чанк оригинала через ML и возвращает
// сущности с глобальными byte offsets.
func (m *CascadeMasker) processChunk(ctx context.Context, job chunkJob) ([]pii.Entity, error) {
	entities, err := m.casc.DetectChunk(ctx, job.original, job.gate)
	if err != nil {
		return nil, err
	}

	var out []pii.Entity
	for _, en := range entities {
		kind, ok := mapEntityType(en.Type)
		if !ok {
			return nil, errors.New("unknown entity type")
		}
		if en.Start < 0 || en.End > len(job.original) || en.Start >= en.End {
			return nil, fmt.Errorf("invalid span [%d:%d]: %w", en.Start, en.End, pii.ErrInvalidSpan)
		}
		if !utf8.ValidString(job.original[en.Start:en.End]) {
			return nil, fmt.Errorf("span cuts utf-8 at [%d:%d]: %w", en.Start, en.End, pii.ErrInvalidSpan)
		}
		if job.original[en.Start:en.End] != en.Text {
			return nil, fmt.Errorf("entity text mismatch at [%d:%d]: %w", en.Start, en.End, pii.ErrInvalidSpan)
		}

		out = append(out, pii.Entity{
			Kind:       kind,
			Start:      job.chunk.StartByte + en.Start,
			End:        job.chunk.StartByte + en.End,
			Confidence: en.Score,
			Source:     pii.SourceML,
		})
	}

	return out, nil
}

// ruleEntitiesToPII конвертирует rule-сущности (глобальные byte offsets) в
// pii.Entity с Source: reg.
func ruleEntitiesToPII(text string, entities []entity.Entity) ([]pii.Entity, error) {
	var out []pii.Entity
	for _, en := range entities {
		kind, ok := mapEntityType(en.Type)
		if !ok {
			return nil, errors.New("unknown entity type")
		}
		if en.Start < 0 || en.End > len(text) || en.Start >= en.End {
			return nil, fmt.Errorf("invalid span [%d:%d]: %w", en.Start, en.End, pii.ErrInvalidSpan)
		}
		if !utf8.ValidString(text[en.Start:en.End]) {
			return nil, fmt.Errorf("span cuts utf-8 at [%d:%d]: %w", en.Start, en.End, pii.ErrInvalidSpan)
		}
		if text[en.Start:en.End] != en.Text {
			return nil, fmt.Errorf("entity text mismatch at [%d:%d]: %w", en.Start, en.End, pii.ErrInvalidSpan)
		}
		out = append(out, pii.Entity{
			Kind:       kind,
			Start:      en.Start,
			End:        en.End,
			Confidence: en.Score,
			Source:     pii.SourceReg,
		})
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
	case entity.CITY:
		return pii.PIIKindCity, true
	case entity.STREET:
		return pii.PIIKindStreet, true
	case entity.HOUSE:
		return pii.PIIKindHouse, true
	case entity.APARTMENT:
		return pii.PIIKindApartment, true
	case entity.POSTAL_CODE:
		return pii.PIIKindPostalCode, true
	case entity.COUNTRY:
		return pii.PIIKindCountry, true
	case entity.REGION:
		return pii.PIIKindRegion, true
	case entity.DISTRICT:
		return pii.PIIKindDistrict, true
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