package processor

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/contract"
	"github.com/kryneuse/alpha_proxy/internal/pii"
	"github.com/kryneuse/alpha_proxy/internal/policy"
	"github.com/kryneuse/alpha_proxy/internal/store"
)

type fakeMasker struct {
	masked     string
	mappings   []pii.TokenMapping
	err        error
	calls      int
	lastPolicy pii.Policy
}

func (f *fakeMasker) Mask(_ context.Context, _ string, pol pii.Policy) (string, []pii.TokenMapping, error) {
	f.calls++
	f.lastPolicy = pol
	return f.masked, f.mappings, f.err
}

func testPolicy() pii.Policy {
	return pii.Policy{
		AllowedKinds:          map[pii.PIIKind]bool{pii.PIIKindPhone: true},
		DetokenizationAllowed: true,
		MinConfidence:         0.5,
	}
}

func newTestProcessor(t *testing.T, m Masker, ttl time.Duration) (*Processor, store.Store) {
	t.Helper()
	st := store.NewMemoryStore(100)
	pp := policy.NewStaticProvider(map[string]pii.Policy{"consumer-1": testPolicy()})
	p, err := New(st, pp, m, ttl)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return p, st
}

func TestProcessNewPayload(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "masked <PHONE_1>", mappings: []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 6, End: 17},
	}}
	p, st := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	resp, err := p.Process(context.Background(), contract.ProcessRequest{
		Payload:    "call 79123456789",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	})
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if resp.Result != "masked <PHONE_1>" {
		t.Fatalf("unexpected result: %q", resp.Result)
	}
	if m.calls != 1 {
		t.Fatalf("expected 1 masker call, got %d", m.calls)
	}

	sess, err := st.Get(context.Background(), "id-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if sess.Status != pii.SessionStatusReady {
		t.Fatalf("expected READY, got %q", sess.Status)
	}
	if sess.PayloadHash != sha256.Sum256([]byte("call 79123456789")) {
		t.Fatal("unexpected payload hash")
	}
	if len(sess.Mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(sess.Mappings))
	}
	if !sess.CreatedAt.Equal(now) {
		t.Fatalf("unexpected CreatedAt: %v", sess.CreatedAt)
	}
	if !sess.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("unexpected ExpiresAt: %v", sess.ExpiresAt)
	}
}

func TestProcessMaskerError(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{err: errors.New("mask failed")}
	p, st := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	_, err := p.Process(context.Background(), contract.ProcessRequest{
		Payload:    "call 79123456789",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	})
	if err == nil || err.Error() != "mask failed" {
		t.Fatalf("expected mask error, got %v", err)
	}

	sess, err := st.Get(context.Background(), "id-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if sess.Status != pii.SessionStatusFailed {
		t.Fatalf("expected FAILED, got %q", sess.Status)
	}
}

func TestProcessExistingSessionConflict(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "masked"}
	p, st := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	// Заранее создаём PROCESSING session.
	existing := &pii.Session{
		PayloadID:   "id-1",
		PayloadHash: sha256.Sum256([]byte("call 79123456789")),
		Status:      pii.SessionStatusProcessing,
		CreatedAt:   now,
		ExpiresAt:   now.Add(time.Hour),
	}
	if _, err := st.PutIfAbsent(context.Background(), existing); err != nil {
		t.Fatalf("PutIfAbsent returned error: %v", err)
	}

	_, err := p.Process(context.Background(), contract.ProcessRequest{
		Payload:    "call 79123456789",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	})
	if !errors.Is(err, pii.ErrProcessingInProgress) {
		t.Fatalf("expected ErrProcessingInProgress, got %v", err)
	}
	if m.calls != 0 {
		t.Fatalf("expected 0 masker calls, got %d", m.calls)
	}
}

