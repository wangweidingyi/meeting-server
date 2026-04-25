package admin

import (
	"context"
	"errors"
	"strings"
	"sync"
)

type MemoryUserStore struct {
	mu    sync.RWMutex
	users map[string]UserRecord
}

func NewMemoryUserStore() *MemoryUserStore {
	return &MemoryUserStore{
		users: make(map[string]UserRecord),
	}
}

func (s *MemoryUserStore) UpsertUser(_ context.Context, user UserRecord) (UserRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if user.CreatedAt == "" {
		if existing, ok := s.users[user.ID]; ok {
			user.CreatedAt = existing.CreatedAt
		} else {
			user.CreatedAt = nowRFC3339()
		}
	}
	user.UpdatedAt = nowRFC3339()
	s.users[user.ID] = cloneUserRecord(user)
	return cloneUserRecord(user), nil
}

func (s *MemoryUserStore) ListUsers(_ context.Context) ([]UserRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]UserRecord, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, cloneUserRecord(user))
	}

	return users, nil
}

func (s *MemoryUserStore) FindUserByID(_ context.Context, userID string) (UserRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[userID]
	if !ok {
		return UserRecord{}, false, nil
	}
	return cloneUserRecord(user), true, nil
}

func (s *MemoryUserStore) FindUserByUsername(_ context.Context, username string) (UserRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	username = strings.TrimSpace(strings.ToLower(username))
	for _, user := range s.users {
		if user.Username == username {
			return cloneUserRecord(user), true, nil
		}
	}
	return UserRecord{}, false, nil
}

func (s *MemoryUserStore) AdminExists(_ context.Context) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, user := range s.users {
		if user.Role == UserRoleAdmin {
			return true, nil
		}
	}
	return false, nil
}

func (s *MemoryUserStore) CreateInitialAdmin(_ context.Context, user UserRecord) (UserRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.users {
		if existing.Role == UserRoleAdmin {
			return UserRecord{}, errors.New("system already initialized")
		}
		if existing.Username == user.Username {
			return UserRecord{}, errors.New("user.username already exists")
		}
	}

	if user.CreatedAt == "" {
		user.CreatedAt = nowRFC3339()
	}
	user.UpdatedAt = nowRFC3339()
	s.users[user.ID] = cloneUserRecord(user)
	return cloneUserRecord(user), nil
}

func cloneUserRecord(user UserRecord) UserRecord {
	cloned := user
	if user.LastMeetingStarted != nil {
		lastMeetingStarted := *user.LastMeetingStarted
		cloned.LastMeetingStarted = &lastMeetingStarted
	}
	if user.LastLoginAt != nil {
		lastLoginAt := *user.LastLoginAt
		cloned.LastLoginAt = &lastLoginAt
	}
	if user.PasswordChangedAt != nil {
		passwordChangedAt := *user.PasswordChangedAt
		cloned.PasswordChangedAt = &passwordChangedAt
	}
	return cloned
}
