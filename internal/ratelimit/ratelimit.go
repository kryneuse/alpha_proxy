// Package ratelimit provides a thread-safe token bucket and a per-consumer
// limiter for the HTTP contour. It uses only the standard library and no
// background goroutines.
package ratelimit

import (
	"math"
	"sync"
	"time"
)

// Clock abstracts time for deterministic tests.
type Clock interface {
	Now() time.Time
}

// RealClock returns the current wall-clock time.
type RealClock struct{}

// Now implements Clock.
func (RealClock) Now() time.Time { return time.Now() }

// TokenBucket is a thread-safe token bucket. It starts full (burst tokens) and
// refills at rate tokens per second based on elapsed time.
type TokenBucket struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
	clock  Clock
}

// NewTokenBucket returns a bucket with the given rate (tokens/second) and burst.
func NewTokenBucket(rate float64, burst int, clock Clock) *TokenBucket {
	return &TokenBucket{
		rate:   rate,
		burst:  float64(burst),
		tokens: float64(burst),
		last:   clock.Now(),
		clock:  clock,
	}
}

// Allow consumes one token if available and returns the retry delay otherwise.
func (b *TokenBucket) Allow() (bool, time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.clock.Now()
	elapsed := now.Sub(b.last).Seconds()
	b.tokens = math.Min(b.burst, b.tokens+elapsed*b.rate)
	b.last = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	deficit := 1 - b.tokens
	delay := time.Duration(deficit / b.rate * float64(time.Second))
	return false, delay
}

// ConsumerLimiter holds one token bucket per known consumer. Unknown consumers
// are not limited and never create state.
type ConsumerLimiter struct {
	mu      sync.Mutex
	buckets map[string]*TokenBucket
}

// NewConsumerLimiter builds a limiter with a bucket for each known consumer.
func NewConsumerLimiter(rps float64, burst int, consumers []string, clock Clock) *ConsumerLimiter {
	l := &ConsumerLimiter{buckets: make(map[string]*TokenBucket, len(consumers))}
	for _, c := range consumers {
		l.buckets[c] = NewTokenBucket(rps, burst, clock)
	}
	return l
}

// Allow checks the bucket for consumerID. Unknown consumers are allowed without
// creating state.
func (l *ConsumerLimiter) Allow(consumerID string) (bool, time.Duration) {
	l.mu.Lock()
	b := l.buckets[consumerID]
	l.mu.Unlock()
	if b == nil {
		return true, 0
	}
	return b.Allow()
}