func TestProcessInvalidConstructor(t *testing.T) {
	st := store.NewMemoryStore(100)
	pp := policy.NewStaticProvider(map[string]pii.Policy{"c": testPolicy()})
	m := &fakeMasker{}

	if _, err := New(nil, pp, m, time.Hour); err == nil {
		t.Fatal("expected error for nil store")
	}
	if _, err := New(st, nil, m, time.Hour); err == nil {
		t.Fatal("expected error for nil policy provider")
	}
	if _, err := New(st, pp, nil, time.Hour); err == nil {
		t.Fatal("expected error for nil masker")
	}
	if _, err := New(st, pp, m, 0); err == nil {
		t.Fatal("expected error for zero ttl")
	}
	if _, err := New(st, pp, m, -time.Hour); err == nil {
		t.Fatal("expected error for negative ttl")
	}
}

func TestProcessCancelledContext(t *testing.T) {
	m := &fakeMasker{}
	p, _ := newTestProcessor(t, m, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Process(ctx, contract.ProcessRequest{
		Payload:    "call 79123456789",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if m.calls != 0 {
		t.Fatalf("expected 0 masker calls, got %d", m.calls)
	}
}

func TestProcessReadyRetry(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "call <PHONE_1>", mappings: []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 5, End: 16},
	}}
	p, _ := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	req := contract.ProcessRequest{
		Payload:    "call 79123456789",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	}

	first, err := p.Process(context.Background(), req)
	if err != nil {
		t.Fatalf("first Process returned error: %v", err)
	}
	if m.calls != 1 {
		t.Fatalf("expected 1 masker call after first, got %d", m.calls)
	}

	second, err := p.Process(context.Background(), req)
	if err != nil {
		t.Fatalf("second Process returned error: %v", err)
	}
	if m.calls != 1 {
		t.Fatalf("expected masker not called on retry, got %d", m.calls)
	}
	if first.Result != second.Result {
		t.Fatalf("expected same masked text, got %q and %q", first.Result, second.Result)
	}
}

func TestProcessFailedSession(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{}
	p, st := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	existing := &pii.Session{
		PayloadID:   "id-1",
		PayloadHash: sha256.Sum256([]byte("call 79123456789")),
		Status:      pii.SessionStatusFailed,
		CreatedAt:   now,
		ExpiresAt:   now.Add(time.Hour),
	}
	if _, err := st.PutIfAbsent(context.Background(), existing); err != nil {
		t.Fatalf("PutIfAbsent returned error: %v", err)
	}

	_, err := p.Process(context.Background(), contract.ProcessRequest{
		Payload:    "call 79123456789",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	})
	if !errors.Is(err, pii.ErrDetectorUnavailable) {
		t.Fatalf("expected ErrDetectorUnavailable, got %v", err)
	}
	if m.calls != 0 {
		t.Fatalf("expected 0 masker calls, got %d", m.calls)
	}
}

func TestProcessReadyDifferentPayload(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "masked <PHONE_1>", mappings: []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 5, End: 16},
	}}
	p, _ := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	req := contract.ProcessRequest{
		Payload:    "call 79123456789",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	}
	if _, err := p.Process(context.Background(), req); err != nil {
		t.Fatalf("first Process returned error: %v", err)
	}

	other := contract.ProcessRequest{
		Payload:    "different payload",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	}
	_, err := p.Process(context.Background(), other)
	if !errors.Is(err, pii.ErrPayloadIDConflict) {
		t.Fatalf("expected ErrPayloadIDConflict, got %v", err)
	}
}

func TestProcessReadyCorruptedMapping(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "masked <PHONE_1>", mappings: []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "wrong", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 5, End: 16},
	}}
	p, _ := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	req := contract.ProcessRequest{
		Payload:    "call 79123456789",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	}
	if _, err := p.Process(context.Background(), req); err != nil {
		t.Fatalf("first Process returned error: %v", err)
	}

	_, err := p.Process(context.Background(), req)
	if !errors.Is(err, pii.ErrInvalidSpan) {
		t.Fatalf("expected ErrInvalidSpan, got %v", err)
	}
}

