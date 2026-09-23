package store

import (
	"context"
	"testing"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

func TestCapacityForcesExpirySweepBeforeScheduledCleanup(t *testing.T) {
	s := NewMemoryStore(1).(*memoryStore)
	s.nextPurge = time.Now().Add(time.Hour)
	s.sessions["expired"] = pii.Session{PayloadID: "expired", ExpiresAt: time.Now().Add(-time.Second)}
	added, err := s.PutIfAbsent(context.Background(), &pii.Session{PayloadID: "new", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil || !added {
		t.Fatalf("expired capacity was not reclaimed: %v", err)
	}
	if _, exists := s.sessions["expired"]; exists {
		t.Fatal("expired session retained")
	}
}
