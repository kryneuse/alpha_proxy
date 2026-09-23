package ml

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

// BatchConfig — настройки набора batch из chunks.
type BatchConfig struct {
	MaxItems      int
	MaxCodePoints int
	MaxWait       time.Duration
	QueueCapacity int
	Workers       int
	RPCTimeout    time.Duration
}

// DefaultBatchConfig возвращает временные значения по умолчанию.
// Эти значения потом настраиваются после тестов с реальным ML сервисом.
func DefaultBatchConfig() BatchConfig {
	return BatchConfig{
		MaxItems:      32,
		MaxCodePoints: 11200,
		MaxWait:       5 * time.Millisecond,
		QueueCapacity: 1024,
		Workers:       4,
		RPCTimeout:    2 * time.Second,
	}
}

// Validate проверяет корректность конфигурации.
func (c BatchConfig) Validate() error {
	if c.MaxItems <= 0 {
		return errors.New("max items must be > 0")
	}
	if c.MaxCodePoints <= 0 {
		return errors.New("max code points must be > 0")
	}
	if c.MaxWait <= 0 {
		return errors.New("max wait must be > 0")
	}
	if c.QueueCapacity <= 0 {
		return errors.New("queue capacity must be > 0")
	}
	if c.Workers <= 0 {
		return errors.New("workers must be > 0")
	}
	if c.RPCTimeout <= 0 {
		return errors.New("rpc timeout must be > 0")
	}
	return nil
}

// TakeBatch берёт элементы с начала списка, пока не достигнут MaxItems или
// MaxCodePoints. Размер считается в Unicode code points через
// utf8.RuneCountInString, а не в байтах. Возвращает выбранные элементы и
// оставшиеся. Входной slice и его элементы не изменяются.
func TakeBatch(ctx context.Context, items []RequestItem, cfg BatchConfig) ([]RequestItem, []RequestItem, error) {
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}
	if len(items) == 0 {
		return nil, nil, nil
	}

	selected := make([]RequestItem, 0, cfg.MaxItems)
	total := 0

	for i, item := range items {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}

		size := utf8.RuneCountInString(item.Text)
		if i == 0 && size > cfg.MaxCodePoints {
			return nil, nil, fmt.Errorf("item exceeds max code points: %w", pii.ErrPayloadTooLarge)
		}
		if len(selected) >= cfg.MaxItems || total+size > cfg.MaxCodePoints {
			break
		}

		selected = append(selected, item)
		total += size
	}

	return selected, items[len(selected):], nil
}
