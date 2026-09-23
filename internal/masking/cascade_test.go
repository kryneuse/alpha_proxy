package masking

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/entity"
	"github.com/kryneuse/alpha_proxy/internal/ml"
	"github.com/kryneuse/alpha_proxy/internal/pii"
)

// fullChunkRunner возвращает entity для всего чанка. reason "ml" отдаёт
// сущность через RunResidual, reason "regex" — через AnalyzeRules.
type fullChunkRunner struct {
	reason    string
	typ       entity.Type
	err       error
	block     chan struct{}
	started   chan struct{}
	mu        sync.Mutex
	active    int
	maxActive int
}

func (f *fullChunkRunner) AnalyzeRules(text string) ([]entity.Entity, string) {
	if f.reason == "regex" && text != "" {
		typ := f.typ
		if typ == "" {
			typ = entity.PHONE
		}
		return []entity.Entity{
			{Type: typ, Text: text, Start: 0, End: len(text), Score: 0.9, Reason: f.reason},
		}, text
	}
	return nil, text
}

func (f *fullChunkRunner) DetectChunk(ctx context.Context, original, gate string) ([]entity.Entity, error) {
	f.mu.Lock()
	f.active++
	if f.active > f.maxActive {
		f.maxActive = f.active
	}
	f.mu.Unlock()
	if f.started != nil {
		select {
		case f.started <- struct{}{}:
		default:
		}
	}
	defer func() {
		f.mu.Lock()
		f.active--
		f.mu.Unlock()
	}()
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	if original == "" || f.reason != "ml" {
		return nil, nil
	}
	typ := f.typ
	if typ == "" {
		typ = entity.PHONE
	}
	return []entity.Entity{
		{Type: typ, Text: original, Start: 0, End: len(original), Score: 0.9, Reason: f.reason},
	}, nil
}

func newTestCascadeMasker(t *testing.T, r cascadeRunner, maxParallel int) *CascadeMasker {
	t.Helper()
	cfg := ml.DefaultChunkConfig()
	m, err := NewCascadeMasker(r, cfg, maxParallel)
	if err != nil {
		t.Fatalf("NewCascadeMasker returned error: %v", err)
	}
	return m
}

func TestCascadeMaskerShortText(t *testing.T) {
	r := &fullChunkRunner{reason: "ml"}
	m := newTestCascadeMasker(t, r, 2)

	masked, mappings, err := m.Mask(context.Background(), "call 79123456789", testPolicy())
	if err != nil {
		t.Fatalf("Mask returned error: %v", err)
	}
	if masked != "<PHONE_1>" {
		t.Fatalf("unexpected masked text: %q", masked)
	}
	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(mappings))
	}
	if mappings[0].Source != pii.SourceML {
		t.Fatalf("expected SourceML, got %q", mappings[0].Source)
	}
}

func TestCascadeMaskerMultipleChunks(t *testing.T) {
	r := &fullChunkRunner{reason: "ml"}
	m := newTestCascadeMasker(t, r, 2)

	text := strings.Repeat("a", 1000)
	masked, mappings, err := m.Mask(context.Background(), text, testPolicy())
	if err != nil {
		t.Fatalf("Mask returned error: %v", err)
	}
	if masked == text {
		t.Fatal("expected masked text to differ")
	}
	if len(mappings) == 0 {
		t.Fatal("expected mappings")
	}
}

func TestCascadeMaskerGlobalOffsets(t *testing.T) {
	r := &fullChunkRunner{reason: "ml"}
	cfg := ml.DefaultChunkConfig()
	cfg.TargetCodePoints = 5
	cfg.MaxCodePoints = 10
	cfg.OverlapCodePoints = 2
	m, err := NewCascadeMasker(r, cfg, 2)
	if err != nil {
		t.Fatalf("NewCascadeMasker returned error: %v", err)
	}

	text := "abcdefghij"
	masked, _, err := m.Mask(context.Background(), text, testPolicy())
	if err != nil {
		t.Fatalf("Mask returned error: %v", err)
	}
	if masked == text {
		t.Fatal("expected masked text to differ")
	}
}