func TestProcessDetokenizeFullMasked(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "call <PHONE_1>", mappings: []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 5, End: 16},
	}}
	p, _ := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	req := contract.ProcessRequest{
		Payload:    "call 79123456789",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	}
	if _, err := p.Process(context.Background(), req); err != nil {
		t.Fatalf("first Process returned error: %v", err)
	}

	detok := contract.ProcessRequest{
		Payload:    "call <PHONE_1>",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	}
	resp, err := p.Process(context.Background(), detok)
	if err != nil {
		t.Fatalf("detokenize Process returned error: %v", err)
	}
	if resp.Result != "call 79123456789" {
		t.Fatalf("expected original text, got %q", resp.Result)
	}
	if m.calls != 1 {
		t.Fatalf("expected masker called once, got %d", m.calls)
	}
}

func TestProcessDetokenizeLLMResponse(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "call <PHONE_1>", mappings: []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 5, End: 16},
	}}
	p, _ := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	req := contract.ProcessRequest{
		Payload:    "call 79123456789",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	}
	if _, err := p.Process(context.Background(), req); err != nil {
		t.Fatalf("first Process returned error: %v", err)
	}

	llm := contract.ProcessRequest{
		Payload:    "Номер клиента: <PHONE_1>. Спасибо.",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	}
	resp, err := p.Process(context.Background(), llm)
	if err != nil {
		t.Fatalf("detokenize Process returned error: %v", err)
	}
	if resp.Result != "Номер клиента: 79123456789. Спасибо." {
		t.Fatalf("unexpected result: %q", resp.Result)
	}
	if m.calls != 1 {
		t.Fatalf("expected masker called once, got %d", m.calls)
	}
}

func TestProcessDetokenizeRepeatedToken(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "call <PHONE_1>", mappings: []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 5, End: 16},
	}}
	p, _ := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	req := contract.ProcessRequest{
		Payload:    "call 79123456789",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	}
	if _, err := p.Process(context.Background(), req); err != nil {
		t.Fatalf("first Process returned error: %v", err)
	}

	detok := contract.ProcessRequest{
		Payload:    "<PHONE_1> и <PHONE_1>",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	}
	resp, err := p.Process(context.Background(), detok)
	if err != nil {
		t.Fatalf("detokenize Process returned error: %v", err)
	}
	if resp.Result != "79123456789 и 79123456789" {
		t.Fatalf("unexpected result: %q", resp.Result)
	}
	if m.calls != 1 {
		t.Fatalf("expected masker called once, got %d", m.calls)
	}
}

func TestProcessDetokenizeDisabled(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "call <PHONE_1>", mappings: []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 5, End: 16},
	}}
	st := store.NewMemoryStore(100)
	pol := testPolicy()
	pol.DetokenizationAllowed = false
	pp := policy.NewStaticProvider(map[string]pii.Policy{"consumer-1": pol})
	p, err := New(st, pp, m, time.Hour)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	p.now = func() time.Time { return now }

	req := contract.ProcessRequest{
		Payload:    "call 79123456789",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	}
	if _, err := p.Process(context.Background(), req); err != nil {
		t.Fatalf("first Process returned error: %v", err)
	}

	detok := contract.ProcessRequest{
		Payload:    "call <PHONE_1>",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	}
	_, err = p.Process(context.Background(), detok)
	if !errors.Is(err, pii.ErrDemaskingDisabled) {
		t.Fatalf("expected ErrDemaskingDisabled, got %v", err)
	}
	if m.calls != 1 {
		t.Fatalf("expected masker called once, got %d", m.calls)
	}
}

func TestProcessDetokenizeUnknownToken(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "call <PHONE_1>", mappings: []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 5, End: 16},
	}}
	p, _ := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	req := contract.ProcessRequest{
		Payload:    "call 79123456789",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	}
	if _, err := p.Process(context.Background(), req); err != nil {
		t.Fatalf("first Process returned error: %v", err)
	}

	detok := contract.ProcessRequest{
		Payload:    "call <EMAIL_1>",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	}
	_, err := p.Process(context.Background(), detok)
	if !errors.Is(err, pii.ErrUnknownToken) {
		t.Fatalf("expected ErrUnknownToken, got %v", err)
	}
	if m.calls != 1 {
		t.Fatalf("expected masker called once, got %d", m.calls)
	}
}

