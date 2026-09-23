package ml

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// BatchConfigFromEnv exposes bounded batching and RPC concurrency for deployment.
func BatchConfigFromEnv() (BatchConfig, error) {
	c := DefaultBatchConfig()
	for key, target := range map[string]*int{
		"ALPHA_PROXY_ML_BATCH_ITEMS":      &c.MaxItems,
		"ALPHA_PROXY_ML_BATCH_CODEPOINTS": &c.MaxCodePoints,
		"ALPHA_PROXY_ML_QUEUE_CAPACITY":   &c.QueueCapacity,
		"ALPHA_PROXY_ML_RPC_WORKERS":      &c.Workers,
	} {
		if value := os.Getenv(key); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return c, fmt.Errorf("invalid %s: %w", key, err)
			}
			*target = parsed
		}
	}
	for key, target := range map[string]*time.Duration{
		"ALPHA_PROXY_ML_BATCH_WAIT":  &c.MaxWait,
		"ALPHA_PROXY_ML_RPC_TIMEOUT": &c.RPCTimeout,
	} {
		if value := os.Getenv(key); value != "" {
			parsed, err := time.ParseDuration(value)
			if err != nil {
				return c, fmt.Errorf("invalid %s: %w", key, err)
			}
			*target = parsed
		}
	}
	return c, c.Validate()
}
