package admin

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresUserStore struct {
	databaseURL string
	initOnce    sync.Once
	initErr     error
	schemaOnce  sync.Once
	schemaErr   error
	pool        *pgxpool.Pool
}

func NewPostgresUserStore(databaseURL string) *PostgresUserStore {
	return &PostgresUserStore{
		databaseURL: databaseURL,
	}
}

func (s *PostgresUserStore) UpsertUser(ctx context.Context, user UserRecord) (UserRecord, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return UserRecord{}, err
	}

	row := s.pool.QueryRow(ctx, `
		insert into admin_users (
			id, username, display_name, role, status, password_hash, last_login_at, password_changed_at, updated_at
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, now())
		on conflict (id) do update
		set username = excluded.username,
			display_name = excluded.display_name,
			role = excluded.role,
			status = excluded.status,
			password_hash = excluded.password_hash,
			last_login_at = excluded.last_login_at,
			password_changed_at = excluded.password_changed_at,
			updated_at = now()
		returning id, username, display_name, role, status, password_hash, created_at, updated_at, last_login_at, password_changed_at
	`, user.ID, user.Username, user.DisplayName, user.Role, user.Status, user.PasswordHash, parseOptionalRFC3339(user.LastLoginAt), parseOptionalRFC3339(user.PasswordChangedAt))

	return scanUserRow(row)
}

func (s *PostgresUserStore) ListUsers(ctx context.Context) ([]UserRecord, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		select id, username, display_name, role, status, password_hash, created_at, updated_at, last_login_at, password_changed_at
		from admin_users
		order by updated_at desc, created_at desc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]UserRecord, 0)
	for rows.Next() {
		user, err := scanUserRow(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	return users, rows.Err()
}

func (s *PostgresUserStore) FindUserByID(ctx context.Context, userID string) (UserRecord, bool, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return UserRecord{}, false, err
	}

	row := s.pool.QueryRow(ctx, `
		select id, username, display_name, role, status, password_hash, created_at, updated_at, last_login_at, password_changed_at
		from admin_users
		where id = $1
	`, userID)

	user, err := scanUserRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserRecord{}, false, nil
	}
	return user, err == nil, err
}

func (s *PostgresUserStore) FindUserByUsername(ctx context.Context, username string) (UserRecord, bool, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return UserRecord{}, false, err
	}

	row := s.pool.QueryRow(ctx, `
		select id, username, display_name, role, status, password_hash, created_at, updated_at, last_login_at, password_changed_at
		from admin_users
		where username = $1
	`, username)

	user, err := scanUserRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserRecord{}, false, nil
	}
	return user, err == nil, err
}

func (s *PostgresUserStore) AdminExists(ctx context.Context) (bool, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return false, err
	}

	var exists bool
	err := s.pool.QueryRow(ctx, `
		select exists(
			select 1
			from admin_users
			where role = 'admin'
		)
	`).Scan(&exists)
	return exists, err
}

func (s *PostgresUserStore) CreateInitialAdmin(ctx context.Context, user UserRecord) (UserRecord, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return UserRecord{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return UserRecord{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock(982451653)`); err != nil {
		return UserRecord{}, err
	}

	var adminExists bool
	if err := tx.QueryRow(ctx, `
		select exists(
			select 1
			from admin_users
			where role = 'admin'
		)
	`).Scan(&adminExists); err != nil {
		return UserRecord{}, err
	}
	if adminExists {
		return UserRecord{}, errors.New("system already initialized")
	}

	row := tx.QueryRow(ctx, `
		insert into admin_users (
			id, username, display_name, role, status, password_hash, last_login_at, password_changed_at, updated_at
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, now())
		returning id, username, display_name, role, status, password_hash, created_at, updated_at, last_login_at, password_changed_at
	`, user.ID, user.Username, user.DisplayName, user.Role, user.Status, user.PasswordHash, parseOptionalRFC3339(user.LastLoginAt), parseOptionalRFC3339(user.PasswordChangedAt))

	createdUser, err := scanUserRow(row)
	if err != nil {
		return UserRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return UserRecord{}, err
	}

	return createdUser, nil
}

func (s *PostgresUserStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *PostgresUserStore) ensureSchema(ctx context.Context) error {
	if err := s.ensurePool(ctx); err != nil {
		return err
	}

	s.schemaOnce.Do(func() {
		_, s.schemaErr = s.pool.Exec(ctx, `
			create table if not exists admin_users (
				id text primary key,
				username text not null unique,
				display_name text not null default '',
				role text not null default 'member',
				status text not null default 'active',
				password_hash text not null default '',
				last_login_at timestamptz,
				password_changed_at timestamptz,
				created_at timestamptz not null default now(),
				updated_at timestamptz not null default now()
			);

			alter table admin_users add column if not exists role text not null default 'member';
			alter table admin_users add column if not exists status text not null default 'active';
			alter table admin_users add column if not exists password_hash text not null default '';
			alter table admin_users add column if not exists last_login_at timestamptz;
			alter table admin_users add column if not exists password_changed_at timestamptz;
		`)
	})

	return s.schemaErr
}

func (s *PostgresUserStore) ensurePool(ctx context.Context) error {
	s.initOnce.Do(func() {
		if s.databaseURL == "" {
			s.initErr = errors.New("database url is required")
			return
		}

		s.pool, s.initErr = pgxpool.New(ctx, s.databaseURL)
	})

	return s.initErr
}

type pgxUserScanner interface {
	Scan(dest ...any) error
}

func scanUserRow(row pgxUserScanner) (UserRecord, error) {
	var (
		user              UserRecord
		createdAt         time.Time
		updatedAt         time.Time
		lastLoginAt       *time.Time
		passwordChangedAt *time.Time
	)
	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.DisplayName,
		&user.Role,
		&user.Status,
		&user.PasswordHash,
		&createdAt,
		&updatedAt,
		&lastLoginAt,
		&passwordChangedAt,
	)
	if err != nil {
		return UserRecord{}, err
	}

	user.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	user.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	if lastLoginAt != nil {
		formatted := lastLoginAt.UTC().Format(time.RFC3339)
		user.LastLoginAt = &formatted
	}
	if passwordChangedAt != nil {
		formatted := passwordChangedAt.UTC().Format(time.RFC3339)
		user.PasswordChangedAt = &formatted
	}
	return user, nil
}

func parseOptionalRFC3339(value *string) any {
	if value == nil || *value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return nil
	}
	return parsed
}