// barrierStore оборачивает настоящий store.Store и задерживает Get до тех пор,
// пока оба конкурентных вызова не получат ErrSessionNotFound.
type barrierStore struct {
	store.Store
	mu       sync.Mutex
	entered  int
	released chan struct{}
	once     sync.Once
}

func (b *barrierStore) Get(ctx context.Context, payloadID string) (*pii.Session, error) {
	sess, err := b.Store.Get(ctx, payloadID)
	if errors.Is(err, pii.ErrSessionNotFound) {
		b.mu.Lock()
		b.entered++
		if b.entered == 2 {
			b.once.Do(func() { close(b.released) })
		}
		b.mu.Unlock()
		<-b.released
	}
	return sess, err
}

func TestProcessConcurrentFirstRequests(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "call <PHONE_1>", mappings: []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 5, End: 16},
	}}
	inner := store.NewMemoryStore(100)
	bs := &barrierStore{Store: inner, released: make(chan struct{})}
	pp := policy.NewStaticProvider(map[string]pii.Policy{"consumer-1": testPolicy()})
	p, err := New(bs, pp, m, time.Hour)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	p.now = func() time.Time { return now }

	req := contract.ProcessRequest{
		Payload:    "call 79123456789",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	}

	type result struct {
		resp contract.ProcessResponse
		err  error
	}
	results := make(chan result, 2)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := p.Process(context.Background(), req)
			results <- result{resp: resp, err: err}
		}()
	}
	wg.Wait()
	close(results)

	var okCount, inProgressCount int
	for r := range results {
		switch {
		case r.err == nil:
			okCount++
			if r.resp.Result != "call <PHONE_1>" {
				t.Fatalf("unexpected result: %q", r.resp.Result)
			}
		case errors.Is(r.err, pii.ErrProcessingInProgress):
			inProgressCount++
		default:
			t.Fatalf("unexpected error: %v", r.err)
		}
	}

	if okCount != 1 {
		t.Fatalf("expected exactly 1 success, got %d", okCount)
	}
	if inProgressCount != 1 {
		t.Fatalf("expected exactly 1 ErrProcessingInProgress, got %d", inProgressCount)
	}
	if m.calls != 1 {
		t.Fatalf("expected masker called once, got %d", m.calls)
	}

	sess, err := inner.Get(context.Background(), "id-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if sess.Status != pii.SessionStatusReady {
		t.Fatalf("expected READY, got %q", sess.Status)
	}
}

func TestProcessMaskKindsOverridesPolicy(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "masked"}
	p, _ := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	_, err := p.Process(context.Background(), contract.ProcessRequest{
		Payload:      "call 79123456789",
		PayloadID:    "id-1",
		ConsumerID:   "consumer-1",
		MaskKinds:    []string{"phone"},
		MaskKindsSet: true,
	})
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if m.calls != 1 {
		t.Fatalf("expected 1 masker call, got %d", m.calls)
	}
	if !m.lastPolicy.AllowedKinds[pii.PIIKindPhone] {
		t.Fatalf("expected phone allowed, got %+v", m.lastPolicy.AllowedKinds)
	}
	if m.lastPolicy.AllowedKinds[pii.PIIKindINN] {
		t.Fatalf("expected inn not allowed, got %+v", m.lastPolicy.AllowedKinds)
	}
}

func TestProcessMaskKindsUnknown(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "masked"}
	p, _ := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	_, err := p.Process(context.Background(), contract.ProcessRequest{
		Payload:      "call 79123456789",
		PayloadID:    "id-1",
		ConsumerID:   "consumer-1",
		MaskKinds:    []string{"unknown"},
		MaskKindsSet: true,
	})
	if err == nil {
		t.Fatal("expected error for unknown mask kind")
	}
	if !errors.Is(err, pii.ErrInvalidMaskKind) {
		t.Fatalf("expected ErrInvalidMaskKind, got %v", err)
	}
	if m.calls != 0 {
		t.Fatalf("expected 0 masker calls, got %d", m.calls)
	}
}

