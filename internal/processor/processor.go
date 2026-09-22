package processor

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/contract"
	"github.com/kryneuse/alpha_proxy/internal/pii"
	"github.com/kryneuse/alpha_proxy/internal/store"
	"github.com/kryneuse/alpha_proxy/internal/tokenizer"
)

// Masker — временная граница для будущего координатора Go detector и ML.
type Masker interface {
	Mask(ctx context.Context, text string, policy pii.Policy) (string, []pii.TokenMapping, error)
}

// Processor реализует contract.Processor.
type Processor struct {
	store  store.Store
	policy pii.PolicyProvider
	masker Masker
	ttl    time.Duration
	now    func() time.Time
}

// New создаёт Processor и проверяет зависимости.
func New(st store.Store, pp pii.PolicyProvider, m Masker, ttl time.Duration) (*Processor, error) {
	if st == nil {
		return nil, errors.New("store must not be nil")
	}
	if pp == nil {
		return nil, errors.New("policy provider must not be nil")
	}
	if m == nil {
		return nil, errors.New("masker must not be nil")
	}
	if ttl <= 0 {
		return nil, errors.New("ttl must be > 0")
	}
	return &Processor{store: st, policy: pp, masker: m, ttl: ttl, now: time.Now}, nil
}

var _ contract.Processor = (*Processor)(nil)

// Process обрабатывает запрос: новый PayloadID, retry исходного запроса или
// детокенизацию для существующей session.
func (p *Processor) Process(ctx context.Context, req contract.ProcessRequest) (contract.ProcessResponse, error) {
	if err := ctx.Err(); err != nil {
		return contract.ProcessResponse{}, err
	}

	pol, err := p.policy.Get(ctx, req.ConsumerID)
	if err != nil {
		return contract.ProcessResponse{}, err
	}

	session, err := p.store.Get(ctx, req.PayloadID)
	if err != nil && !errors.Is(err, pii.ErrSessionNotFound) {
		return contract.ProcessResponse{}, err
	}
	if session != nil {
		return p.handleExisting(ctx, req, session, pol)
	}

	now := p.now()
	hash := sha256.Sum256([]byte(req.Payload))
	newSession := &pii.Session{
		PayloadID:   req.PayloadID,
		PayloadHash: hash,
		Status:      pii.SessionStatusProcessing,
		CreatedAt:   now,
		ExpiresAt:   now.Add(p.ttl),
	}

	added, err := p.store.PutIfAbsent(ctx, newSession)
	if err != nil {
		return contract.ProcessResponse{}, err
	}
	if !added {
		return contract.ProcessResponse{}, pii.ErrProcessingInProgress
	}

	masked, mappings, err := p.masker.Mask(ctx, req.Payload, pol)
	if err != nil {
		failed := *newSession
		failed.Status = pii.SessionStatusFailed
		_ = p.store.Update(ctx, &failed)
		return contract.ProcessResponse{}, err
	}

	ready := *newSession
	ready.Mappings = mappings
	ready.Status = pii.SessionStatusReady
	if err := p.store.Update(ctx, &ready); err != nil {
		return contract.ProcessResponse{}, err
	}

	return contract.ProcessResponse{Result: masked}, nil
}

// handleExisting обрабатывает запрос для уже существующей session.
func (p *Processor) handleExisting(ctx context.Context, req contract.ProcessRequest, session *pii.Session, pol pii.Policy) (contract.ProcessResponse, error) {
	switch session.Status {
	case pii.SessionStatusProcessing:
		return contract.ProcessResponse{}, pii.ErrProcessingInProgress
	case pii.SessionStatusFailed:
		return contract.ProcessResponse{}, pii.ErrDetectorUnavailable
	case pii.SessionStatusReady:
		// retry исходного запроса или детокенизация
	default:
		return contract.ProcessResponse{}, pii.ErrStoreUnavailable
	}

	hash := sha256.Sum256([]byte(req.Payload))
	if hash == session.PayloadHash {
		// retry исходного запроса
		replacements := make([]pii.Replacement, 0, len(session.Mappings))
		for _, m := range session.Mappings {
			replacements = append(replacements, pii.Replacement{
				Start:    m.Start,
				End:      m.End,
				Token:    m.Token,
				Original: m.Original,
				Kind:     m.Kind,
				Source:   m.Source,
			})
		}

		masked, _, err := tokenizer.ApplyReplacements(ctx, req.Payload, replacements)
		if err != nil {
			return contract.ProcessResponse{}, err
		}
		return contract.ProcessResponse{Result: masked}, nil
	}

	// hash не совпадает — возможно, это запрос на детокенизацию.
	knownToken := false
	for _, m := range session.Mappings {
		if m.Token != "" && strings.Contains(req.Payload, m.Token) {
			knownToken = true
			break
		}
	}

	if knownToken && !pol.DetokenizationAllowed {
		return contract.ProcessResponse{}, pii.ErrDemaskingDisabled
	}

	detokenized, err := tokenizer.Detokenize(ctx, req.Payload, session.Mappings)
	if err != nil {
		return contract.ProcessResponse{}, err
	}

	if detokenized == req.Payload {
		// Токенов не было — это не retry и не детокенизация.
		return contract.ProcessResponse{}, pii.ErrPayloadIDConflict
	}

	return contract.ProcessResponse{Result: detokenized}, nil
}
