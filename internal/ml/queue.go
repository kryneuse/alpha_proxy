package ml

import (
	"context"
	"errors"
	"fmt"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

// batchJob — единица работы для будущего batcher: chunk и канал для результата.
type batchJob struct {
	ctx    context.Context
	item   RequestItem
	result chan jobResult
}

// jobResult — результат обработки одного chunk.
type jobResult struct {
	result ChunkResult
	err    error
}

// itemQueue — FIFO очередь на основе буферизированного канала с фиксированной
// capacity. Не содержит бесконечных очередей, дополнительных slice и не
// запускает goroutine.
type itemQueue struct {
	ch chan batchJob
}

// newItemQueue создаёт очередь с заданной capacity.
func newItemQueue(capacity int) (*itemQueue, error) {
	if capacity <= 0 {
		return nil, errors.New("queue capacity must be > 0")
	}
	return &itemQueue{ch: make(chan batchJob, capacity)}, nil
}

// enqueue добавляет job в очередь. Если очередь заполнена, не ждёт
// освобождения места и сразу возвращает ошибку с оборачиванием
// pii.ErrDetectorUnavailable. В ошибку не добавляются chunk_id, текст или
// другие данные запроса.
func (q *itemQueue) enqueue(ctx context.Context, job batchJob) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case q.ch <- job:
		return nil
	default:
		return fmt.Errorf("queue is full: %w", pii.ErrDetectorUnavailable)
	}
}

// dequeue ожидает следующий job или отмену context. Сохраняет FIFO порядок.
func (q *itemQueue) dequeue(ctx context.Context) (batchJob, error) {
	if err := ctx.Err(); err != nil {
		return batchJob{}, err
	}
	select {
	case job := <-q.ch:
		return job, nil
	case <-ctx.Done():
		return batchJob{}, ctx.Err()
	}
}