func TestProcessMaskKindsNotSetUsesConsumerPolicy(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "masked"}
	p, _ := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	_, err := p.Process(context.Background(), contract.ProcessRequest{
		Payload:    "call 79123456789",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	})
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if m.calls != 1 {
		t.Fatalf("expected 1 masker call, got %d", m.calls)
	}
	if !m.lastPolicy.AllowedKinds[pii.PIIKindPhone] {
		t.Fatalf("expected phone allowed, got %+v", m.lastPolicy.AllowedKinds)
	}
	if len(m.lastPolicy.AllowedKinds) != 1 {
		t.Fatalf("expected exactly one allowed kind, got %+v", m.lastPolicy.AllowedKinds)
	}
}

func TestProcessMaskKindsEmptyNarrowsToNothing(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "masked"}
	p, _ := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	_, err := p.Process(context.Background(), contract.ProcessRequest{
		Payload:      "call 79123456789",
		PayloadID:    "id-1",
		ConsumerID:   "consumer-1",
		MaskKinds:    []string{},
		MaskKindsSet: true,
	})
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if m.calls != 1 {
		t.Fatalf("expected 1 masker call, got %d", m.calls)
	}
	if len(m.lastPolicy.AllowedKinds) != 0 {
		t.Fatalf("expected empty AllowedKinds, got %+v", m.lastPolicy.AllowedKinds)
	}
}

func TestProcessMaskKindsDuplicates(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "masked"}
	p, _ := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	_, err := p.Process(context.Background(), contract.ProcessRequest{
		Payload:      "call 79123456789",
		PayloadID:    "id-1",
		ConsumerID:   "consumer-1",
		MaskKinds:    []string{"phone", "phone"},
		MaskKindsSet: true,
	})
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if m.calls != 1 {
		t.Fatalf("expected 1 masker call, got %d", m.calls)
	}
	if len(m.lastPolicy.AllowedKinds) != 1 || !m.lastPolicy.AllowedKinds[pii.PIIKindPhone] {
		t.Fatalf("expected only phone allowed, got %+v", m.lastPolicy.AllowedKinds)
	}
}

func TestProcessMaskKindsForbiddenByPolicy(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "masked"}
	p, _ := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	_, err := p.Process(context.Background(), contract.ProcessRequest{
		Payload:      "call 79123456789",
		PayloadID:    "id-1",
		ConsumerID:   "consumer-1",
		MaskKinds:    []string{"email"},
		MaskKindsSet: true,
	})
	if err == nil {
		t.Fatal("expected error for forbidden mask kind")
	}
	if !errors.Is(err, pii.ErrPolicyRejected) {
		t.Fatalf("expected ErrPolicyRejected, got %v", err)
	}
	if m.calls != 0 {
		t.Fatalf("expected 0 masker calls, got %d", m.calls)
	}
}

func TestProcessMaskKindsRetryCanonical(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "masked <PHONE_1>", mappings: []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 5, End: 16},
	}}
	p, _ := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	first := contract.ProcessRequest{
		Payload:      "call 79123456789",
		PayloadID:    "id-1",
		ConsumerID:   "consumer-1",
		MaskKinds:    []string{"phone", "phone"},
		MaskKindsSet: true,
	}
	if _, err := p.Process(context.Background(), first); err != nil {
		t.Fatalf("first Process returned error: %v", err)
	}
	if m.calls != 1 {
		t.Fatalf("expected 1 masker call after first, got %d", m.calls)
	}

	// Same selection in a different order and with duplicates is a retry.
	retry := contract.ProcessRequest{
		Payload:      "call 79123456789",
		PayloadID:    "id-1",
		ConsumerID:   "consumer-1",
		MaskKinds:    []string{"phone"},
		MaskKindsSet: true,
	}
	resp, err := p.Process(context.Background(), retry)
	if err != nil {
		t.Fatalf("retry Process returned error: %v", err)
	}
	if m.calls != 1 {
		t.Fatalf("expected masker not called on retry, got %d", m.calls)
	}
	if resp.Result != "call <PHONE_1>" {
		t.Fatalf("unexpected result: %q", resp.Result)
	}
}

