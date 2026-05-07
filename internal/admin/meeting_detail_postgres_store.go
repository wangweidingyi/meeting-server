package admin

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresMeetingDetailStore struct {
	databaseURL string
	initOnce    sync.Once
	initErr     error
	schemaOnce  sync.Once
	schemaErr   error
	pool        *pgxpool.Pool
}

func NewPostgresMeetingDetailStore(databaseURL string) *PostgresMeetingDetailStore {
	return &PostgresMeetingDetailStore{databaseURL: databaseURL}
}

func (s *PostgresMeetingDetailStore) UpsertTranscriptSegment(ctx context.Context, segment TranscriptSegmentRecord) (TranscriptSegmentRecord, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return TranscriptSegmentRecord{}, err
	}

	row := s.pool.QueryRow(ctx, `
		insert into meeting_transcript_segments (
			meeting_id, segment_id, start_ms, end_ms, text, is_final, speaker_id, revision, updated_at
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, now())
		on conflict (meeting_id, segment_id) do update
		set start_ms = excluded.start_ms,
			end_ms = excluded.end_ms,
			text = excluded.text,
			is_final = excluded.is_final,
			speaker_id = excluded.speaker_id,
			revision = excluded.revision,
			updated_at = now()
		returning meeting_id, segment_id, start_ms, end_ms, text, is_final, speaker_id, revision
	`, segment.MeetingID, segment.SegmentID, int64(segment.StartMS), int64(segment.EndMS), segment.Text, segment.IsFinal, segment.SpeakerID, int64(segment.Revision))

	return scanTranscriptSegmentRow(row)
}

