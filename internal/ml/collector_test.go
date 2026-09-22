package ml

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

func newCollector(t *testing.T, capacity int, cfg BatchConfig) *batchCollector {
	t.Helper()
	q, err := newItemQueue(capacity)
	if err != nil {
		t.Fatalf("newItemQueue returned error: %v", err)
	}
	c, err := newBatchCollector(q, cfg)
	if err != nil {
		t.Fatalf("newBatchCollector returned error: %v", err)
	}
	return c
}

func nextBatchWithTimeout(t *testing.T, c *batchCollector, ctx context.Context, timeout time.Duration) ([]batchJob, error) {
	t.Helper()
	type result struct {
		batch []batchJob
		err   error
	}
	done := make(chan result, 1)
	go func() {
		batch, err := c.nextBatch(ctx)
		done <- result{batch: batch, err: err}
	}()
	select {
	case r := <-done:
		return r.batch, r.err
	case <-time.After(timeout):
		t.Fatal("nextBatch did not return within timeout")
		return nil, nil
	}
}

func TestCollectorByMaxItems(t *testing.T) {
	cfg := DefaultBatchConfig()
	cfg.MaxItems = 2
	cfg.MaxWait = time.Hour
	c := newCollector(t, 10, cfg)
	ctx := context.Background()

	for _, id := range []string{"a", "b", "c"} {
		if err := c.queue.enqueue(ctx, newTestJob(ctx, id)); err != nil {
			t.Fatalf("enqueue returned error: %v", err)
		}
	}

	batch, err := c.nextBatch(ctx)
	if err != nil {
		t.Fatalf("nextBatch returned error: %v", err)
	}
	if len(batch) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(batch))
	}
	if batch[0].item.ChunkID != "a" || batch[1].item.ChunkID != "b" {
		t.Fatalf("unexpected batch order: %+v", batch)
	}
}

func TestCollectorByMaxCodePoints(t *testing.T) {
	cfg := DefaultBatchConfig()
	cfg.MaxCodePoints = 150
	cfg.MaxWait = time.Hour
	c := newCollector(t, 10, cfg)
	ctx := context.Background()

	for _, id := range []string{"a", "b"} {
		job := newTestJob(ctx, id)
		job.item.Text = strings.Repeat("x", 100)
		if err := c.queue.enqueue(ctx, job); err != nil {
			t.Fatalf("enqueue returned error: %v", err)
		}
	}

	batch, err := c.nextBatch(ctx)
	if err != nil {
		t.Fatalf("nextBatch returned error: %v", err)
	}
	if len(batch) != 1 {
		t.Fatalf("expected 1 job (100 <= 150), got %d", len(batch))
	}
}

func TestCollectorPendingOverflow(t *testing.T) {
	cfg := DefaultBatchConfig()
	cfg.MaxCodePoints = 150
	cfg.MaxWait = 10 * time.Millisecond
	c := newCollector(t, 10, cfg)
	ctx := context.Background()

	for _, id := range []string{"a", "b"} {
		job := newTestJob(ctx, id)
		job.item.Text = strings.Repeat("x", 100)
		if err := c.queue.enqueue(ctx, job); err != nil {
			t.Fatalf("enqueue returned error: %v", err)
		}
	}

	// Первый batch: job a (100). Job b (100) не помещается (200 > 150) — pending.
	batch, err := nextBatchWithTimeout(t, c, ctx, 2*time.Second)
	if err != nil {
		t.Fatalf("nextBatch returned error: %v", err)
	}
	if len(batch) != 1 || batch[0].item.ChunkID != "a" {
		t.Fatalf("expected batch with job a, got %+v", batch)
	}

	// Второй batch должен сначала взять pending job b.
	batch2, err := nextBatchWithTimeout(t, c, ctx, 2*time.Second)
	if err != nil {
		t.Fatalf("nextBatch returned error: %v", err)
	}
	if len(batch2) != 1 || batch2[0].item.ChunkID != "b" {
		t.Fatalf("expected pending job b first, got %+v", batch2)
	}
}

func TestCollectorFIFO(t *testing.T) {
	cfg := DefaultBatchConfig()
	cfg.MaxItems = 3
	cfg.MaxWait = time.Hour
	c := newCollector(t, 10, cfg)
	ctx := context.Background()

	for _, id := range []string{"a", "b", "c"} {
		if err := c.queue.enqueue(ctx, newTestJob(ctx, id)); err != nil {
			t.Fatalf("enqueue returned error: %v", err)
		}
	}

	batch, err := c.nextBatch(ctx)
	if err != nil {
		t.Fatalf("nextBatch returned error: %v", err)
	}
	if len(batch) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(batch))
	}
	for i, want := range []string{"a", "b", "c"} {
		if batch[i].item.ChunkID != want {
			t.Fatalf("expected FIFO order, got %q at %d, want %q", batch[i].item.ChunkID, i, want)
		}
	}
}

