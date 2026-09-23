package ml

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

type fakeClient struct {
	mu   sync.Mutex
	reqs []BatchRequest
	resp BatchResponse
	err  error
}

func (f *fakeClient) ProcessBatch(_ context.Context, req BatchRequest) (BatchResponse, error) {
	f.mu.Lock()
	f.reqs = append(f.reqs, req)
	f.mu.Unlock()
	return f.resp, f.err
}

func (f *fakeClient) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reqs)
}

func (f *fakeClient) lastRequest() BatchRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.reqs) == 0 {
		return BatchRequest{}
	}
	return f.reqs[len(f.reqs)-1]
}

// recordingClient записывает запросы и строит ответ через respFn.
type recordingClient struct {
	mu     sync.Mutex
	reqs   []BatchRequest
	respFn func(req BatchRequest) BatchResponse
}

func (r *recordingClient) ProcessBatch(_ context.Context, req BatchRequest) (BatchResponse, error) {
	r.mu.Lock()
	r.reqs = append(r.reqs, req)
	r.mu.Unlock()
	return r.respFn(req), nil
}

func (r *recordingClient) requestCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reqs)
}

func (r *recordingClient) lastRequest() BatchRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.reqs) == 0 {
		return BatchRequest{}
	}
	return r.reqs[len(r.reqs)-1]
}

func TestBatcherBatchesJobs(t *testing.T) {
	fc := &recordingClient{respFn: func(req BatchRequest) BatchResponse {
		results := make([]ChunkResult, 0, len(req.Items))
		for _, item := range req.Items {
			results = append(results, ChunkResult{ChunkID: item.ChunkID})
		}
		return BatchResponse{
			BatchID:      req.BatchID,
			ModelVersion: "v1",
			OffsetUnit:   OffsetsUnicodeCodePoints,
			Results:      results,
		}
	}}
	cfg := DefaultBatchConfig()
	cfg.MaxItems = 2
	cfg.MaxWait = time.Hour
	cfg.Workers = 1
	b, err := NewBatcher(fc, cfg)
	if err != nil {
		t.Fatalf("NewBatcher returned error: %v", err)
	}
	t.Cleanup(b.Close)

	ctx := context.Background()
	var wg sync.WaitGroup
	results := make([]ChunkResult, 2)
	errs := make([]error, 2)
	for i, id := range []string{"c1", "c2"} {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			results[i], errs[i] = b.Process(ctx, RequestItem{ChunkID: id, Text: "text"})
		}(i, id)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Process %d returned error: %v", i, err)
		}
	}
	if fc.requestCount() != 1 {
		t.Fatalf("expected 1 batch request, got %d", fc.requestCount())
	}
	req := fc.lastRequest()
	if len(req.Items) != 2 {
		t.Fatalf("expected 2 items in batch, got %d", len(req.Items))
	}
	if req.OffsetUnit != OffsetsUnicodeCodePoints {
		t.Fatalf("unexpected offset unit: %q", req.OffsetUnit)
	}
	// Транспортные chunk_id должны быть уникальными.
	if req.Items[0].ChunkID == req.Items[1].ChunkID {
		t.Fatalf("transport chunk ids must be unique: %q", req.Items[0].ChunkID)
	}
}

func TestBatcherReorderedResponse(t *testing.T) {
	fc := &recordingClient{respFn: func(req BatchRequest) BatchResponse {
		results := make([]ChunkResult, 0, len(req.Items))
		for _, item := range req.Items {
			typ := "email"
			if item.Text == "text2" {
				typ = "phone"
			}
			results = append(results, ChunkResult{
				ChunkID:  item.ChunkID,
				Entities: []MLEntity{{Type: typ, Start: 0, End: 1, Confidence: 0.9}},
			})
		}
		// Возвращаем результаты в обратном порядке.
		for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
			results[i], results[j] = results[j], results[i]
		}
		return BatchResponse{
			BatchID:      req.BatchID,
			ModelVersion: "v1",
			OffsetUnit:   OffsetsUnicodeCodePoints,
			Results:      results,
		}
	}}
	cfg := DefaultBatchConfig()
	cfg.MaxItems = 2
	cfg.MaxWait = time.Hour
	cfg.Workers = 1
	b, err := NewBatcher(fc, cfg)
	if err != nil {
		t.Fatalf("NewBatcher returned error: %v", err)
	}
	t.Cleanup(b.Close)

	ctx := context.Background()
	var wg sync.WaitGroup
	results := make([]ChunkResult, 2)
	errs := make([]error, 2)
	for i, text := range []string{"text1", "text2"} {
		wg.Add(1)
		go func(i int, text string) {
			defer wg.Done()
			results[i], errs[i] = b.Process(ctx, RequestItem{ChunkID: "c1", Text: text})
		}(i, text)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Process %d returned error: %v", i, err)
		}
	}
	if len(results[0].Entities) != 1 || results[0].Entities[0].Type != "email" {
		t.Fatalf("text1 got wrong result: %+v", results[0])
	}
	if len(results[1].Entities) != 1 || results[1].Entities[0].Type != "phone" {
		t.Fatalf("text2 got wrong result: %+v", results[1])
	}
	// Исходный chunk_id должен быть восстановлен.
	if results[0].ChunkID != "c1" || results[1].ChunkID != "c1" {
		t.Fatalf("expected original chunk id c1, got %q and %q", results[0].ChunkID, results[1].ChunkID)
	}
}