func TestCascadeMaskerCyrillic(t *testing.T) {
	r := &fullChunkRunner{reason: "ml"}
	m := newTestCascadeMasker(t, r, 2)

	text := "звони 79123456789"
	masked, mappings, err := m.Mask(context.Background(), text, testPolicy())
	if err != nil {
		t.Fatalf("Mask returned error: %v", err)
	}
	if masked == text {
		t.Fatal("expected masked text to differ")
	}
	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(mappings))
	}
}

func TestCascadeMaskerOverlapDedup(t *testing.T) {
	r := &fullChunkRunner{reason: "ml"}
	cfg := ml.DefaultChunkConfig()
	cfg.TargetCodePoints = 5
	cfg.MaxCodePoints = 10
	cfg.OverlapCodePoints = 2
	m, err := NewCascadeMasker(r, cfg, 2)
	if err != nil {
		t.Fatalf("NewCascadeMasker returned error: %v", err)
	}

	text := "hello world"
	masked, _, err := m.Mask(context.Background(), text, testPolicy())
	if err != nil {
		t.Fatalf("Mask returned error: %v", err)
	}
	if masked == text {
		t.Fatal("expected masked text to differ")
	}
}

func TestCascadeMaskerRuleAndML(t *testing.T) {
	r := &fullChunkRunner{reason: "regex"}
	m := newTestCascadeMasker(t, r, 2)

	masked, mappings, err := m.Mask(context.Background(), "call 79123456789", testPolicy())
	if err != nil {
		t.Fatalf("Mask returned error: %v", err)
	}
	if masked != "<PHONE_1>" {
		t.Fatalf("unexpected masked text: %q", masked)
	}
	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(mappings))
	}
	if mappings[0].Source != pii.SourceReg {
		t.Fatalf("expected SourceReg, got %q", mappings[0].Source)
	}
}

func TestCascadeMaskerPolicyFilter(t *testing.T) {
	r := &fullChunkRunner{reason: "ml"}
	m := newTestCascadeMasker(t, r, 2)

	pol := testPolicy()
	pol.AllowedKinds = map[pii.PIIKind]bool{pii.PIIKindINN: true}

	masked, mappings, err := m.Mask(context.Background(), "call 79123456789", pol)
	if err != nil {
		t.Fatalf("Mask returned error: %v", err)
	}
	if masked != "call 79123456789" {
		t.Fatalf("expected unchanged text, got %q", masked)
	}
	if len(mappings) != 0 {
		t.Fatalf("expected 0 mappings, got %d", len(mappings))
	}
}

