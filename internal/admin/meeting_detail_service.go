package admin

import (
	"context"
	"errors"
	"sort"
	"strings"
)

type TranscriptSegmentRecord struct {
	SegmentID string  `json:"segment_id"`
	MeetingID string  `json:"meeting_id"`
	StartMS   uint64  `json:"start_ms"`
	EndMS     uint64  `json:"end_ms"`
	Text      string  `json:"text"`
	IsFinal   bool    `json:"is_final"`
	SpeakerID *string `json:"speaker_id"`
	Revision  uint64  `json:"revision"`
}

type SummarySnapshotRecord struct {
	MeetingID    string   `json:"meeting_id"`
	Version      uint64   `json:"version"`
	UpdatedAt    string   `json:"updated_at"`
	AbstractText string   `json:"abstract_text"`
	KeyPoints    []string `json:"key_points"`
	Decisions    []string `json:"decisions"`
	Risks        []string `json:"risks"`
	ActionItems  []string `json:"action_items"`
	IsFinal      bool     `json:"is_final"`
}

type ActionItemsRecord struct {
	MeetingID string   `json:"meeting_id"`
	Version   uint64   `json:"version"`
	UpdatedAt string   `json:"updated_at"`
	Items     []string `json:"items"`
	IsFinal   bool     `json:"is_final"`
}

type SessionCheckpointRecord struct {
	MeetingID                     string  `json:"meeting_id"`
	LastControlSeq                uint64  `json:"last_control_seq"`
	LastUDPSeqSent                uint64  `json:"last_udp_seq_sent"`
	LastUploadedMixedMS           uint64  `json:"last_uploaded_mixed_ms"`
	LastTranscriptSegmentRevision uint64  `json:"last_transcript_segment_revision"`
	LastSummaryVersion            uint64  `json:"last_summary_version"`
	LastActionItemVersion         uint64  `json:"last_action_item_version"`
	LocalRecordingState           string  `json:"local_recording_state"`
	RecoveryToken                 *string `json:"recovery_token"`
	UpdatedAt                     string  `json:"updated_at"`
}

type AudioAssetRecord struct {
	MeetingID          string  `json:"meeting_id"`
	MicOriginalPath    *string `json:"mic_original_path"`
	SystemOriginalPath *string `json:"system_original_path"`
	MixedUplinkPath    *string `json:"mixed_uplink_path"`
}

type MeetingDetailResponse struct {
	Meeting            MeetingRecord             `json:"meeting"`
	TranscriptSegments []TranscriptSegmentRecord `json:"transcript_segments"`
	Summary            *SummarySnapshotRecord    `json:"summary"`
	ActionItems        []string                  `json:"action_items"`
}

type MeetingDetailStore interface {
	UpsertTranscriptSegment(ctx context.Context, segment TranscriptSegmentRecord) (TranscriptSegmentRecord, error)
	ListTranscriptSegmentsByMeeting(ctx context.Context, meetingID string) ([]TranscriptSegmentRecord, error)
	UpsertSummarySnapshot(ctx context.Context, summary SummarySnapshotRecord) (SummarySnapshotRecord, error)
	LatestSummarySnapshot(ctx context.Context, meetingID string) (*SummarySnapshotRecord, error)
	ApplyActionItems(ctx context.Context, actionItems ActionItemsRecord) (*SummarySnapshotRecord, error)
	UpsertCheckpoint(ctx context.Context, checkpoint SessionCheckpointRecord) (SessionCheckpointRecord, error)
	FindCheckpoint(ctx context.Context, meetingID string) (SessionCheckpointRecord, bool, error)
	UpsertAudioAssets(ctx context.Context, assets AudioAssetRecord) (AudioAssetRecord, error)
	FindAudioAssets(ctx context.Context, meetingID string) (AudioAssetRecord, bool, error)
}

type MeetingDetailService struct {
	store MeetingDetailStore
}

func NewMeetingDetailService(store MeetingDetailStore) *MeetingDetailService {
	if store == nil {
		panic("meeting detail store is required")
	}

	return &MeetingDetailService{store: store}
}

