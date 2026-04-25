package admin

import (
	"context"
	"sync"
)

type MemoryAuthStore struct {
	mu       sync.RWMutex
	sessions map[string]AuthSessionRecord
}

func NewMemoryAuthStore() *MemoryAuthStore {
	return &MemoryAuthStore{
		sessions: make(map[string]AuthSessionRecord),
	}
}

func (s *MemoryAuthStore) EnsureSchema(_ context.Context) error {
	return nil
}

func (s *MemoryAuthStore) CreateSession(_ context.Context, session AuthSessionRecord) (AuthSessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.TokenHash] = cloneAuthSession(session)
	return cloneAuthSession(session), nil
}

func (s *MemoryAuthStore) FindSessionByTokenHash(_ context.Context, tokenHash string) (AuthSessionRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[tokenHash]
	if !ok {
		return AuthSessionRecord{}, false, nil
	}
	return cloneAuthSession(session), true, nil
}

func (s *MemoryAuthStore) RevokeSessionByTokenHash(_ context.Context, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[tokenHash]
	if !ok {
		return nil
	}
	now := nowRFC3339()
	session.RevokedAt = &now
	s.sessions[tokenHash] = session
	return nil
}

func (s *MemoryAuthStore) TouchSession(_ context.Context, sessionID string, lastSeenAt string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for tokenHash, session := range s.sessions {
		if session.ID != sessionID {
			continue
		}
		session.LastSeenAt = lastSeenAt
		s.sessions[tokenHash] = session
		return nil
	}
	return nil
}

func cloneAuthSession(session AuthSessionRecord) AuthSessionRecord {
	cloned := session
	if session.RevokedAt != nil {
		revokedAt := *session.RevokedAt
		cloned.RevokedAt = &revokedAt
	}
	return cloned
}