func TestCascadeMaskerUnknownType(t *testing.T) {
	r := &fullChunkRunner{reason: "ml", typ: entity.Type("UNKNOWN")}
	m := newTestCascadeMasker(t, r, 2)

	_, _, err := m.Mask(context.Background(), "call 79123456789", testPolicy())
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestCascadeMaskerInvalidSpan(t *testing.T) {
	r := &invalidSpanRunner{}
	m := newTestCascadeMasker(t, r, 2)

	_, _, err := m.Mask(context.Background(), "call 79123456789", testPolicy())
	if !errors.Is(err, pii.ErrInvalidSpan) {
		t.Fatalf("expected ErrInvalidSpan, got %v", err)
	}
}

type invalidSpanRunner struct{}

func (invalidSpanRunner) AnalyzeRules(text string) ([]entity.Entity, string) {
	return nil, text
}

func (invalidSpanRunner) DetectChunk(_ context.Context, original, gate string) ([]entity.Entity, error) {
	return []entity.Entity{
		{Type: entity.PHONE, Text: "x", Start: -1, End: 1, Score: 0.9, Reason: "ml"},
	}, nil
}

func TestCascadeMaskerRunnerError(t *testing.T) {
	r := &fullChunkRunner{reason: "ml", err: errors.New("runner failed")}
	m := newTestCascadeMasker(t, r, 2)

	_, _, err := m.Mask(context.Background(), "call 79123456789", testPolicy())
	if err == nil || err.Error() != "runner failed" {
		t.Fatalf("expected runner error, got %v", err)
	}
}

func TestCascadeMaskerCancelledContext(t *testing.T) {
	r := &fullChunkRunner{reason: "ml"}
	m := newTestCascadeMasker(t, r, 2)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := m.Mask(ctx, "call 79123456789", testPolicy())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestCascadeMaskerMaxParallel(t *testing.T) {
	block := make(chan struct{})
	started := make(chan struct{}, 10)
	r := &fullChunkRunner{reason: "ml", block: block, started: started}
	cfg := ml.DefaultChunkConfig()
	cfg.TargetCodePoints = 5
	cfg.MaxCodePoints = 10
	cfg.OverlapCodePoints = 2
	m, err := NewCascadeMasker(r, cfg, 2)
	if err != nil {
		t.Fatalf("NewCascadeMasker returned error: %v", err)
	}

	text := strings.Repeat("a", 100)
	done := make(chan struct{})
	go func() {
		_, _, _ = m.Mask(context.Background(), text, testPolicy())
		close(done)
	}()

	// Ждём, пока 2 воркера станут активными.
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("expected 2 workers to start")
		}
	}

	r.mu.Lock()
	maxActive := r.maxActive
	r.mu.Unlock()
	if maxActive > 2 {
		t.Fatalf("expected max parallel <= 2, got %d", maxActive)
	}

	close(block)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Mask did not finish")
	}
}

func TestNewCascadeMaskerInvalid(t *testing.T) {
	r := &fullChunkRunner{}
	cfg := ml.DefaultChunkConfig()
	if _, err := NewCascadeMasker(nil, cfg, 2); err == nil {
		t.Fatal("expected error for nil runner")
	}
	if _, err := NewCascadeMasker(r, cfg, 0); err == nil {
		t.Fatal("expected error for zero max parallel")
	}
	bad := cfg
	bad.MaxCodePoints = 0
	if _, err := NewCascadeMasker(r, bad, 2); err == nil {
		t.Fatal("expected error for invalid chunk config")
	}
}

var errSentinel = errors.New("sentinel")

// mixedRunner возвращает sentinel для чанка с "ERR", блокируется для чанка с
// "BLOCK", остальные обрабатывает без сущностей.
type mixedRunner struct {
	mu        sync.Mutex
	processed []string
}

func (m *mixedRunner) AnalyzeRules(text string) ([]entity.Entity, string) {
	return nil, text
}

func (m *mixedRunner) DetectChunk(ctx context.Context, original, gate string) ([]entity.Entity, error) {
	m.mu.Lock()
	m.processed = append(m.processed, original)
	m.mu.Unlock()
	if strings.Contains(original, "ERR") {
		return nil, errSentinel
	}
	if strings.Contains(original, "BLOCK") {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return nil, nil
}

func TestCascadeMaskerFirstErrorWins(t *testing.T) {
	r := &mixedRunner{}
	cfg := ml.DefaultChunkConfig()
	cfg.TargetCodePoints = 3
	cfg.MaxCodePoints = 5
	cfg.OverlapCodePoints = 1
	m, err := NewCascadeMasker(r, cfg, 2)
	if err != nil {
		t.Fatalf("NewCascadeMasker returned error: %v", err)
	}

	_, _, err = m.Mask(context.Background(), "ERR. BLOCK.", testPolicy())
	if !errors.Is(err, errSentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.processed {
		if p == "." {
			t.Fatalf("chunk after error should not be processed: %q", p)
		}
	}
}