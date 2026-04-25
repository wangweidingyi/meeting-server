package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const (
	ClientTypeAdminWeb = "admin_web"
	ClientTypeDesktop  = "desktop"
)

type AuthSessionRecord struct {
	ID         string
	UserID     string
	TokenHash  string
	ClientType string
	DeviceID   string
	ExpiresAt  string
	RevokedAt  *string
	LastSeenAt string
	CreatedAt  string
}

type AuthUser struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Status      string `json:"status"`
}

type LoginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	ClientType string `json:"client_type"`
	DeviceID   string `json:"device_id"`
}

type AuthSessionResponse struct {
	Token     string   `json:"token"`
	ExpiresAt string   `json:"expires_at"`
	User      AuthUser `json:"user"`
}

type AuthStore interface {
	EnsureSchema(ctx context.Context) error
	CreateSession(ctx context.Context, session AuthSessionRecord) (AuthSessionRecord, error)
	FindSessionByTokenHash(ctx context.Context, tokenHash string) (AuthSessionRecord, bool, error)
	RevokeSessionByTokenHash(ctx context.Context, tokenHash string) error
	TouchSession(ctx context.Context, sessionID string, lastSeenAt string) error
}

type AuthContext struct {
	User    UserRecord
	Session AuthSessionRecord
}

type AuthService struct {
	users    *UserService
	sessions AuthStore
	now      func() time.Time
}

func NewAuthService(users *UserService, sessions AuthStore) *AuthService {
	if users == nil {
		panic("user service is required")
	}
	if sessions == nil {
		panic("auth session store is required")
	}
	return &AuthService{
		users:    users,
		sessions: sessions,
		now:      time.Now,
	}
}

func (s *AuthService) EnsureReady(ctx context.Context) error {
	return s.sessions.EnsureSchema(ctx)
}

func (s *AuthService) Login(ctx context.Context, request LoginRequest) (AuthSessionResponse, error) {
	request.Username = strings.TrimSpace(request.Username)
	request.Password = strings.TrimSpace(request.Password)
	request.ClientType = normalizeClientType(request.ClientType)
	request.DeviceID = strings.TrimSpace(request.DeviceID)
	if request.Username == "" || request.Password == "" {
		return AuthSessionResponse{}, errors.New("username and password are required")
	}

	user, err := s.users.Authenticate(ctx, request.Username, request.Password)
	if err != nil {
		return AuthSessionResponse{}, err
	}

	token, err := generateOpaqueToken()
	if err != nil {
		return AuthSessionResponse{}, err
	}

	now := s.now().UTC()
	expiresAt := now.Add(30 * 24 * time.Hour)
	session, err := s.sessions.CreateSession(ctx, AuthSessionRecord{
		ID:         "session-" + token[:16],
		UserID:     user.ID,
		TokenHash:  hashToken(token),
		ClientType: request.ClientType,
		DeviceID:   request.DeviceID,
		ExpiresAt:  expiresAt.Format(time.RFC3339),
		LastSeenAt: now.Format(time.RFC3339),
		CreatedAt:  now.Format(time.RFC3339),
	})
	if err != nil {
		return AuthSessionResponse{}, err
	}

	if err := s.users.TouchLastLogin(ctx, user.ID); err != nil {
		return AuthSessionResponse{}, err
	}

	return AuthSessionResponse{
		Token:     token,
		ExpiresAt: session.ExpiresAt,
		User:      authUserFromRecord(user),
	}, nil
}

func (s *AuthService) AuthenticateToken(ctx context.Context, token string) (AuthContext, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return AuthContext{}, errors.New("missing bearer token")
	}

	session, found, err := s.sessions.FindSessionByTokenHash(ctx, hashToken(token))
	if err != nil {
		return AuthContext{}, err
	}
	if !found {
		return AuthContext{}, errors.New("invalid session")
	}
	if session.RevokedAt != nil {
		return AuthContext{}, errors.New("session revoked")
	}
	if expiresAt, err := time.Parse(time.RFC3339, session.ExpiresAt); err == nil && expiresAt.Before(s.now().UTC()) {
		return AuthContext{}, errors.New("session expired")
	}

	user, found, err := s.users.FindByID(ctx, session.UserID)
	if err != nil {
		return AuthContext{}, err
	}
	if !found {
		return AuthContext{}, errors.New("user not found")
	}
	if user.Status != UserStatusActive {
		return AuthContext{}, errors.New("user is disabled")
	}

	_ = s.sessions.TouchSession(ctx, session.ID, s.now().UTC().Format(time.RFC3339))
	return AuthContext{User: user, Session: session}, nil
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	return s.sessions.RevokeSessionByTokenHash(ctx, hashToken(token))
}

func authUserFromRecord(user UserRecord) AuthUser {
	return AuthUser{
		ID:          user.ID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Role:        user.Role,
		Status:      user.Status,
	}
}

func normalizeClientType(clientType string) string {
	clientType = strings.TrimSpace(clientType)
	switch clientType {
	case ClientTypeDesktop:
		return ClientTypeDesktop
	default:
		return ClientTypeAdminWeb
	}
}

func generateOpaqueToken() (string, error) {
	var buffer [32]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer[:]), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