func (s *MeetingDetailService) UpsertTranscriptSegment(ctx context.Context, segment TranscriptSegmentRecord) (TranscriptSegmentRecord, error) {
	if err := validateTranscriptSegmentRecord(segment); err != nil {
		return TranscriptSegmentRecord{}, err
	}

	return s.store.UpsertTranscriptSegment(ctx, normalizeTranscriptSegmentRecord(segment))
}

func (s *MeetingDetailService) ListTranscriptSegmentsByMeeting(ctx context.Context, meetingID string) ([]TranscriptSegmentRecord, error) {
	meetingID = strings.TrimSpace(meetingID)
	if meetingID == "" {
		return nil, errors.New("meeting_id is required")
	}

	segments, err := s.store.ListTranscriptSegmentsByMeeting(ctx, meetingID)
	if err != nil {
		return nil, err
	}

	sort.Slice(segments, func(left, right int) bool {
		if segments[left].StartMS == segments[right].StartMS {
			if segments[left].Revision == segments[right].Revision {
				return segments[left].SegmentID < segments[right].SegmentID
			}
			return segments[left].Revision < segments[right].Revision
		}
		return segments[left].StartMS < segments[right].StartMS
	})

	return segments, nil
}

func (s *MeetingDetailService) UpsertSummarySnapshot(ctx context.Context, summary SummarySnapshotRecord) (SummarySnapshotRecord, error) {
	if err := validateSummarySnapshotRecord(summary); err != nil {
		return SummarySnapshotRecord{}, err
	}

	return s.store.UpsertSummarySnapshot(ctx, normalizeSummarySnapshotRecord(summary))
}

func (s *MeetingDetailService) LatestSummarySnapshot(ctx context.Context, meetingID string) (*SummarySnapshotRecord, error) {
	meetingID = strings.TrimSpace(meetingID)
	if meetingID == "" {
		return nil, errors.New("meeting_id is required")
	}

	return s.store.LatestSummarySnapshot(ctx, meetingID)
}

func (s *MeetingDetailService) ApplyActionItems(ctx context.Context, actionItems ActionItemsRecord) (*SummarySnapshotRecord, error) {
	if err := validateActionItemsRecord(actionItems); err != nil {
		return nil, err
	}

	return s.store.ApplyActionItems(ctx, normalizeActionItemsRecord(actionItems))
}

func (s *MeetingDetailService) UpsertCheckpoint(ctx context.Context, checkpoint SessionCheckpointRecord) (SessionCheckpointRecord, error) {
	if err := validateSessionCheckpointRecord(checkpoint); err != nil {
		return SessionCheckpointRecord{}, err
	}

	return s.store.UpsertCheckpoint(ctx, normalizeSessionCheckpointRecord(checkpoint))
}

func (s *MeetingDetailService) FindCheckpoint(ctx context.Context, meetingID string) (SessionCheckpointRecord, bool, error) {
	meetingID = strings.TrimSpace(meetingID)
	if meetingID == "" {
		return SessionCheckpointRecord{}, false, errors.New("meeting_id is required")
	}

	return s.store.FindCheckpoint(ctx, meetingID)
}

func (s *MeetingDetailService) UpsertAudioAssets(ctx context.Context, assets AudioAssetRecord) (AudioAssetRecord, error) {
	if err := validateAudioAssetRecord(assets); err != nil {
		return AudioAssetRecord{}, err
	}

	return s.store.UpsertAudioAssets(ctx, normalizeAudioAssetRecord(assets))
}

func (s *MeetingDetailService) FindAudioAssets(ctx context.Context, meetingID string) (AudioAssetRecord, bool, error) {
	meetingID = strings.TrimSpace(meetingID)
	if meetingID == "" {
		return AudioAssetRecord{}, false, errors.New("meeting_id is required")
	}

	return s.store.FindAudioAssets(ctx, meetingID)
}

func validateTranscriptSegmentRecord(segment TranscriptSegmentRecord) error {
	if strings.TrimSpace(segment.MeetingID) == "" {
		return errors.New("transcript_segment.meeting_id is required")
	}
	if strings.TrimSpace(segment.SegmentID) == "" {
		return errors.New("transcript_segment.segment_id is required")
	}
	return nil
}