func TestProcessMaskKindsRetryConflict(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "masked <PHONE_1>", mappings: []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 6, End: 17},
	}}
	st := store.NewMemoryStore(100)
	pol := testPolicy()
	pol.AllowedKinds = map[pii.PIIKind]bool{pii.PIIKindPhone: true, pii.PIIKindEmail: true}
	pp := policy.NewStaticProvider(map[string]pii.Policy{"consumer-1": pol})
	p, err := New(st, pp, m, time.Hour)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	p.now = func() time.Time { return now }

	first := contract.ProcessRequest{
		Payload:      "call 79123456789",
		PayloadID:    "id-1",
		ConsumerID:   "consumer-1",
		MaskKinds:    []string{"phone"},
		MaskKindsSet: true,
	}
	if _, err := p.Process(context.Background(), first); err != nil {
		t.Fatalf("first Process returned error: %v", err)
	}

	// Different selection at the same payload and payload_id is a conflict.
	other := contract.ProcessRequest{
		Payload:      "call 79123456789",
		PayloadID:    "id-1",
		ConsumerID:   "consumer-1",
		MaskKinds:    []string{"email"},
		MaskKindsSet: true,
	}
	_, err = p.Process(context.Background(), other)
	if !errors.Is(err, pii.ErrPayloadIDConflict) {
		t.Fatalf("expected ErrPayloadIDConflict, got %v", err)
	}
	if m.calls != 1 {
		t.Fatalf("expected masker called once, got %d", m.calls)
	}
}

func TestProcessMaskKindsAbsentVsEmptyConflict(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "masked <PHONE_1>", mappings: []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 6, End: 17},
	}}
	p, _ := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	// Explicit empty array.
	empty := contract.ProcessRequest{
		Payload:      "call 79123456789",
		PayloadID:    "id-1",
		ConsumerID:   "consumer-1",
		MaskKinds:    []string{},
		MaskKindsSet: true,
	}
	if _, err := p.Process(context.Background(), empty); err != nil {
		t.Fatalf("first Process returned error: %v", err)
	}

	// Absent field is a different setting.
	absent := contract.ProcessRequest{
		Payload:    "call 79123456789",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	}
	_, err := p.Process(context.Background(), absent)
	if !errors.Is(err, pii.ErrPayloadIDConflict) {
		t.Fatalf("expected ErrPayloadIDConflict, got %v", err)
	}
	if m.calls != 1 {
		t.Fatalf("expected masker called once, got %d", m.calls)
	}
}

func TestProcessMaskKindsDetokenizeWithoutKinds(t *testing.T) {
	now := time.Now()
	m := &fakeMasker{masked: "call <PHONE_1>", mappings: []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 5, End: 16},
	}}
	p, _ := newTestProcessor(t, m, time.Hour)
	p.now = func() time.Time { return now }

	mask := contract.ProcessRequest{
		Payload:      "call 79123456789",
		PayloadID:    "id-1",
		ConsumerID:   "consumer-1",
		MaskKinds:    []string{"phone"},
		MaskKindsSet: true,
	}
	if _, err := p.Process(context.Background(), mask); err != nil {
		t.Fatalf("mask Process returned error: %v", err)
	}
	if m.calls != 1 {
		t.Fatalf("expected 1 masker call, got %d", m.calls)
	}

	// Detokenization must work without re-passing mask_kinds.
	detok := contract.ProcessRequest{
		Payload:    "call <PHONE_1>",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	}
	resp, err := p.Process(context.Background(), detok)
	if err != nil {
		t.Fatalf("detokenize Process returned error: %v", err)
	}
	if resp.Result != "call 79123456789" {
		t.Fatalf("unexpected detokenized result: %q", resp.Result)
	}
	if m.calls != 1 {
		t.Fatalf("expected masker not called on detokenize, got %d", m.calls)
	}
}
