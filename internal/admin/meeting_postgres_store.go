package admin

import (
	"context"
	"errors"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresMeetingStore struct {
	databaseURL string
	initOnce    sync.Once
	initErr     error
	schemaOnce  sync.Once
	schemaErr   error
	pool        *pgxpool.Pool
}

func NewPostgresMeetingStore(databaseURL string) *PostgresMeetingStore {
	return &PostgresMeetingStore{
		databaseURL: databaseURL,
	}
}

func (s *PostgresMeetingStore) UpsertMeeting(ctx context.Context, meeting MeetingRecord) (MeetingRecord, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return MeetingRecord{}, err
	}

	row := s.pool.QueryRow(ctx, `
		insert into meetings (id, user_id, user_name, client_id, title, status, started_at, ended_at, duration_ms, updated_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		on conflict (id) do update
		set user_id = excluded.user_id,
			user_name = excluded.user_name,
			client_id = excluded.client_id,
			title = excluded.title,
			status = excluded.status,
			started_at = excluded.started_at,
			ended_at = excluded.ended_at,
			duration_ms = excluded.duration_ms,
			updated_at = now()
		returning id, user_id, user_name, client_id, title, status, started_at, ended_at, duration_ms
	`, meeting.ID, meeting.UserID, meeting.UserName, meeting.ClientID, meeting.Title, meeting.Status, meeting.StartedAt, meeting.EndedAt, meeting.DurationMS)

	return scanMeetingRow(row)
}

func (s *PostgresMeetingStore) ListMeetings(ctx context.Context) ([]MeetingRecord, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		select id, user_id, user_name, client_id, title, status, started_at, ended_at, duration_ms
		from meetings
		order by started_at desc, updated_at desc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	meetings := make([]MeetingRecord, 0)
	for rows.Next() {
		meeting, err := scanMeetingRow(rows)
		if err != nil {
			return nil, err
		}
		meetings = append(meetings, meeting)
	}

	return meetings, rows.Err()
}

func (s *PostgresMeetingStore) ListMeetingsByUser(ctx context.Context, userID string) ([]MeetingRecord, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		select id, user_id, user_name, client_id, title, status, started_at, ended_at, duration_ms
		from meetings
		where user_id = $1
		order by started_at desc, updated_at desc
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	meetings := make([]MeetingRecord, 0)
	for rows.Next() {
		meeting, err := scanMeetingRow(rows)
		if err != nil {
			return nil, err
		}
		meetings = append(meetings, meeting)
	}

	return meetings, rows.Err()
}

func (s *PostgresMeetingStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *PostgresMeetingStore) ensureSchema(ctx context.Context) error {
	if err := s.ensurePool(ctx); err != nil {
		return err
	}

	s.schemaOnce.Do(func() {
		_, s.schemaErr = s.pool.Exec(ctx, `
			create table if not exists meetings (
				id text primary key,
				user_id text,
				user_name text not null default '',
				client_id text not null,
				title text not null,
				status text not null,
				started_at text not null,
				ended_at text,
				duration_ms bigint not null default 0,
				updated_at timestamptz not null default now()
			);

			alter table meetings add column if not exists user_id text;
			alter table meetings add column if not exists user_name text not null default '';
			update meetings
			set user_id = client_id
			where user_id is null or btrim(user_id) = '';
			update meetings
			set user_name = user_id
			where btrim(user_name) = '';
		`)
	})

	return s.schemaErr
}

func (s *PostgresMeetingStore) ensurePool(ctx context.Context) error {
	s.initOnce.Do(func() {
		if s.databaseURL == "" {
			s.initErr = errors.New("database url is required")
			return
		}

		s.pool, s.initErr = pgxpool.New(ctx, s.databaseURL)
	})

	return s.initErr
}

type pgxMeetingScanner interface {
	Scan(dest ...any) error
}

func scanMeetingRow(row pgxMeetingScanner) (MeetingRecord, error) {
	var meeting MeetingRecord
	err := row.Scan(
		&meeting.ID,
		&meeting.UserID,
		&meeting.UserName,
		&meeting.ClientID,
		&meeting.Title,
		&meeting.Status,
		&meeting.StartedAt,
		&meeting.EndedAt,
		&meeting.DurationMS,
	)
	return meeting, err
}