func TestBatcherRPCError(t *testing.T) {
	rpcErr := errors.New("rpc failed")
	fc := &fakeClient{err: rpcErr}
	cfg := DefaultBatchConfig()
	cfg.MaxItems = 2
	cfg.MaxWait = time.Hour
	cfg.Workers = 1
	b, err := NewBatcher(fc, cfg)
	if err != nil {
		t.Fatalf("NewBatcher returned error: %v", err)
	}
	t.Cleanup(b.Close)

	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, id := range []string{"c1", "c2"} {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			_, errs[i] = b.Process(ctx, RequestItem{ChunkID: id, Text: "text"})
		}(i, id)
	}
	wg.Wait()

	for i, err := range errs {
		if err == nil || err.Error() != "rpc failed" {
			t.Fatalf("Process %d expected rpc error, got %v", i, err)
		}
	}
}

func TestBatcherPartialChunkError(t *testing.T) {
	fc := &recordingClient{respFn: func(req BatchRequest) BatchResponse {
		results := make([]ChunkResult, 0, len(req.Items))
		for _, item := range req.Items {
			cr := ChunkResult{ChunkID: item.ChunkID}
			if item.Text == "text2" {
				cr.ErrorCode = "TOO_LARGE"
			}
			results = append(results, cr)
		}
		return BatchResponse{
			BatchID:      req.BatchID,
			ModelVersion: "v1",
			OffsetUnit:   OffsetsUnicodeCodePoints,
			Results:      results,
		}
	}}
	cfg := DefaultBatchConfig()
	cfg.MaxItems = 2
	cfg.MaxWait = time.Hour
	cfg.Workers = 1
	b, err := NewBatcher(fc, cfg)
	if err != nil {
		t.Fatalf("NewBatcher returned error: %v", err)
	}
	t.Cleanup(b.Close)

	ctx := context.Background()
	var wg sync.WaitGroup
	results := make([]ChunkResult, 2)
	errs := make([]error, 2)
	for i, text := range []string{"text1", "text2"} {
		wg.Add(1)
		go func(i int, text string) {
			defer wg.Done()
			results[i], errs[i] = b.Process(ctx, RequestItem{ChunkID: "c1", Text: text})
		}(i, text)
	}
	wg.Wait()

	if errs[0] != nil {
		t.Fatalf("text1 should succeed, got error: %v", errs[0])
	}
	if !errors.Is(errs[1], pii.ErrDetectorUnavailable) {
		t.Fatalf("text2 expected ErrDetectorUnavailable, got %v", errs[1])
	}
}

func TestBatcherQueueOverflow(t *testing.T) {
	// Блокирующий client: воркер забирает первый job и ждёт.
	release := make(chan struct{})
	blocking := &blockingClient{release: release, started: make(chan struct{})}
	cfg := DefaultBatchConfig()
	cfg.QueueCapacity = 1
	cfg.MaxItems = 1
	cfg.MaxWait = time.Hour
	cfg.Workers = 1
	b, err := NewBatcher(blocking, cfg)
	if err != nil {
		t.Fatalf("NewBatcher returned error: %v", err)
	}
	t.Cleanup(func() {
		close(release)
		b.Close()
	})

	ctx := context.Background()
	// Первый job занимает воркера.
	done := make(chan struct{})
	go func() {
		_, _ = b.Process(ctx, RequestItem{ChunkID: "c1", Text: "text"})
		close(done)
	}()

	// Ждём, пока воркер заберёт первый job.
	<-blocking.started

	// Второй job кладём в очередь напрямую (воркер занят), третий переполнит её.
	job := batchJob{
		ctx:    ctx,
		item:   RequestItem{ChunkID: "c2", Text: "text"},
		result: make(chan jobResult, 1),
	}
	if err := b.queue.enqueue(ctx, job); err != nil {
		t.Fatalf("c2 should enqueue, got error: %v", err)
	}
	_, err3 := b.Process(ctx, RequestItem{ChunkID: "c3", Text: "text"})
	if !errors.Is(err3, pii.ErrDetectorUnavailable) {
		t.Fatalf("c3 expected ErrDetectorUnavailable, got %v", err3)
	}
}

