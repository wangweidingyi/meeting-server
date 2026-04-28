package transcripts

import (
	"context"
	"errors"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Snapshot struct {
	MeetingID string
	SegmentID string
	StartMS   uint64
	EndMS     uint64
	Text      string
	IsFinal   bool
	Revision  uint64
}

type Store interface {
	UpsertSnapshot(snapshot Snapshot) error
	LoadSnapshot(meetingID string) (Snapshot, bool, error)
	Close()
}

type MemoryStore struct {
	mu        sync.Mutex
	snapshots map[string]Snapshot
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		snapshots: make(map[string]Snapshot),
	}
}

func (s *MemoryStore) UpsertSnapshot(snapshot Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.snapshots[snapshot.MeetingID] = snapshot
	return nil
}

func (s *MemoryStore) LoadSnapshot(meetingID string) (Snapshot, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot, ok := s.snapshots[meetingID]
	return snapshot, ok, nil
}

func (s *MemoryStore) Close() {}

type PostgresStore struct {
	databaseURL string
	initOnce    sync.Once
	initErr     error
	schemaOnce  sync.Once
	schemaErr   error
	pool        *pgxpool.Pool
}

func NewPostgresStore(databaseURL string) *PostgresStore {
	return &PostgresStore{databaseURL: databaseURL}
}

func (s *PostgresStore) UpsertSnapshot(snapshot Snapshot) error {
	if err := s.ensureSchema(context.Background()); err != nil {
		return err
	}

	_, err := s.pool.Exec(context.Background(), `
		insert into meeting_transcripts (meeting_id, segment_id, start_ms, end_ms, text, is_final, revision, updated_at)
		values ($1, $2, $3, $4, $5, $6, $7, now())
		on conflict (meeting_id) do update
		set segment_id = excluded.segment_id,
			start_ms = excluded.start_ms,
			end_ms = excluded.end_ms,
			text = excluded.text,
			is_final = excluded.is_final,
			revision = excluded.revision,
			updated_at = now()
	`, snapshot.MeetingID, snapshot.SegmentID, int64(snapshot.StartMS), int64(snapshot.EndMS), snapshot.Text, snapshot.IsFinal, int64(snapshot.Revision))
	return err
}

func (s *PostgresStore) LoadSnapshot(meetingID string) (Snapshot, bool, error) {
	if err := s.ensureSchema(context.Background()); err != nil {
		return Snapshot{}, false, err
	}

	row := s.pool.QueryRow(context.Background(), `
		select meeting_id, segment_id, start_ms, end_ms, text, is_final, revision
		from meeting_transcripts
		where meeting_id = $1
	`, meetingID)

	var (
		snapshot Snapshot
		startMS  int64
		endMS    int64
		revision int64
	)
	if err := row.Scan(
		&snapshot.MeetingID,
		&snapshot.SegmentID,
		&startMS,
		&endMS,
		&snapshot.Text,
		&snapshot.IsFinal,
		&revision,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Snapshot{}, false, nil
		}
		return Snapshot{}, false, err
	}

	snapshot.StartMS = uint64(startMS)
	snapshot.EndMS = uint64(endMS)
	snapshot.Revision = uint64(revision)
	return snapshot, true, nil
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
			create table if not exists meeting_transcripts (
				meeting_id text primary key,
				segment_id text not null,
				start_ms bigint not null default 0,
				end_ms bigint not null default 0,
				text text not null default '',
				is_final boolean not null default false,
				revision bigint not null default 0,
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
