package ml

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

// batchCollector собирает jobs из очереди в batch согласно BatchConfig.
// Хранит один pending job — когда следующий job уже взят из очереди, но не
// помещается в текущий batch по лимиту символов.
type batchCollector struct {
	queue   *itemQueue
	cfg     BatchConfig
	pending *batchJob
}

// newBatchCollector создаёт collector и проверяет BatchConfig и queue.
func newBatchCollector(queue *itemQueue, cfg BatchConfig) (*batchCollector, error) {
	if queue == nil {
		return nil, errors.New("queue must not be nil")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &batchCollector{queue: queue, cfg: cfg}, nil
}

// nextBatch дожидается первого подходящего job, после него запускает один
// time.Timer на MaxWait и собирает остальные jobs, пока не достигнут MaxItems,
// MaxCodePoints или не сработает таймер. Если следующий job не помещается по
// лимиту символов, он сохраняется как pending и возвращается текущий batch.
// Следующий вызов сначала берёт pending и только потом читает очередь.
// Порядок jobs остаётся FIFO.
func (c *batchCollector) nextBatch(ctx context.Context) ([]batchJob, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Находим первый подходящий job (из pending или из очереди).
	var first batchJob
	for {
		var job batchJob
		if c.pending != nil {
			job = *c.pending
			c.pending = nil
		} else {
			j, err := c.queue.dequeue(ctx)
			if err != nil {
				return nil, err
			}
			job = j
		}

		if err := job.ctx.Err(); err != nil {
			job.result <- jobResult{err: err}
			continue
		}
		size := utf8.RuneCountInString(job.item.Text)
		if size > c.cfg.MaxCodePoints {
			job.result <- jobResult{err: fmt.Errorf("item exceeds max code points: %w", pii.ErrPayloadTooLarge)}
			continue
		}

		first = job
		break
	}

	batch := []batchJob{first}
	total := utf8.RuneCountInString(first.item.Text)

	// Если первый job уже заполнил лимит символов, возвращаем сразу.
	if total >= c.cfg.MaxCodePoints {
		return batch, nil
	}

	timer := time.NewTimer(c.cfg.MaxWait)
	defer timer.Stop()

	for len(batch) < c.cfg.MaxItems {
		if err := ctx.Err(); err != nil {
			return c.failBatch(batch, err)
		}

		select {
		case job := <-c.queue.ch:
			if err := job.ctx.Err(); err != nil {
				job.result <- jobResult{err: err}
				continue
			}
			size := utf8.RuneCountInString(job.item.Text)
			if size > c.cfg.MaxCodePoints {
				job.result <- jobResult{err: fmt.Errorf("item exceeds max code points: %w", pii.ErrPayloadTooLarge)}
				continue
			}
			if total+size > c.cfg.MaxCodePoints {
				c.pending = &job
				return batch, nil
			}
			batch = append(batch, job)
			total += size
			if total >= c.cfg.MaxCodePoints {
				return batch, nil
			}
		case <-timer.C:
			return batch, nil
		case <-ctx.Done():
			return c.failBatch(batch, ctx.Err())
		}
	}

	return batch, nil
}

// failBatch отправляет каждому собранному job его context ошибку через result
// канал и возвращает ошибку.
func (c *batchCollector) failBatch(batch []batchJob, err error) ([]batchJob, error) {
	for _, job := range batch {
		job.result <- jobResult{err: err}
	}
	return nil, err
}