type blockingClient struct {
	release chan struct{}
	started chan struct{}
	once    sync.Once
}

func (b *blockingClient) ProcessBatch(_ context.Context, _ BatchRequest) (BatchResponse, error) {
	b.once.Do(func() { close(b.started) })
	<-b.release
	return BatchResponse{}, nil
}

func TestBatcherCancelledContext(t *testing.T) {
	fc := &fakeClient{resp: BatchResponse{}}
	cfg := DefaultBatchConfig()
	cfg.Workers = 1
	b, err := NewBatcher(fc, cfg)
	if err != nil {
		t.Fatalf("NewBatcher returned error: %v", err)
	}
	t.Cleanup(b.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = b.Process(ctx, RequestItem{ChunkID: "c1", Text: "text"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestBatcherMultipleWorkers(t *testing.T) {
	cfg := DefaultBatchConfig()
	cfg.MaxItems = 2
	cfg.MaxWait = 10 * time.Millisecond
	cfg.Workers = 2
	b, err := NewBatcher(echoClient{}, cfg)
	if err != nil {
		t.Fatalf("NewBatcher returned error: %v", err)
	}
	t.Cleanup(b.Close)

	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i, id := range []string{"c1", "c2", "c3", "c4"} {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			_, errs[i] = b.Process(ctx, RequestItem{ChunkID: id, Text: "text"})
		}(i, id)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Process %d returned error: %v", i, err)
		}
	}
}

// echoClient возвращает результат для каждого chunk_id из запроса.
type echoClient struct{}

func (echoClient) ProcessBatch(_ context.Context, req BatchRequest) (BatchResponse, error) {
	results := make([]ChunkResult, 0, len(req.Items))
	for _, item := range req.Items {
		results = append(results, ChunkResult{ChunkID: item.ChunkID})
	}
	return BatchResponse{
		BatchID:      req.BatchID,
		ModelVersion: "v1",
		OffsetUnit:   OffsetsUnicodeCodePoints,
		Results:      results,
	}, nil
}

func TestBatcherDoubleClose(t *testing.T) {
	fc := &fakeClient{resp: BatchResponse{}}
	cfg := DefaultBatchConfig()
	cfg.Workers = 1
	b, err := NewBatcher(fc, cfg)
	if err != nil {
		t.Fatalf("NewBatcher returned error: %v", err)
	}

	b.Close()
	b.Close() // не должно паниковать

	_, err = b.Process(context.Background(), RequestItem{ChunkID: "c1", Text: "text"})
	if !errors.Is(err, pii.ErrDetectorUnavailable) {
		t.Fatalf("expected ErrDetectorUnavailable after Close, got %v", err)
	}
}

func TestNewBatcherNilClient(t *testing.T) {
	if _, err := NewBatcher(nil, DefaultBatchConfig()); err == nil {
		t.Fatal("expected error for nil client")
	}
}

// blockingCtxClient блокируется, пока переданный context не отменён.
type blockingCtxClient struct {
	started chan struct{}
	once    sync.Once
}

func (b *blockingCtxClient) ProcessBatch(ctx context.Context, _ BatchRequest) (BatchResponse, error) {
	b.once.Do(func() { close(b.started) })
	<-ctx.Done()
	return BatchResponse{}, ctx.Err()
}

func TestBatcherCloseDuringProcess(t *testing.T) {
	bc := &blockingCtxClient{started: make(chan struct{})}
	cfg := DefaultBatchConfig()
	cfg.MaxItems = 1
	cfg.MaxWait = time.Hour
	cfg.Workers = 1
	b, err := NewBatcher(bc, cfg)
	if err != nil {
		t.Fatalf("NewBatcher returned error: %v", err)
	}

	ctx := context.Background()

	// Первый Process занимает воркера и блокируется в RPC.
	done1 := make(chan struct{})
	go func() {
		_, _ = b.Process(ctx, RequestItem{ChunkID: "c1", Text: "text"})
		close(done1)
	}()
	<-bc.started

	// Второй Process попадает в очередь.
	done2 := make(chan struct{})
	go func() {
		_, _ = b.Process(ctx, RequestItem{ChunkID: "c2", Text: "text"})
		close(done2)
	}()

	closeDone := make(chan struct{})
	go func() {
		b.Close()
		close(closeDone)
	}()

	select {
	case <-done1:
	case <-time.After(5 * time.Second):
		t.Fatal("first Process did not finish")
	}
	select {
	case <-done2:
	case <-time.After(5 * time.Second):
		t.Fatal("second Process did not finish")
	}
	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not finish")
	}
}
