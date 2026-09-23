package store

import (
	"context"
	"sync"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

type memoryStore struct {
	mu          sync.Mutex
	maxSessions int
	sessions    map[string]pii.Session
}

func NewMemoryStore(maxSessions int) Store {
	return &memoryStore{
		maxSessions: maxSessions,
		sessions:    make(map[string]pii.Session),
	}
}

func (s *memoryStore) Get(ctx context.Context, payloadID string) (*pii.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	session, ok := s.sessions[payloadID]
	if !ok {
		return nil, pii.ErrSessionNotFound
	}

	if isExpired(session, now) {
		delete(s.sessions, payloadID)
		return nil, pii.ErrSessionNotFound
	}

	copy := cloneSession(session)
	return &copy, nil
}

func (s *memoryStore) PutIfAbsent(ctx context.Context, session *pii.Session) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if session == nil {
		return false, pii.ErrStoreUnavailable
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	if existing, ok := s.sessions[session.PayloadID]; ok {
		if !isExpired(existing, now) {
			return false, nil
		}
		delete(s.sessions, session.PayloadID)
	}

	s.purgeExpired(now)

	if len(s.sessions) >= s.maxSessions {
		return false, pii.ErrStoreUnavailable
	}

	s.sessions[session.PayloadID] = cloneSession(*session)
	return true, nil
}

func (s *memoryStore) Update(ctx context.Context, session *pii.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if session == nil {
		return pii.ErrStoreUnavailable
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	existing, ok := s.sessions[session.PayloadID]
	if !ok {
		return pii.ErrSessionNotFound
	}

	if isExpired(existing, now) {
		delete(s.sessions, session.PayloadID)
		return pii.ErrSessionNotFound
	}

	s.sessions[session.PayloadID] = cloneSession(*session)
	return nil
}

func (s *memoryStore) Delete(ctx context.Context, payloadID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, payloadID)
	return nil
}

func (s *memoryStore) purgeExpired(now time.Time) {
	for id, session := range s.sessions {
		if isExpired(session, now) {
			delete(s.sessions, id)
		}
	}
}

func isExpired(session pii.Session, now time.Time) bool {
	return !session.ExpiresAt.IsZero() && !now.Before(session.ExpiresAt)
}

func cloneSession(session pii.Session) pii.Session {
	clone := session
	clone.Mappings = cloneMappings(session.Mappings)
	clone.MaskKinds = cloneMaskKinds(session.MaskKinds)
	return clone
}

func cloneMappings(mappings []pii.TokenMapping) []pii.TokenMapping {
	if mappings == nil {
		return nil
	}
	clone := make([]pii.TokenMapping, len(mappings))
	copy(clone, mappings)
	return clone
}

func cloneMaskKinds(kinds []pii.PIIKind) []pii.PIIKind {
	if kinds == nil {
		return nil
	}
	clone := make([]pii.PIIKind, len(kinds))
	copy(clone, kinds)
	return clone
}
