package admin

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresAuthStore struct {
	databaseURL string
	initOnce    sync.Once
	initErr     error
	schemaOnce  sync.Once
	schemaErr   error
	pool        *pgxpool.Pool
}

func NewPostgresAuthStore(databaseURL string) *PostgresAuthStore {
	return &PostgresAuthStore{databaseURL: databaseURL}
}

func (s *PostgresAuthStore) EnsureSchema(ctx context.Context) error {
	return s.ensureSchema(ctx)
}

func (s *PostgresAuthStore) CreateSession(ctx context.Context, session AuthSessionRecord) (AuthSessionRecord, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return AuthSessionRecord{}, err
	}

	row := s.pool.QueryRow(ctx, `
		insert into auth_sessions (
			id, user_id, token_hash, client_type, device_id, expires_at, revoked_at, last_seen_at, created_at
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		returning id, user_id, token_hash, client_type, device_id, expires_at, revoked_at, last_seen_at, created_at
	`, session.ID, session.UserID, session.TokenHash, session.ClientType, session.DeviceID, parseRequiredRFC3339(session.ExpiresAt), parseOptionalRFC3339(session.RevokedAt), parseRequiredRFC3339(session.LastSeenAt), parseRequiredRFC3339(session.CreatedAt))

	return scanAuthSession(row)
}

func (s *PostgresAuthStore) FindSessionByTokenHash(ctx context.Context, tokenHash string) (AuthSessionRecord, bool, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return AuthSessionRecord{}, false, err
	}

	row := s.pool.QueryRow(ctx, `
		select id, user_id, token_hash, client_type, device_id, expires_at, revoked_at, last_seen_at, created_at
		from auth_sessions
		where token_hash = $1
	`, tokenHash)

	session, err := scanAuthSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return AuthSessionRecord{}, false, nil
	}
	return session, err == nil, err
}

func (s *PostgresAuthStore) RevokeSessionByTokenHash(ctx context.Context, tokenHash string) error {
	if err := s.ensureSchema(ctx); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		update auth_sessions
		set revoked_at = now()
		where token_hash = $1 and revoked_at is null
	`, tokenHash)
	return err
}

func (s *PostgresAuthStore) TouchSession(ctx context.Context, sessionID string, lastSeenAt string) error {
	if err := s.ensureSchema(ctx); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		update auth_sessions
		set last_seen_at = $2
		where id = $1
	`, sessionID, parseRequiredRFC3339(lastSeenAt))
	return err
}

func (s *PostgresAuthStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *PostgresAuthStore) ensureSchema(ctx context.Context) error {
	if err := s.ensurePool(ctx); err != nil {
		return err
	}

	s.schemaOnce.Do(func() {
		_, s.schemaErr = s.pool.Exec(ctx, `
			create table if not exists auth_sessions (
				id text primary key,
				user_id text not null references admin_users(id) on delete cascade,
				token_hash text not null unique,
				client_type text not null default 'admin_web',
				device_id text not null default '',
				expires_at timestamptz not null,
				revoked_at timestamptz,
				last_seen_at timestamptz not null default now(),
				created_at timestamptz not null default now()
			);
		`)
	})

	return s.schemaErr
}

func (s *PostgresAuthStore) ensurePool(ctx context.Context) error {
	s.initOnce.Do(func() {
		if s.databaseURL == "" {
			s.initErr = errors.New("database url is required")
			return
		}

		s.pool, s.initErr = pgxpool.New(ctx, s.databaseURL)
	})

	return s.initErr
}

type pgxAuthSessionScanner interface {
	Scan(dest ...any) error
}

func scanAuthSession(row pgxAuthSessionScanner) (AuthSessionRecord, error) {
	var (
		session    AuthSessionRecord
		expiresAt  time.Time
		lastSeenAt time.Time
		createdAt  time.Time
		revokedAt  *time.Time
	)
	err := row.Scan(
		&session.ID,
		&session.UserID,
		&session.TokenHash,
		&session.ClientType,
		&session.DeviceID,
		&expiresAt,
		&revokedAt,
		&lastSeenAt,
		&createdAt,
	)
	if err != nil {
		return AuthSessionRecord{}, err
	}

	session.ExpiresAt = expiresAt.UTC().Format(time.RFC3339)
	session.LastSeenAt = lastSeenAt.UTC().Format(time.RFC3339)
	session.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	if revokedAt != nil {
		formatted := revokedAt.UTC().Format(time.RFC3339)
		session.RevokedAt = &formatted
	}
	return session, nil
}

func parseRequiredRFC3339(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Now().UTC()
	}
	return parsed
}