func normalizeTranscriptSegmentRecord(segment TranscriptSegmentRecord) TranscriptSegmentRecord {
	normalized := segment
	normalized.MeetingID = strings.TrimSpace(normalized.MeetingID)
	normalized.SegmentID = strings.TrimSpace(normalized.SegmentID)
	normalized.Text = strings.TrimSpace(normalized.Text)
	if normalized.SpeakerID != nil {
		speakerID := strings.TrimSpace(*normalized.SpeakerID)
		if speakerID == "" {
			normalized.SpeakerID = nil
		} else {
			normalized.SpeakerID = &speakerID
		}
	}
	return normalized
}

func validateSummarySnapshotRecord(summary SummarySnapshotRecord) error {
	if strings.TrimSpace(summary.MeetingID) == "" {
		return errors.New("summary.meeting_id is required")
	}
	if strings.TrimSpace(summary.UpdatedAt) == "" {
		return errors.New("summary.updated_at is required")
	}
	return nil
}

func normalizeSummarySnapshotRecord(summary SummarySnapshotRecord) SummarySnapshotRecord {
	normalized := summary
	normalized.MeetingID = strings.TrimSpace(normalized.MeetingID)
	normalized.UpdatedAt = strings.TrimSpace(normalized.UpdatedAt)
	normalized.AbstractText = strings.TrimSpace(normalized.AbstractText)
	normalized.KeyPoints = cloneStrings(normalized.KeyPoints)
	normalized.Decisions = cloneStrings(normalized.Decisions)
	normalized.Risks = cloneStrings(normalized.Risks)
	normalized.ActionItems = cloneStrings(normalized.ActionItems)
	return normalized
}

func validateActionItemsRecord(actionItems ActionItemsRecord) error {
	if strings.TrimSpace(actionItems.MeetingID) == "" {
		return errors.New("action_items.meeting_id is required")
	}
	if strings.TrimSpace(actionItems.UpdatedAt) == "" {
		return errors.New("action_items.updated_at is required")
	}
	return nil
}

func normalizeActionItemsRecord(actionItems ActionItemsRecord) ActionItemsRecord {
	normalized := actionItems
	normalized.MeetingID = strings.TrimSpace(normalized.MeetingID)
	normalized.UpdatedAt = strings.TrimSpace(normalized.UpdatedAt)
	normalized.Items = cloneStrings(normalized.Items)
	return normalized
}

func validateSessionCheckpointRecord(checkpoint SessionCheckpointRecord) error {
	if strings.TrimSpace(checkpoint.MeetingID) == "" {
		return errors.New("checkpoint.meeting_id is required")
	}
	if strings.TrimSpace(checkpoint.LocalRecordingState) == "" {
		return errors.New("checkpoint.local_recording_state is required")
	}
	if strings.TrimSpace(checkpoint.UpdatedAt) == "" {
		return errors.New("checkpoint.updated_at is required")
	}
	return nil
}

func normalizeSessionCheckpointRecord(checkpoint SessionCheckpointRecord) SessionCheckpointRecord {
	normalized := checkpoint
	normalized.MeetingID = strings.TrimSpace(normalized.MeetingID)
	normalized.LocalRecordingState = strings.TrimSpace(normalized.LocalRecordingState)
	normalized.UpdatedAt = strings.TrimSpace(normalized.UpdatedAt)
	if normalized.RecoveryToken != nil {
		token := strings.TrimSpace(*normalized.RecoveryToken)
		if token == "" {
			normalized.RecoveryToken = nil
		} else {
			normalized.RecoveryToken = &token
		}
	}
	return normalized
}

func validateAudioAssetRecord(assets AudioAssetRecord) error {
	if strings.TrimSpace(assets.MeetingID) == "" {
		return errors.New("audio_assets.meeting_id is required")
	}
	return nil
}

func normalizeAudioAssetRecord(assets AudioAssetRecord) AudioAssetRecord {
	normalized := assets
	normalized.MeetingID = strings.TrimSpace(normalized.MeetingID)
	normalized.MicOriginalPath = normalizeOptionalString(assets.MicOriginalPath)
	normalized.SystemOriginalPath = normalizeOptionalString(assets.SystemOriginalPath)
	normalized.MixedUplinkPath = normalizeOptionalString(assets.MixedUplinkPath)
	return normalized
}

func cloneStrings(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}

	cloned := make([]string, 0, len(items))
	for _, item := range items {
		cloned = append(cloned, strings.TrimSpace(item))
	}
	return cloned
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}
