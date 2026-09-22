package ml

import (
	"context"
	"errors"
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

func newTestJob(ctx context.Context, id string) batchJob {
	return batchJob{
		ctx:    ctx,
		item:   RequestItem{ChunkID: id, Text: "text"},
		result: make(chan jobResult, 1),
	}
}

func TestQueueFIFO(t *testing.T) {
	q, err := newItemQueue(3)
	if err != nil {
		t.Fatalf("newItemQueue returned error: %v", err)
	}
	ctx := context.Background()

	jobs := []batchJob{newTestJob(ctx, "a"), newTestJob(ctx, "b"), newTestJob(ctx, "c")}
	for _, j := range jobs {
		if err := q.enqueue(ctx, j); err != nil {
			t.Fatalf("enqueue returned error: %v", err)
		}
	}

	for i, want := range jobs {
		got, err := q.dequeue(ctx)
		if err != nil {
			t.Fatalf("dequeue returned error: %v", err)
		}
		if got.item.ChunkID != want.item.ChunkID {
			t.Fatalf("expected FIFO order, got %q at position %d, want %q", got.item.ChunkID, i, want.item.ChunkID)
		}
	}
}

func TestQueueFillToCapacity(t *testing.T) {
	q, err := newItemQueue(2)
	if err != nil {
		t.Fatalf("newItemQueue returned error: %v", err)
	}
	ctx := context.Background()

	if err := q.enqueue(ctx, newTestJob(ctx, "a")); err != nil {
		t.Fatalf("enqueue returned error: %v", err)
	}
	if err := q.enqueue(ctx, newTestJob(ctx, "b")); err != nil {
		t.Fatalf("enqueue returned error: %v", err)
	}
}

func TestQueueOverflow(t *testing.T) {
	q, err := newItemQueue(1)
	if err != nil {
		t.Fatalf("newItemQueue returned error: %v", err)
	}
	ctx := context.Background()

	if err := q.enqueue(ctx, newTestJob(ctx, "a")); err != nil {
		t.Fatalf("enqueue returned error: %v", err)
	}

	err = q.enqueue(ctx, newTestJob(ctx, "b"))
	if !errors.Is(err, pii.ErrDetectorUnavailable) {
		t.Fatalf("expected ErrDetectorUnavailable, got %v", err)
	}
}

func TestQueueEnqueueCancelledContext(t *testing.T) {
	q, err := newItemQueue(1)
	if err != nil {
		t.Fatalf("newItemQueue returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = q.enqueue(ctx, newTestJob(ctx, "a"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestQueueDequeueCancelledContext(t *testing.T) {
	q, err := newItemQueue(1)
	if err != nil {
		t.Fatalf("newItemQueue returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = q.dequeue(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestQueueDequeueAfterEnqueue(t *testing.T) {
	q, err := newItemQueue(1)
	if err != nil {
		t.Fatalf("newItemQueue returned error: %v", err)
	}
	ctx := context.Background()

	job := newTestJob(ctx, "a")
	if err := q.enqueue(ctx, job); err != nil {
		t.Fatalf("enqueue returned error: %v", err)
	}

	got, err := q.dequeue(ctx)
	if err != nil {
		t.Fatalf("dequeue returned error: %v", err)
	}
	if got.item.ChunkID != "a" {
		t.Fatalf("expected chunk a, got %q", got.item.ChunkID)
	}
}

func TestQueueInvalidCapacity(t *testing.T) {
	if _, err := newItemQueue(0); err == nil {
		t.Fatal("expected error for zero capacity")
	}
	if _, err := newItemQueue(-1); err == nil {
		t.Fatal("expected error for negative capacity")
	}
}

func TestQueueDequeueCancelledContextKeepsJob(t *testing.T) {
	q, err := newItemQueue(1)
	if err != nil {
		t.Fatalf("newItemQueue returned error: %v", err)
	}
	ctx := context.Background()

	job := newTestJob(ctx, "a")
	if err := q.enqueue(ctx, job); err != nil {
		t.Fatalf("enqueue returned error: %v", err)
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = q.dequeue(cancelCtx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// Job должен остаться в очереди.
	got, err := q.dequeue(ctx)
	if err != nil {
		t.Fatalf("dequeue returned error: %v", err)
	}
	if got.item.ChunkID != "a" {
		t.Fatalf("expected job a still in queue, got %q", got.item.ChunkID)
	}
}
