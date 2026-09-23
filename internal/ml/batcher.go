package ml

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

// Batcher исполняет очередь ML: собирает jobs в batch и отправляет их в
// client.ProcessBatch.
type Batcher struct {
	client Client
	cfg    BatchConfig
	queue  *itemQueue

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	seq    atomic.Uint64
	closed atomic.Bool
	once   sync.Once
}

// NewBatcher создаёт Batcher, проверяет client и cfg, создаёт очередь и
// запускает cfg.Workers воркеров.
func NewBatcher(client Client, cfg BatchConfig) (*Batcher, error) {
	if client == nil {
		return nil, errors.New("client must not be nil")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	queue, err := newItemQueue(cfg.QueueCapacity)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	b := &Batcher{
		client: client,
		cfg:    cfg,
		queue:  queue,
		ctx:    ctx,
		cancel: cancel,
	}

	b.wg.Add(cfg.Workers)
	for i := 0; i < cfg.Workers; i++ {
		go b.worker()
	}

	return b, nil
}

// Process ставит item в очередь и ждёт результат или отмену context.
func (b *Batcher) Process(ctx context.Context, item RequestItem) (ChunkResult, error) {
	if err := ctx.Err(); err != nil {
		return ChunkResult{}, err
	}
	if b.closed.Load() {
		return ChunkResult{}, pii.ErrDetectorUnavailable
	}
	if item.ChunkID == "" {
		return ChunkResult{}, errors.New("chunk id must not be empty")
	}
	if !utf8.ValidString(item.Text) {
		return ChunkResult{}, errors.New("text is not valid UTF-8")
	}

	job := batchJob{
		ctx:    ctx,
		item:   item,
		result: make(chan jobResult, 1),
	}
	if err := b.queue.enqueue(ctx, job); err != nil {
		return ChunkResult{}, err
	}

	select {
	case r := <-job.result:
		return r.result, r.err
	case <-ctx.Done():
		return ChunkResult{}, ctx.Err()
	case <-b.ctx.Done():
		return ChunkResult{}, pii.ErrDetectorUnavailable
	}
}

// Close отменяет воркеров, ждёт их завершения и безопасно вызывается
// несколько раз.
func (b *Batcher) Close() {
	b.once.Do(func() {
		b.closed.Store(true)
		b.cancel()
		b.wg.Wait()
	})
}

func (b *Batcher) worker() {
	defer b.wg.Done()

	collector, err := newBatchCollector(b.queue, b.cfg)
	if err != nil {
		return
	}

	for {
		batch, err := collector.nextBatch(b.ctx)
		if err != nil {
			return
		}
		b.processBatch(batch)
	}
}

func (b *Batcher) processBatch(batch []batchJob) {
	batchID := fmt.Sprintf("batch-%d", b.seq.Add(1))
	req := BatchRequest{
		BatchID:    batchID,
		OffsetUnit: OffsetsUnicodeCodePoints,
		Items:      make([]RequestItem, 0, len(batch)),
	}
	// Назначаем каждому job уникальный транспортный chunk_id, чтобы чанки
	// разных запросов с одинаковым исходным chunk_id не перепутались.
	transportIDs := make([]string, len(batch))
	for i, job := range batch {
		transportID := fmt.Sprintf("%s-%d", batchID, i)
		transportIDs[i] = transportID
		req.Items = append(req.Items, RequestItem{
			ChunkID: transportID,
			Text:    job.item.Text,
		})
	}

	resp, err := b.client.ProcessBatch(b.ctx, req)
	if err != nil {
		for _, job := range batch {
			job.result <- jobResult{err: err}
		}
		return
	}

	byChunk := make(map[string]ChunkResult, len(resp.Results))
	for _, r := range resp.Results {
		byChunk[r.ChunkID] = r
	}

	for i, job := range batch {
		r, ok := byChunk[transportIDs[i]]
		if !ok {
			job.result <- jobResult{err: pii.ErrDetectorUnavailable}
			continue
		}
		if r.ErrorCode != "" {
			job.result <- jobResult{err: pii.ErrDetectorUnavailable}
			continue
		}
		// Восстанавливаем исходный chunk_id из RequestItem.
		r.ChunkID = job.item.ChunkID
		job.result <- jobResult{result: r}
	}
}
