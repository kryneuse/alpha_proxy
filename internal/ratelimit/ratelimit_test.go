package ratelimit

import (
	"testing"
	"time"
)

// fakeClock is a deterministic clock for tests.
type fakeClock struct {
	t time.Time
}

func (c *fakeClock) Now() time.Time { return c.t }

func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func TestTokenBucketAllowsBurst(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	b := NewTokenBucket(1, 3, clock)

	for i := 0; i < 3; i++ {
		ok, delay := b.Allow()
		if !ok {
			t.Fatalf("request %d denied, want allowed", i)
		}
		if delay != 0 {
			t.Fatalf("request %d delay = %v, want 0", i, delay)
		}
	}
}

func TestTokenBucketDeniesAfterBurst(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	b := NewTokenBucket(1, 2, clock)

	b.Allow()
	b.Allow()
	ok, delay := b.Allow()
	if ok {
		t.Fatal("request allowed, want denied")
	}
	if delay <= 0 {
		t.Fatalf("delay = %v, want positive", delay)
	}
}

func TestTokenBucketRefillsAfterTime(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	b := NewTokenBucket(1, 1, clock)

	if ok, _ := b.Allow(); !ok {
		t.Fatal("first request denied")
	}
	if ok, _ := b.Allow(); ok {
		t.Fatal("second request allowed, want denied")
	}

	// Advance 1 second: one token refills.
	clock.advance(time.Second)
	if ok, _ := b.Allow(); !ok {
		t.Fatal("request after refill denied")
	}
}

func TestTokenBucketRetryDelayRoundsUp(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	// rate 1 token/sec, burst 1.
	b := NewTokenBucket(1, 1, clock)
	b.Allow()

	// After 0.5s, deficit is 0.5 tokens -> delay ~0.5s.
	clock.advance(500 * time.Millisecond)
	ok, delay := b.Allow()
	if ok {
		t.Fatal("request allowed, want denied")
	}
	if delay <= 0 || delay > time.Second {
		t.Fatalf("delay = %v, want in (0, 1s]", delay)
	}
}

func TestConsumerLimiterIndependentBuckets(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	l := NewConsumerLimiter(1, 1, []string{"a", "b"}, clock)

	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("consumer a first request denied")
	}
	if ok, _ := l.Allow("a"); ok {
		t.Fatal("consumer a second request allowed, want denied")
	}
	// Consumer b has its own bucket and is still allowed.
	if ok, _ := l.Allow("b"); !ok {
		t.Fatal("consumer b request denied, want allowed")
	}
}

func TestConsumerLimiterUnknownNoState(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	l := NewConsumerLimiter(1, 1, []string{"a"}, clock)

	before := len(l.buckets)
	// Unknown consumer is always allowed and creates no state.
	for i := 0; i < 5; i++ {
		if ok, _ := l.Allow("unknown"); !ok {
			t.Fatalf("unknown consumer denied on request %d", i)
		}
	}
	if got := len(l.buckets); got != before {
		t.Fatalf("bucket map size = %d, want %d (unknown consumer created state)", got, before)
	}
	// Known consumer still has its own full bucket.
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("known consumer denied")
	}
}