func TestCollectorMaxWait(t *testing.T) {
	cfg := DefaultBatchConfig()
	cfg.MaxItems = 10
	cfg.MaxWait = 10 * time.Millisecond
	c := newCollector(t, 10, cfg)
	ctx := context.Background()

	if err := c.queue.enqueue(ctx, newTestJob(ctx, "a")); err != nil {
		t.Fatalf("enqueue returned error: %v", err)
	}

	type result struct {
		batch []batchJob
		err   error
	}
	done := make(chan result, 1)
	go func() {
		batch, err := c.nextBatch(ctx)
		done <- result{batch: batch, err: err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("nextBatch returned error: %v", r.err)
		}
		if len(r.batch) != 1 || r.batch[0].item.ChunkID != "a" {
			t.Fatalf("expected batch with job a after MaxWait, got %+v", r.batch)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("nextBatch did not return within timeout")
	}
}

func TestCollectorSkipsCancelledJob(t *testing.T) {
	cfg := DefaultBatchConfig()
	cfg.MaxItems = 10
	cfg.MaxWait = 10 * time.Millisecond
	c := newCollector(t, 10, cfg)
	ctx := context.Background()

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.queue.enqueue(ctx, newTestJob(cancelCtx, "bad")); err != nil {
		t.Fatalf("enqueue returned error: %v", err)
	}
	if err := c.queue.enqueue(ctx, newTestJob(ctx, "good")); err != nil {
		t.Fatalf("enqueue returned error: %v", err)
	}

	batch, err := nextBatchWithTimeout(t, c, ctx, 2*time.Second)
	if err != nil {
		t.Fatalf("nextBatch returned error: %v", err)
	}
	if len(batch) != 1 || batch[0].item.ChunkID != "good" {
		t.Fatalf("expected batch with good job, got %+v", batch)
	}
}

func TestCollectorTooLargeJob(t *testing.T) {
	cfg := DefaultBatchConfig()
	cfg.MaxCodePoints = 100
	cfg.MaxWait = 10 * time.Millisecond
	c := newCollector(t, 10, cfg)
	ctx := context.Background()

	big := newTestJob(ctx, "big")
	big.item.Text = strings.Repeat("x", 200)
	if err := c.queue.enqueue(ctx, big); err != nil {
		t.Fatalf("enqueue returned error: %v", err)
	}
	if err := c.queue.enqueue(ctx, newTestJob(ctx, "good")); err != nil {
		t.Fatalf("enqueue returned error: %v", err)
	}

	batch, err := nextBatchWithTimeout(t, c, ctx, 2*time.Second)
	if err != nil {
		t.Fatalf("nextBatch returned error: %v", err)
	}
	if len(batch) != 1 || batch[0].item.ChunkID != "good" {
		t.Fatalf("expected batch with good job, got %+v", batch)
	}

	// Big job должен получить ошибку в свой result канал.
	select {
	case r := <-big.result:
		if !errors.Is(r.err, pii.ErrPayloadTooLarge) {
			t.Fatalf("expected ErrPayloadTooLarge, got %v", r.err)
		}
	default:
		t.Fatal("expected error sent to big job result channel")
	}
}

func TestCollectorCancelledContext(t *testing.T) {
	cfg := DefaultBatchConfig()
	cfg.MaxWait = time.Hour
	c := newCollector(t, 10, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.nextBatch(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestCollectorImmediateReturnAtExactMaxCodePoints(t *testing.T) {
	cfg := DefaultBatchConfig()
	cfg.MaxCodePoints = 100
	cfg.MaxWait = time.Hour
	c := newCollector(t, 10, cfg)
	ctx := context.Background()

	job := newTestJob(ctx, "a")
	job.item.Text = strings.Repeat("x", 100)
	if err := c.queue.enqueue(ctx, job); err != nil {
		t.Fatalf("enqueue returned error: %v", err)
	}

	// Первый job ровно MaxCodePoints — batch должен вернуться сразу, без
	// ожидания MaxWait.
	batch, err := nextBatchWithTimeout(t, c, ctx, 2*time.Second)
	if err != nil {
		t.Fatalf("nextBatch returned error: %v", err)
	}
	if len(batch) != 1 || batch[0].item.ChunkID != "a" {
		t.Fatalf("expected batch with job a, got %+v", batch)
	}
}

func TestCollectorCancelledJobReceivesError(t *testing.T) {
	cfg := DefaultBatchConfig()
	cfg.MaxItems = 1
	cfg.MaxWait = time.Hour
	c := newCollector(t, 10, cfg)
	ctx := context.Background()

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	bad := newTestJob(cancelCtx, "bad")
	if err := c.queue.enqueue(ctx, bad); err != nil {
		t.Fatalf("enqueue returned error: %v", err)
	}
	good := newTestJob(ctx, "good")
	if err := c.queue.enqueue(ctx, good); err != nil {
		t.Fatalf("enqueue returned error: %v", err)
	}

	batch, err := nextBatchWithTimeout(t, c, ctx, 2*time.Second)
	if err != nil {
		t.Fatalf("nextBatch returned error: %v", err)
	}
	if len(batch) != 1 || batch[0].item.ChunkID != "good" {
		t.Fatalf("expected batch with good job, got %+v", batch)
	}

	// Отменённый job должен получить context ошибку в свой result канал.
	select {
	case r := <-bad.result:
		if !errors.Is(r.err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", r.err)
		}
	default:
		t.Fatal("expected error sent to cancelled job result channel")
	}
}

func TestCollectorCancelledAfterCollectionStarted(t *testing.T) {
	cfg := DefaultBatchConfig()
	cfg.MaxItems = 10
	cfg.MaxWait = time.Hour
	c := newCollector(t, 10, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	job := newTestJob(ctx, "a")
	if err := c.queue.enqueue(ctx, job); err != nil {
		t.Fatalf("enqueue returned error: %v", err)
	}

	// Collector сначала заберёт job, а затем получит context.DeadlineExceeded.
	_, err := c.nextBatch(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}

	// Собранный job должен получить ту же ошибку через свой result канал.
	select {
	case r := <-job.result:
		if !errors.Is(r.err, context.DeadlineExceeded) {
			t.Fatalf("expected context.DeadlineExceeded, got %v", r.err)
		}
	default:
		t.Fatal("expected error sent to collected job result channel")
	}
}

func TestCollectorNilQueue(t *testing.T) {
	cfg := DefaultBatchConfig()
	if _, err := newBatchCollector(nil, cfg); err == nil {
		t.Fatal("expected error for nil queue")
	}
}
