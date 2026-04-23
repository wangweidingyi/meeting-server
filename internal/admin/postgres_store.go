package admin

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"meeting-server/internal/config"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const runtimeSettingsKey = "runtime_ai"

type PostgresStore struct {
	databaseURL string
	initOnce    sync.Once
	initErr     error
	schemaOnce  sync.Once
	schemaErr   error
	pool        *pgxpool.Pool
}

func NewPostgresStore(databaseURL string) *PostgresStore {
	return &PostgresStore{
		databaseURL: databaseURL,
	}
}

func (s *PostgresStore) Load(ctx context.Context) (Settings, bool, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return Settings{}, false, err
	}

	row := s.pool.QueryRow(ctx, `
		select version, updated_at, data
		from admin_settings
		where name = $1
	`, runtimeSettingsKey)

	var (
		version   int64
		updatedAt time.Time
		data      []byte
	)
	if err := row.Scan(&version, &updatedAt, &data); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Settings{}, false, nil
		}
		return Settings{}, false, err
	}

	decoded, _, err := decodeStoredSettings(version, updatedAt, data)
	return decoded, true, err
}

func (s *PostgresStore) Save(ctx context.Context, settings Settings) (Settings, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return Settings{}, err
	}

	payload, err := json.Marshal(settings.AI)
	if err != nil {
		return Settings{}, err
	}

	row := s.pool.QueryRow(ctx, `
		insert into admin_settings (name, version, data, updated_at)
		values ($1, 1, $2::jsonb, now())
		on conflict (name) do update
		set version = admin_settings.version + 1,
			data = excluded.data,
			updated_at = now()
		returning version, updated_at, data
	`, runtimeSettingsKey, payload)

	var (
		version   int64
		updatedAt time.Time
		data      []byte
	)
	if err := row.Scan(&version, &updatedAt, &data); err != nil {
		return Settings{}, err
	}

	decoded, _, err := decodeStoredSettings(version, updatedAt, data)
	return decoded, err
}

func (s *PostgresStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *PostgresStore) ensureSchema(ctx context.Context) error {
	if err := s.ensurePool(ctx); err != nil {
		return err
	}

	s.schemaOnce.Do(func() {
		_, s.schemaErr = s.pool.Exec(ctx, `
			create table if not exists admin_settings (
				name text primary key,
				version bigint not null,
				data jsonb not null,
				updated_at timestamptz not null default now()
			);

			create table if not exists admin_users (
				id text primary key,
				username text not null unique,
				display_name text not null default '',
				role text not null default 'viewer',
				created_at timestamptz not null default now(),
				updated_at timestamptz not null default now()
			);
		`)
	})

	return s.schemaErr
}

func (s *PostgresStore) ensurePool(ctx context.Context) error {
	s.initOnce.Do(func() {
		if s.databaseURL == "" {
			s.initErr = errors.New("database url is required")
			return
		}

		s.pool, s.initErr = pgxpool.New(ctx, s.databaseURL)
	})

	return s.initErr
}

func decodeStoredSettings(version int64, updatedAt time.Time, data []byte) (Settings, bool, error) {
	var aiSettings config.AIConfig
	if err := json.Unmarshal(data, &aiSettings); err != nil {
		return Settings{}, false, err
	}

	return Settings{
		Version:   version,
		UpdatedAt: updatedAt.UTC().Format(time.RFC3339),
		AI:        aiSettings,
	}, true, nil
}
