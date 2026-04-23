package admin

import (
	"context"
	"sync"
	"time"
)

type MemoryStore struct {
	mu       sync.RWMutex
	settings Settings
	hasValue bool
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func (s *MemoryStore) Load(_ context.Context) (Settings, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.hasValue {
		return Settings{}, false, nil
	}

	return cloneSettings(s.settings), true, nil
}

func (s *MemoryStore) Save(_ context.Context, settings Settings) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	version := s.settings.Version + 1
	if !s.hasValue {
		version = 1
	}

	s.settings = cloneSettings(Settings{
		Version:   version,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		AI:        settings.AI,
	})
	s.hasValue = true

	return cloneSettings(s.settings), nil
}
