package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

func newTestSession(payloadID string) *pii.Session {
	return &pii.Session{
		PayloadID:   payloadID,
		PayloadHash: sha256.Sum256([]byte(payloadID)),
		Mappings: []pii.TokenMapping{
			{Token: "tok1", Original: "orig1", Kind: pii.PIIKindEmail, Source: pii.SourceML, Start: 0, End: 5},
			{Token: "tok2", Original: "orig2", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 6, End: 11},
		},
		Status:    pii.SessionStatusProcessing,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
}

func TestPutIfAbsentAndGet(t *testing.T) {
	s := NewMemoryStore(10)
	ctx := context.Background()
	session := newTestSession("id-1")

	added, err := s.PutIfAbsent(ctx, session)
	if err != nil {
		t.Fatalf("PutIfAbsent returned error: %v", err)
	}
	if !added {
		t.Fatal("expected PutIfAbsent to return true")
	}

	got, err := s.Get(ctx, "id-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got == nil {
		t.Fatal("expected session to be returned")
	}
	if got.PayloadID != "id-1" || got.PayloadHash != sha256.Sum256([]byte("id-1")) {
		t.Fatalf("unexpected session: %+v", got)
	}
	if len(got.Mappings) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(got.Mappings))
	}
}

func TestPutIfAbsentDoesNotOverwrite(t *testing.T) {
	s := NewMemoryStore(10)
	ctx := context.Background()

	added, err := s.PutIfAbsent(ctx, newTestSession("id-1"))
	if err != nil || !added {
		t.Fatalf("first PutIfAbsent: added=%v err=%v", added, err)
	}

	overwrite := newTestSession("id-1")
	overwrite.PayloadHash = sha256.Sum256([]byte("changed"))

	added, err = s.PutIfAbsent(ctx, overwrite)
	if err != nil {
		t.Fatalf("second PutIfAbsent returned error: %v", err)
	}
	if added {
		t.Fatal("expected second PutIfAbsent to return false")
	}

	got, err := s.Get(ctx, "id-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.PayloadHash != sha256.Sum256([]byte("id-1")) {
		t.Fatalf("session was overwritten, got PayloadHash=%x", got.PayloadHash)
	}
}

func TestUpdateExistingSession(t *testing.T) {
	s := NewMemoryStore(10)
	ctx := context.Background()

	if _, err := s.PutIfAbsent(ctx, newTestSession("id-1")); err != nil {
		t.Fatalf("PutIfAbsent returned error: %v", err)
	}

	updated := newTestSession("id-1")
	updated.PayloadHash = sha256.Sum256([]byte("updated"))
	updated.Status = pii.SessionStatusReady

	if err := s.Update(ctx, updated); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	got, err := s.Get(ctx, "id-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.PayloadHash != sha256.Sum256([]byte("updated")) {
		t.Fatalf("expected updated PayloadHash, got %x", got.PayloadHash)
	}
	if got.Status != pii.SessionStatusReady {
		t.Fatalf("expected status READY, got %q", got.Status)
	}
}

