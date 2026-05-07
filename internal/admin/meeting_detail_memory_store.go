package admin

import "context"

type MemoryMeetingDetailStore struct {
	segments    map[string]map[string]TranscriptSegmentRecord
	summaries   map[string]SummarySnapshotRecord
	checkpoints map[string]SessionCheckpointRecord
	audioAssets map[string]AudioAssetRecord
}

func NewMemoryMeetingDetailStore() *MemoryMeetingDetailStore {
	return &MemoryMeetingDetailStore{
		segments:    make(map[string]map[string]TranscriptSegmentRecord),
		summaries:   make(map[string]SummarySnapshotRecord),
		checkpoints: make(map[string]SessionCheckpointRecord),
		audioAssets: make(map[string]AudioAssetRecord),
	}
}

func (s *MemoryMeetingDetailStore) UpsertTranscriptSegment(_ context.Context, segment TranscriptSegmentRecord) (TranscriptSegmentRecord, error) {
	if _, ok := s.segments[segment.MeetingID]; !ok {
		s.segments[segment.MeetingID] = make(map[string]TranscriptSegmentRecord)
	}

	s.segments[segment.MeetingID][segment.SegmentID] = cloneTranscriptSegmentRecord(segment)
	return cloneTranscriptSegmentRecord(segment), nil
}

func (s *MemoryMeetingDetailStore) ListTranscriptSegmentsByMeeting(_ context.Context, meetingID string) ([]TranscriptSegmentRecord, error) {
	items := s.segments[meetingID]
	segments := make([]TranscriptSegmentRecord, 0, len(items))
	for _, segment := range items {
		segments = append(segments, cloneTranscriptSegmentRecord(segment))
	}
	return segments, nil
}

func (s *MemoryMeetingDetailStore) UpsertSummarySnapshot(_ context.Context, summary SummarySnapshotRecord) (SummarySnapshotRecord, error) {
	s.summaries[summary.MeetingID] = cloneSummarySnapshotRecord(summary)
	return cloneSummarySnapshotRecord(summary), nil
}

func (s *MemoryMeetingDetailStore) LatestSummarySnapshot(_ context.Context, meetingID string) (*SummarySnapshotRecord, error) {
	summary, ok := s.summaries[meetingID]
	if !ok {
		return nil, nil
	}
	cloned := cloneSummarySnapshotRecord(summary)
	return &cloned, nil
}

func (s *MemoryMeetingDetailStore) ApplyActionItems(_ context.Context, actionItems ActionItemsRecord) (*SummarySnapshotRecord, error) {
	summary, ok := s.summaries[actionItems.MeetingID]
	if !ok {
		summary = SummarySnapshotRecord{
			MeetingID: actionItems.MeetingID,
		}
	}

	summary.Version = maxUint64(summary.Version, actionItems.Version)
	summary.UpdatedAt = actionItems.UpdatedAt
	summary.ActionItems = cloneStrings(actionItems.Items)
	summary.IsFinal = summary.IsFinal || actionItems.IsFinal
	s.summaries[actionItems.MeetingID] = cloneSummarySnapshotRecord(summary)

	cloned := cloneSummarySnapshotRecord(summary)
	return &cloned, nil
}

func (s *MemoryMeetingDetailStore) UpsertCheckpoint(_ context.Context, checkpoint SessionCheckpointRecord) (SessionCheckpointRecord, error) {
	s.checkpoints[checkpoint.MeetingID] = cloneSessionCheckpointRecord(checkpoint)
	return cloneSessionCheckpointRecord(checkpoint), nil
}

func (s *MemoryMeetingDetailStore) FindCheckpoint(_ context.Context, meetingID string) (SessionCheckpointRecord, bool, error) {
	checkpoint, ok := s.checkpoints[meetingID]
	if !ok {
		return SessionCheckpointRecord{}, false, nil
	}
	return cloneSessionCheckpointRecord(checkpoint), true, nil
}

func (s *MemoryMeetingDetailStore) UpsertAudioAssets(_ context.Context, assets AudioAssetRecord) (AudioAssetRecord, error) {
	s.audioAssets[assets.MeetingID] = cloneAudioAssetRecord(assets)
	return cloneAudioAssetRecord(assets), nil
}

func (s *MemoryMeetingDetailStore) FindAudioAssets(_ context.Context, meetingID string) (AudioAssetRecord, bool, error) {
	assets, ok := s.audioAssets[meetingID]
	if !ok {
		return AudioAssetRecord{}, false, nil
	}
	return cloneAudioAssetRecord(assets), true, nil
}

func cloneTranscriptSegmentRecord(segment TranscriptSegmentRecord) TranscriptSegmentRecord {
	cloned := segment
	if segment.SpeakerID != nil {
		speakerID := *segment.SpeakerID
		cloned.SpeakerID = &speakerID
	}
	return cloned
}

func cloneSummarySnapshotRecord(summary SummarySnapshotRecord) SummarySnapshotRecord {
	cloned := summary
	cloned.KeyPoints = cloneStrings(summary.KeyPoints)
	cloned.Decisions = cloneStrings(summary.Decisions)
	cloned.Risks = cloneStrings(summary.Risks)
	cloned.ActionItems = cloneStrings(summary.ActionItems)
	return cloned
}

func cloneSessionCheckpointRecord(checkpoint SessionCheckpointRecord) SessionCheckpointRecord {
	cloned := checkpoint
	if checkpoint.RecoveryToken != nil {
		token := *checkpoint.RecoveryToken
		cloned.RecoveryToken = &token
	}
	return cloned
}

func cloneAudioAssetRecord(assets AudioAssetRecord) AudioAssetRecord {
	cloned := assets
	cloned.MicOriginalPath = cloneOptionalString(assets.MicOriginalPath)
	cloned.SystemOriginalPath = cloneOptionalString(assets.SystemOriginalPath)
	cloned.MixedUplinkPath = cloneOptionalString(assets.MixedUplinkPath)
	return cloned
}

func cloneOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func maxUint64(left, right uint64) uint64 {
	if left > right {
		return left
	}
	return right
}