func (s *PostgresMeetingDetailStore) ListTranscriptSegmentsByMeeting(ctx context.Context, meetingID string) ([]TranscriptSegmentRecord, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		select meeting_id, segment_id, start_ms, end_ms, text, is_final, speaker_id, revision
		from meeting_transcript_segments
		where meeting_id = $1
		order by start_ms asc, revision asc, segment_id asc
	`, meetingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	segments := make([]TranscriptSegmentRecord, 0)
	for rows.Next() {
		segment, err := scanTranscriptSegmentRow(rows)
		if err != nil {
			return nil, err
		}
		segments = append(segments, segment)
	}

	return segments, rows.Err()
}

func (s *PostgresMeetingDetailStore) UpsertSummarySnapshot(ctx context.Context, summary SummarySnapshotRecord) (SummarySnapshotRecord, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return SummarySnapshotRecord{}, err
	}

	row := s.pool.QueryRow(ctx, `
		insert into meeting_summary_snapshots (
			meeting_id, version, updated_at_label, abstract_text, key_points, decisions, risks, action_items, is_final, persisted_at
		)
		values ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7::jsonb, $8::jsonb, $9, now())
		on conflict (meeting_id) do update
		set version = excluded.version,
			updated_at_label = excluded.updated_at_label,
			abstract_text = excluded.abstract_text,
			key_points = excluded.key_points,
			decisions = excluded.decisions,
			risks = excluded.risks,
			action_items = excluded.action_items,
			is_final = excluded.is_final,
			persisted_at = now()
		returning meeting_id, version, updated_at_label, abstract_text, key_points, decisions, risks, action_items, is_final
	`, summary.MeetingID, int64(summary.Version), summary.UpdatedAt, summary.AbstractText, mustJSON(summary.KeyPoints), mustJSON(summary.Decisions), mustJSON(summary.Risks), mustJSON(summary.ActionItems), summary.IsFinal)

	return scanSummarySnapshotRow(row)
}

func (s *PostgresMeetingDetailStore) LatestSummarySnapshot(ctx context.Context, meetingID string) (*SummarySnapshotRecord, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return nil, err
	}

	row := s.pool.QueryRow(ctx, `
		select meeting_id, version, updated_at_label, abstract_text, key_points, decisions, risks, action_items, is_final
		from meeting_summary_snapshots
		where meeting_id = $1
	`, meetingID)

	summary, err := scanSummarySnapshotRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &summary, nil
}

func (s *PostgresMeetingDetailStore) ApplyActionItems(ctx context.Context, actionItems ActionItemsRecord) (*SummarySnapshotRecord, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return nil, err
	}

	current, err := s.LatestSummarySnapshot(ctx, actionItems.MeetingID)
	if err != nil {
		return nil, err
	}

	next := SummarySnapshotRecord{
		MeetingID:   actionItems.MeetingID,
		Version:     actionItems.Version,
		UpdatedAt:   actionItems.UpdatedAt,
		ActionItems: cloneStrings(actionItems.Items),
		IsFinal:     actionItems.IsFinal,
	}
	if current != nil {
		next = *current
		next.Version = maxUint64(current.Version, actionItems.Version)
		next.UpdatedAt = actionItems.UpdatedAt
		next.ActionItems = cloneStrings(actionItems.Items)
		next.IsFinal = current.IsFinal || actionItems.IsFinal
	}

	summary, err := s.UpsertSummarySnapshot(ctx, next)
	if err != nil {
		return nil, err
	}

	return &summary, nil
}

func (s *PostgresMeetingDetailStore) UpsertCheckpoint(ctx context.Context, checkpoint SessionCheckpointRecord) (SessionCheckpointRecord, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return SessionCheckpointRecord{}, err
	}

	row := s.pool.QueryRow(ctx, `
		insert into meeting_session_checkpoints (
			meeting_id,
			last_control_seq,
			last_udp_seq_sent,
			last_uploaded_mixed_ms,
			last_transcript_segment_revision,
			last_summary_version,
			last_action_item_version,
			local_recording_state,
			recovery_token,
			updated_at_label,
			persisted_at
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
		on conflict (meeting_id) do update
		set last_control_seq = excluded.last_control_seq,
			last_udp_seq_sent = excluded.last_udp_seq_sent,
			last_uploaded_mixed_ms = excluded.last_uploaded_mixed_ms,
			last_transcript_segment_revision = excluded.last_transcript_segment_revision,
			last_summary_version = excluded.last_summary_version,
			last_action_item_version = excluded.last_action_item_version,
			local_recording_state = excluded.local_recording_state,
			recovery_token = excluded.recovery_token,
			updated_at_label = excluded.updated_at_label,
			persisted_at = now()
		returning meeting_id, last_control_seq, last_udp_seq_sent, last_uploaded_mixed_ms, last_transcript_segment_revision, last_summary_version, last_action_item_version, local_recording_state, recovery_token, updated_at_label
	`, checkpoint.MeetingID, int64(checkpoint.LastControlSeq), int64(checkpoint.LastUDPSeqSent), int64(checkpoint.LastUploadedMixedMS), int64(checkpoint.LastTranscriptSegmentRevision), int64(checkpoint.LastSummaryVersion), int64(checkpoint.LastActionItemVersion), checkpoint.LocalRecordingState, checkpoint.RecoveryToken, checkpoint.UpdatedAt)

	return scanCheckpointRow(row)
}

func (s *PostgresMeetingDetailStore) FindCheckpoint(ctx context.Context, meetingID string) (SessionCheckpointRecord, bool, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return SessionCheckpointRecord{}, false, err
	}

	row := s.pool.QueryRow(ctx, `
		select meeting_id, last_control_seq, last_udp_seq_sent, last_uploaded_mixed_ms, last_transcript_segment_revision, last_summary_version, last_action_item_version, local_recording_state, recovery_token, updated_at_label
		from meeting_session_checkpoints
		where meeting_id = $1
	`, meetingID)

	checkpoint, err := scanCheckpointRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SessionCheckpointRecord{}, false, nil
		}
		return SessionCheckpointRecord{}, false, err
	}

	return checkpoint, true, nil
}

func (s *PostgresMeetingDetailStore) UpsertAudioAssets(ctx context.Context, assets AudioAssetRecord) (AudioAssetRecord, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return AudioAssetRecord{}, err
	}

	row := s.pool.QueryRow(ctx, `
		insert into meeting_audio_assets (
			meeting_id, mic_original_path, system_original_path, mixed_uplink_path, updated_at
		)
		values ($1, $2, $3, $4, now())
		on conflict (meeting_id) do update
		set mic_original_path = excluded.mic_original_path,
			system_original_path = excluded.system_original_path,
			mixed_uplink_path = excluded.mixed_uplink_path,
			updated_at = now()
		returning meeting_id, mic_original_path, system_original_path, mixed_uplink_path
	`, assets.MeetingID, assets.MicOriginalPath, assets.SystemOriginalPath, assets.MixedUplinkPath)

	return scanAudioAssetsRow(row)
}

func (s *PostgresMeetingDetailStore) FindAudioAssets(ctx context.Context, meetingID string) (AudioAssetRecord, bool, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return AudioAssetRecord{}, false, err
	}

	row := s.pool.QueryRow(ctx, `
		select meeting_id, mic_original_path, system_original_path, mixed_uplink_path
		from meeting_audio_assets
		where meeting_id = $1
	`, meetingID)

	assets, err := scanAudioAssetsRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AudioAssetRecord{}, false, nil
		}
		return AudioAssetRecord{}, false, err
	}

	return assets, true, nil
}

func (s *PostgresMeetingDetailStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *PostgresMeetingDetailStore) ensureSchema(ctx context.Context) error {
	if err := s.ensurePool(ctx); err != nil {
		return err
	}

	s.schemaOnce.Do(func() {
		_, s.schemaErr = s.pool.Exec(ctx, `
			create table if not exists meeting_transcript_segments (
				meeting_id text not null,
				segment_id text not null,
				start_ms bigint not null default 0,
				end_ms bigint not null default 0,
				text text not null default '',
				is_final boolean not null default false,
				speaker_id text,
				revision bigint not null default 0,
				updated_at timestamptz not null default now(),
				primary key (meeting_id, segment_id)
			);

			create table if not exists meeting_summary_snapshots (
				meeting_id text primary key,
				version bigint not null default 0,
				updated_at_label text not null default '',
				abstract_text text not null default '',
				key_points jsonb not null default '[]'::jsonb,
				decisions jsonb not null default '[]'::jsonb,
				risks jsonb not null default '[]'::jsonb,
				action_items jsonb not null default '[]'::jsonb,
				is_final boolean not null default false,
				persisted_at timestamptz not null default now()
			);

			create table if not exists meeting_session_checkpoints (
				meeting_id text primary key,
				last_control_seq bigint not null default 0,
				last_udp_seq_sent bigint not null default 0,
				last_uploaded_mixed_ms bigint not null default 0,
				last_transcript_segment_revision bigint not null default 0,
				last_summary_version bigint not null default 0,
				last_action_item_version bigint not null default 0,
				local_recording_state text not null default '',
				recovery_token text,
				updated_at_label text not null default '',
				persisted_at timestamptz not null default now()
			);

			create table if not exists meeting_audio_assets (
				meeting_id text primary key,
				mic_original_path text,
				system_original_path text,
				mixed_uplink_path text,
				updated_at timestamptz not null default now()
			);
		`)
	})

	return s.schemaErr
}

func (s *PostgresMeetingDetailStore) ensurePool(ctx context.Context) error {
	s.initOnce.Do(func() {
		if s.databaseURL == "" {
			s.initErr = errors.New("database url is required")
			return
		}

		s.pool, s.initErr = pgxpool.New(ctx, s.databaseURL)
	})

	return s.initErr
}

type pgxDetailScanner interface {
	Scan(dest ...any) error
}

func scanTranscriptSegmentRow(row pgxDetailScanner) (TranscriptSegmentRecord, error) {
	var (
		segment  TranscriptSegmentRecord
		startMS  int64
		endMS    int64
		revision int64
	)
	err := row.Scan(&segment.MeetingID, &segment.SegmentID, &startMS, &endMS, &segment.Text, &segment.IsFinal, &segment.SpeakerID, &revision)
	if err != nil {
		return TranscriptSegmentRecord{}, err
	}
	segment.StartMS = uint64(startMS)
	segment.EndMS = uint64(endMS)
	segment.Revision = uint64(revision)
	return segment, nil
}

func scanSummarySnapshotRow(row pgxDetailScanner) (SummarySnapshotRecord, error) {
	var (
		summary         SummarySnapshotRecord
		version         int64
		keyPointsJSON   []byte
		decisionsJSON   []byte
		risksJSON       []byte
		actionItemsJSON []byte
	)
	err := row.Scan(&summary.MeetingID, &version, &summary.UpdatedAt, &summary.AbstractText, &keyPointsJSON, &decisionsJSON, &risksJSON, &actionItemsJSON, &summary.IsFinal)
	if err != nil {
		return SummarySnapshotRecord{}, err
	}
	summary.Version = uint64(version)
	summary.KeyPoints = parseStringArrayJSON(keyPointsJSON)
	summary.Decisions = parseStringArrayJSON(decisionsJSON)
	summary.Risks = parseStringArrayJSON(risksJSON)
	summary.ActionItems = parseStringArrayJSON(actionItemsJSON)
	return summary, nil
}

func scanCheckpointRow(row pgxDetailScanner) (SessionCheckpointRecord, error) {
	var (
		checkpoint             SessionCheckpointRecord
		lastControlSeq         int64
		lastUDPSeqSent         int64
		lastUploadedMixedMS    int64
		lastTranscriptRevision int64
		lastSummaryVersion     int64
		lastActionItemVersion  int64
	)
	err := row.Scan(&checkpoint.MeetingID, &lastControlSeq, &lastUDPSeqSent, &lastUploadedMixedMS, &lastTranscriptRevision, &lastSummaryVersion, &lastActionItemVersion, &checkpoint.LocalRecordingState, &checkpoint.RecoveryToken, &checkpoint.UpdatedAt)
	if err != nil {
		return SessionCheckpointRecord{}, err
	}
	checkpoint.LastControlSeq = uint64(lastControlSeq)
	checkpoint.LastUDPSeqSent = uint64(lastUDPSeqSent)
	checkpoint.LastUploadedMixedMS = uint64(lastUploadedMixedMS)
	checkpoint.LastTranscriptSegmentRevision = uint64(lastTranscriptRevision)
	checkpoint.LastSummaryVersion = uint64(lastSummaryVersion)
	checkpoint.LastActionItemVersion = uint64(lastActionItemVersion)
	return checkpoint, nil
}

func scanAudioAssetsRow(row pgxDetailScanner) (AudioAssetRecord, error) {
	var assets AudioAssetRecord
	err := row.Scan(&assets.MeetingID, &assets.MicOriginalPath, &assets.SystemOriginalPath, &assets.MixedUplinkPath)
	return assets, err
}

func mustJSON(items []string) string {
	payload, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(payload)
}

func parseStringArrayJSON(payload []byte) []string {
	if len(payload) == 0 {
		return []string{}
	}

	var items []string
	if err := json.Unmarshal(payload, &items); err != nil {
		return []string{}
	}
	return items
}