func TestUpdateMissingSession(t *testing.T) {
	s := NewMemoryStore(10)
	ctx := context.Background()

	err := s.Update(ctx, newTestSession("missing"))
	if !errors.Is(err, pii.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestDeleteSession(t *testing.T) {
	s := NewMemoryStore(10)
	ctx := context.Background()

	if _, err := s.PutIfAbsent(ctx, newTestSession("id-1")); err != nil {
		t.Fatalf("PutIfAbsent returned error: %v", err)
	}

	if err := s.Delete(ctx, "id-1"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	if _, err := s.Get(ctx, "id-1"); !errors.Is(err, pii.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound after delete, got %v", err)
	}
}

func TestDeleteMissingSessionNoError(t *testing.T) {
	s := NewMemoryStore(10)
	ctx := context.Background()

	if err := s.Delete(ctx, "missing"); err != nil {
		t.Fatalf("expected no error on repeated delete, got %v", err)
	}
	if err := s.Delete(ctx, "missing"); err != nil {
		t.Fatalf("expected no error on second delete, got %v", err)
	}
}

func TestGetMissingSession(t *testing.T) {
	s := NewMemoryStore(10)
	ctx := context.Background()

	if _, err := s.Get(ctx, "missing"); !errors.Is(err, pii.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestGetExpiredSession(t *testing.T) {
	s := NewMemoryStore(10)
	ctx := context.Background()

	session := newTestSession("id-1")
	session.ExpiresAt = time.Now().Add(-time.Minute)

	if _, err := s.PutIfAbsent(ctx, session); err != nil {
		t.Fatalf("PutIfAbsent returned error: %v", err)
	}

	if _, err := s.Get(ctx, "id-1"); !errors.Is(err, pii.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound for expired session, got %v", err)
	}
}

func TestExpiredSessionsPurgedBeforeLimit(t *testing.T) {
	s := NewMemoryStore(2)
	ctx := context.Background()

	expired := newTestSession("expired-1")
	expired.ExpiresAt = time.Now().Add(-time.Minute)
	if _, err := s.PutIfAbsent(ctx, expired); err != nil {
		t.Fatalf("PutIfAbsent expired returned error: %v", err)
	}

	active := newTestSession("active-1")
	if _, err := s.PutIfAbsent(ctx, active); err != nil {
		t.Fatalf("PutIfAbsent active returned error: %v", err)
	}

	// Store is full (2 sessions), but the expired one should be purged
	// before the limit check, freeing a slot.
	third := newTestSession("active-2")
	added, err := s.PutIfAbsent(ctx, third)
	if err != nil {
		t.Fatalf("PutIfAbsent returned error: %v", err)
	}
	if !added {
		t.Fatal("expected expired session to be purged and slot freed")
	}

	if _, err := s.Get(ctx, "expired-1"); !errors.Is(err, pii.ErrSessionNotFound) {
		t.Fatalf("expected expired session to be gone, got %v", err)
	}
}

func TestLimitReachedReturnsErrStoreUnavailable(t *testing.T) {
	s := NewMemoryStore(1)
	ctx := context.Background()

	if _, err := s.PutIfAbsent(ctx, newTestSession("id-1")); err != nil {
		t.Fatalf("PutIfAbsent returned error: %v", err)
	}

	_, err := s.PutIfAbsent(ctx, newTestSession("id-2"))
	if !errors.Is(err, pii.ErrStoreUnavailable) {
		t.Fatalf("expected ErrStoreUnavailable, got %v", err)
	}
}

func TestPutIfAbsentNilSession(t *testing.T) {
	s := NewMemoryStore(10)
	ctx := context.Background()

	_, err := s.PutIfAbsent(ctx, nil)
	if !errors.Is(err, pii.ErrStoreUnavailable) {
		t.Fatalf("expected ErrStoreUnavailable for nil session, got %v", err)
	}
}

func TestUpdateNilSession(t *testing.T) {
	s := NewMemoryStore(10)
	ctx := context.Background()

	err := s.Update(ctx, nil)
	if !errors.Is(err, pii.ErrStoreUnavailable) {
		t.Fatalf("expected ErrStoreUnavailable for nil session, got %v", err)
	}
}

func TestCancelledContext(t *testing.T) {
	s := NewMemoryStore(10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := s.Get(ctx, "id-1"); err == nil {
		t.Fatal("expected error from Get with cancelled context")
	}
	if _, err := s.PutIfAbsent(ctx, newTestSession("id-1")); err == nil {
		t.Fatal("expected error from PutIfAbsent with cancelled context")
	}
	if err := s.Update(ctx, newTestSession("id-1")); err == nil {
		t.Fatal("expected error from Update with cancelled context")
	}
	if err := s.Delete(ctx, "id-1"); err == nil {
		t.Fatal("expected error from Delete with cancelled context")
	}
}

func TestDefensiveCopyOnPut(t *testing.T) {
	s := NewMemoryStore(10)
	ctx := context.Background()

	session := newTestSession("id-1")
	if _, err := s.PutIfAbsent(ctx, session); err != nil {
		t.Fatalf("PutIfAbsent returned error: %v", err)
	}

	// Mutate the original session and its slices after storing.
	session.PayloadHash = sha256.Sum256([]byte("mutated"))
	session.Mappings[0].Token = "mutated-token"
	session.Mappings[0].Original = "mutated-original"

	got, err := s.Get(ctx, "id-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.PayloadHash != sha256.Sum256([]byte("id-1")) {
		t.Fatalf("stored session was mutated, got PayloadHash=%x", got.PayloadHash)
	}
	if got.Mappings[0].Token != "tok1" || got.Mappings[0].Original != "orig1" {
		t.Fatalf("stored mappings were mutated, got %+v", got.Mappings[0])
	}
}

func TestConcurrentPutIfAbsent(t *testing.T) {
	s := NewMemoryStore(100)
	ctx := context.Background()

	const goroutines = 50
	start := make(chan struct{})
	var wg sync.WaitGroup

	var mu sync.Mutex
	addedCount := 0
	errCount := 0

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			added, err := s.PutIfAbsent(ctx, newTestSession("id-1"))
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errCount++
			}
			if added {
				addedCount++
			}
		}()
	}

	close(start)
	wg.Wait()

	if errCount != 0 {
		t.Fatalf("expected no errors, got %d", errCount)
	}
	if addedCount != 1 {
		t.Fatalf("expected exactly 1 successful PutIfAbsent, got %d", addedCount)
	}

	got, err := s.Get(ctx, "id-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got == nil || got.PayloadID != "id-1" {
		t.Fatalf("expected session to be present, got %+v", got)
	}
}

func TestDefensiveCopyOnGet(t *testing.T) {
	s := NewMemoryStore(10)
	ctx := context.Background()

	if _, err := s.PutIfAbsent(ctx, newTestSession("id-1")); err != nil {
		t.Fatalf("PutIfAbsent returned error: %v", err)
	}

	got, err := s.Get(ctx, "id-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	// Mutate the returned session and its slices.
	got.PayloadHash = sha256.Sum256([]byte("mutated"))
	got.Mappings[0].Token = "mutated-token"
	got.Mappings[0].Original = "mutated-original"

	again, err := s.Get(ctx, "id-1")
	if err != nil {
		t.Fatalf("second Get returned error: %v", err)
	}
	if again.PayloadHash != sha256.Sum256([]byte("id-1")) {
		t.Fatalf("stored session was mutated via Get, got PayloadHash=%x", again.PayloadHash)
	}
	if again.Mappings[0].Token != "tok1" || again.Mappings[0].Original != "orig1" {
		t.Fatalf("stored mappings were mutated via Get, got %+v", again.Mappings[0])
	}
}
